package server

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/wrgl/wrgl/pkg/api"
	"github.com/wrgl/wrgl/pkg/api/payload"
	"github.com/wrgl/wrgl/pkg/transaction"
	"github.com/wrgl/wrgld/pkg/webhook"
)

func parseJSONRequest(r *http.Request, rw http.ResponseWriter, obj interface{}) bool {
	if v := r.Header.Get("Content-Type"); !strings.Contains(v, api.CTJSON) {
		SendError(rw, r, http.StatusBadRequest, "JSON payload expected")
		return false
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
	return true
}

func (s *Server) handleUpdateTransaction(rw http.ResponseWriter, r *http.Request) {
	author := GetAuthor(r)
	if author == nil {
		SendHTTPError(rw, r, http.StatusUnauthorized)
		return
	}
	db := s.getDB(r)
	rs := s.getRS(r)
	tid, ok := extractTransactionID(rw, r, rs)
	if !ok {
		return
	}
	req := &payload.UpdateTransactionRequest{}
	if !parseJSONRequest(r, rw, req) {
		return
	}
	if req.Commit {
		commitsMap, err := transaction.Commit(db, rs, *tid)
		if err != nil {
			panic(err)
		}
		ws, err := webhook.NewSender(s.getConfig(r), s.logger, s.webhookSenderOpts...)
		if err != nil {
			panic(err)
		}
		defer ws.Flush()
		commits := []webhook.Commit{}
		for refname, com := range commitsMap {
			commits = append(commits, webhook.Commit{
				Sum:     hex.EncodeToString(com.Sum),
				Ref:     refname,
				Message: com.Message,
			})
		}
		ws.EnqueueEvent(&webhook.CommitEvent{
			TransactionID: tid.String(),
			Commits:       commits,
			AuthorName:    author.Name,
			AuthorEmail:   author.Email,
		})
	} else if req.Discard {
		if err := transaction.Discard(rs, *tid); err != nil {
			panic(err)
		}
	} else {
		SendError(rw, r, http.StatusBadRequest, "must either discard or commit transaction")
		return
	}
}
