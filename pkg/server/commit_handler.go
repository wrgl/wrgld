package server

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/gzip"
	"github.com/wrgl/wrgl/pkg/api/payload"
	"github.com/wrgl/wrgl/pkg/ingest"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
	"github.com/wrgl/wrgl/pkg/sorter"
	"github.com/wrgl/wrgld/pkg/webhook"
)

func (s *Server) handleCommit(rw http.ResponseWriter, r *http.Request) {
	author := GetAuthor(r)
	if author == nil {
		SendHTTPError(rw, r, http.StatusUnauthorized)
		return
	}
	err := r.ParseMultipartForm(0)
	if err != nil {
		if err == http.ErrNotMultipart || err == http.ErrMissingBoundary {
			SendError(rw, r, http.StatusUnsupportedMediaType, err.Error())
			return
		}
		panic(err)
	}
	branch := r.PostFormValue("branch")
	if branch == "" {
		SendError(rw, r, http.StatusBadRequest, "missing branch name")
		return
	}
	if !ref.HeadPattern.MatchString(branch) {
		SendError(rw, r, http.StatusBadRequest, "invalid branch name")
		return
	}
	message := r.PostFormValue("message")
	if message == "" {
		SendError(rw, r, http.StatusBadRequest, "missing message")
		return
	}
	if len(r.MultipartForm.File["file"]) == 0 {
		SendError(rw, r, http.StatusBadRequest, "missing file")
		return
	}
	fh := r.MultipartForm.File["file"][0]
	var f io.ReadCloser
	f, err = fh.Open()
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if strings.HasSuffix(fh.Filename, ".gz") {
		f, err = gzip.NewReader(f)
		if err != nil {
			panic(err)
		}
		defer f.Close()
	}
	var primaryKey []string
	if sl := r.MultipartForm.Value["primaryKey"]; len(sl) > 0 && len(sl[0]) > 0 {
		primaryKey = strings.Split(sl[0], ",")
	}
	db := s.getDB(r)
	rs := s.getRS(r)

	var tid *uuid.UUID
	if s := r.PostFormValue("txid"); s != "" {
		tid = &uuid.UUID{}
		*tid, err = uuid.Parse(s)
		if err != nil {
			SendError(rw, r, http.StatusBadRequest, "invalid txid")
			return
		}
		if _, err := rs.GetTransaction(*tid); err != nil {
			SendError(rw, r, http.StatusNotFound, "transaction not found")
			return
		}
	}

	var opts = []ingest.InserterOption{}
	sorter := s.sPool.Get().(*sorter.Sorter)
	sorter.Reset()
	defer s.sPool.Put(sorter)
	sum, err := ingest.IngestTable(db, sorter, f, primaryKey, s.logger.V(1), opts...)
	if err != nil {
		if v, ok := err.(*csv.ParseError); ok {
			sendCSVError(rw, r, v)
			return
		} else if v, ok := err.(*ingest.Error); ok {
			SendError(rw, r, http.StatusBadRequest, fmt.Sprintf("ingest error: %s", v.Error()))
			return
		} else {
			panic(err)
		}
	}

	commit := &objects.Commit{
		Table:       sum,
		Message:     message,
		Time:        time.Now(),
		AuthorEmail: author.Email,
		AuthorName:  author.Name,
	}
	parent, _ := ref.GetHead(rs, branch)
	if parent != nil {
		commit.Parents = [][]byte{parent}
	}
	buf := bytes.NewBuffer(nil)
	_, err = commit.WriteTo(buf)
	if err != nil {
		panic(err)
	}
	commitSum, err := objects.SaveCommit(db, buf.Bytes())
	if err != nil {
		panic(err)
	}
	if tid != nil {
		if err = ref.SaveTransactionRef(rs, *tid, branch, commitSum); err != nil {
			panic(err)
		}
	} else {
		if err = ref.CommitHead(rs, branch, commitSum, commit, nil); err != nil {
			panic(err)
		}
		ws, err := webhook.NewSender(s.getConfig(r), s.logger, s.webhookSenderOpts...)
		if err != nil {
			panic(err)
		}
		defer ws.Flush()
		ws.EnqueueEvent(&webhook.CommitEvent{
			Commits: []webhook.Commit{
				{
					Sum:     hex.EncodeToString(commitSum),
					Ref:     ref.HeadRef(branch),
					Message: commit.Message,
				},
			},
			AuthorName:  commit.AuthorName,
			AuthorEmail: commit.AuthorEmail,
		})
	}

	if s.postCommit != nil {
		s.postCommit(r, commit, commitSum, branch, tid)
	}
	resp := &payload.CommitResponse{
		Sum:   &payload.Hex{},
		Table: &payload.Hex{},
	}
	copy((*resp.Sum)[:], commitSum)
	copy((*resp.Table)[:], sum)
	WriteJSON(rw, r, resp)
}
