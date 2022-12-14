package server

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/wrgl/wrgl/pkg/api"
	"github.com/wrgl/wrgl/pkg/api/payload"
	apiutils "github.com/wrgl/wrgl/pkg/api/utils"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/encoding/packfile"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
	"github.com/wrgl/wrgld/pkg/webhook"
)

type ReceivePackSession struct {
	db           objects.Store
	rs           ref.Store
	c            *conf.Config
	updates      map[string]*payload.Update
	state        stateFn
	receiver     *apiutils.ObjectReceiver
	id           uuid.UUID
	receiverOpts []apiutils.ObjectReceiveOption
	ws           *webhook.Sender
	logger       logr.Logger
}

func parseReceivePackRequest(r *http.Request) (req *payload.ReceivePackRequest, err error) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	defer r.Body.Close()
	req = &payload.ReceivePackRequest{}
	if err = json.Unmarshal(b, req); err != nil {
		return
	}
	return req, nil
}

func NewReceivePackSession(db objects.Store, rs ref.Store, c *conf.Config, id uuid.UUID, ws *webhook.Sender, logger logr.Logger, receiverOpts ...apiutils.ObjectReceiveOption) *ReceivePackSession {
	s := &ReceivePackSession{
		db:           db,
		rs:           rs,
		c:            c,
		id:           id,
		ws:           ws,
		receiverOpts: receiverOpts,
		logger:       logger.WithName("ReceivePackSession").WithValues("session_id", id.String()),
	}
	s.state = s.greet
	return s
}

