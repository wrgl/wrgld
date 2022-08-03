package server

import (
	"encoding/hex"
	"net/http"

	"github.com/wrgl/wrgl/pkg/objects"
)

func (s *Server) handleGetCommitProfile(rw http.ResponseWriter, r *http.Request) {
	db := s.getDB(r)
	m := commitURIPat.FindStringSubmatch(r.URL.Path)
	if m == nil {
		SendHTTPError(rw, r, http.StatusNotFound)
		return
	}
	sum, err := hex.DecodeString(m[1])
	if err != nil {
		panic(err)
	}
	com, err := objects.GetCommit(db, sum)
	if err != nil {
		SendHTTPError(rw, r, http.StatusNotFound)
		return
	}
	prof, err := objects.GetTableProfile(db, com.Table)
	if err != nil {
		SendHTTPError(rw, r, http.StatusNotFound)
		return
	}
	WriteJSON(rw, r, prof)
}

func (s *Server) handleGetTableProfile(rw http.ResponseWriter, r *http.Request) {
	db := s.getDB(r)
	m := tableURIPat.FindStringSubmatch(r.URL.Path)
	if m == nil {
		SendHTTPError(rw, r, http.StatusNotFound)
		return
	}
	sum, err := hex.DecodeString(m[1])
	if err != nil {
		panic(err)
	}
	prof, err := objects.GetTableProfile(db, sum)
	if err != nil {
		SendHTTPError(rw, r, http.StatusNotFound)
		return
	}
	WriteJSON(rw, r, prof)
}