func (s *ReceivePackSession) respondWithTableACKs(rw http.ResponseWriter, r *http.Request, acks [][]byte) {
	rw.Header().Set("Content-Type", api.CTJSON)
	http.SetCookie(rw, &http.Cookie{
		Name:     api.CookieReceivePackSession,
		Value:    s.id.String(),
		Path:     r.URL.Path,
		HttpOnly: true,
		MaxAge:   3600 * 3,
	})
	resp := &payload.ReceivePackResponse{
		TableACKs: payload.BytesSliceToHexSlice(acks),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	_, err = rw.Write(b)
	if err != nil {
		panic(err)
	}
}

func (s *ReceivePackSession) saveRefs() error {
	if s.ws != nil {
		defer s.ws.Flush()
	}
	for dst, u := range s.updates {
		oldSum, _ := ref.GetRef(s.rs, strings.TrimPrefix(dst, "refs/"))
		if (u.OldSum == nil && oldSum != nil) || (u.OldSum != nil && !bytes.Equal(oldSum, (*u.OldSum)[:])) {
			u.ErrMsg = "remote ref updated since checkout"
			continue
		}
		var sum []byte
		refname := strings.TrimPrefix(dst, "refs/")
		if u.Sum == nil {
			if s.c.Receive != nil && *s.c.Receive.DenyDeletes {
				u.ErrMsg = "remote does not support deleting refs"
			} else {
				err := ref.DeleteRef(s.rs, refname)
				if err != nil {
					return err
				}
				if s.ws != nil {
					evt := &webhook.RefUpdateEvent{
						Ref: refname,
					}
					if oldSum != nil {
						evt.OldSum = hex.EncodeToString(oldSum)
					}
					s.ws.EnqueueEvent(evt)
				}
			}
			continue
		} else {
			sum = (*u.Sum)[:]
			if !objects.CommitExist(s.db, sum) {
				u.ErrMsg = "remote did not receive commit"
				continue
			}
		}
		var msg string
		if oldSum != nil {
			if s.c.Receive != nil && *s.c.Receive.DenyNonFastForwards {
				fastForward, err := ref.IsAncestorOf(s.db, oldSum, sum)
				if err != nil {
					return err
				} else if !fastForward {
					u.ErrMsg = "remote does not support non-fast-fowards"
					continue
				}
			}
			msg = "update ref"
		} else {
			msg = "create ref"
		}
		err := ref.SaveRef(
			s.rs,
			refname,
			sum,
			s.c.User.Name,
			s.c.User.Email,
			"receive-pack",
			msg, nil,
		)
		if err != nil {
			return err
		}
		if s.ws != nil {
			evt := &webhook.RefUpdateEvent{
				Ref:     refname,
				Sum:     hex.EncodeToString(sum),
				Action:  "receive-pack",
				Message: msg,
			}
			if oldSum != nil {
				evt.OldSum = hex.EncodeToString(oldSum)
			}
			s.ws.EnqueueEvent(evt)
		}
	}
	return nil
}

func (s *ReceivePackSession) greet(rw http.ResponseWriter, r *http.Request) (nextState stateFn) {
	defer s.logDuration("greet")()
	var err error
	if v := r.Header.Get("Content-Type"); !strings.Contains(v, api.CTJSON) {
		SendError(rw, r, http.StatusBadRequest, "updates expected")
		return nil
	}
	req, err := parseReceivePackRequest(r)
	if err != nil {
		return
	}
	s.updates = req.Updates
	commits := [][]byte{}
	outdated := false
	for dst, u := range s.updates {
		oldSum, err := ref.GetRef(s.rs, strings.TrimPrefix(dst, "refs/"))
		if err != nil {
			oldSum = nil
		}
		if (u.OldSum == nil && oldSum != nil) || (u.OldSum != nil && !bytes.Equal(oldSum, (*u.OldSum)[:])) {
			outdated = true
			u.ErrMsg = "remote ref updated since checkout"
		}
		if u.Sum != nil {
			commits = append(commits, (*u.Sum)[:])
		}
	}
	if outdated {
		return s.reportStatus(rw, r)
	}
	if len(commits) > 0 {
		s.receiver = apiutils.NewObjectReceiver(s.db, commits, s.logger.V(1), s.receiverOpts...)
		s.respondWithTableACKs(rw, r, s.negotiateTables(req))
		return s.negotiate
	}
	err = s.saveRefs()
	if err != nil {
		panic(err)
	}
	return s.reportStatus(rw, r)
}

func (s *ReceivePackSession) negotiateTables(req *payload.ReceivePackRequest) (acks [][]byte) {
	for _, sum := range req.TableHaves {
		b := (*sum)[:]
		if objects.TableExist(s.db, b) {
			acks = append(acks, b)
		}
	}
	return acks
}

func (s *ReceivePackSession) negotiate(rw http.ResponseWriter, r *http.Request) (nextState stateFn) {
	defer s.logDuration("negotiate")()
	ct := r.Header.Get("Content-Type")
	if ct == api.CTPackfile {
		return s.receiveObjects(rw, r)
	} else if strings.Contains(ct, api.CTJSON) {
		req, err := parseReceivePackRequest(r)
		if err != nil {
			return
		}
		s.respondWithTableACKs(rw, r, s.negotiateTables(req))
		return s.negotiate
	} else {
		SendError(rw, r, http.StatusBadRequest, "unanticipated content-type")
		return nil
	}
}

func (s *ReceivePackSession) receiveObjects(rw http.ResponseWriter, r *http.Request) (nextState stateFn) {
	defer s.logDuration("receive objects")()
	if v := r.Header.Get("Content-Type"); v != api.CTPackfile {
		SendError(rw, r, http.StatusBadRequest, "packfile expected")
		return nil
	}
	body := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(r.Body)
		if err != nil {
			panic(err)
		}
		body = io.NopCloser(gr)
	}
	pr, err := packfile.NewPackfileReader(body)
	if err != nil {
		panic(err)
	}
	done, err := s.receiver.Receive(pr, nil)
	if err != nil {
		panic(err)
	}
	if !done {
		http.SetCookie(rw, &http.Cookie{
			Name:     api.CookieReceivePackSession,
			Value:    s.id.String(),
			Path:     r.URL.Path,
			HttpOnly: true,
			MaxAge:   3600 * 3,
		})
		rw.WriteHeader(http.StatusOK)
		return s.receiveObjects
	}
	err = s.saveRefs()
	if err != nil {
		panic(err)
	}
	return s.reportStatus(rw, r)
}

func (s *ReceivePackSession) reportStatus(rw http.ResponseWriter, r *http.Request) (nextState stateFn) {
	defer s.logDuration("report status")()
	rw.Header().Set("Content-Type", api.CTJSON)
	// remove cookie
	http.SetCookie(rw, &http.Cookie{
		Name:     api.CookieReceivePackSession,
		Value:    s.id.String(),
		Path:     r.URL.Path,
		HttpOnly: true,
		Expires:  time.Now().Add(time.Hour * -24),
	})
	resp := &payload.ReceivePackResponse{
		Updates: s.updates,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	_, err = rw.Write(b)
	if err != nil {
		panic(err)
	}
	return nil
}

func (s *ReceivePackSession) logDuration(msg string) func() {
	start := time.Now()
	return func() {
		s.logger.Info(msg, "duration", time.Since(start))
	}
}

// ServeHTTP receives, saves objects and update refs, returns true when session is
// finished and can be removed.
func (s *ReceivePackSession) ServeHTTP(rw http.ResponseWriter, r *http.Request) bool {
	defer s.logDuration("serve http")()
	s.state = s.state(rw, r)
	return s.state == nil
}
