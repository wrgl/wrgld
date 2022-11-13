package server

import (
	"net/http"

	"github.com/wrgl/wrgl/pkg/prune"
	"github.com/wrgl/wrgl/pkg/transaction"
)

func (s *Server) handleGC(rw http.ResponseWriter, r *http.Request) {
	db := s.getDB(r)
	rs := s.getRS(r)
	cs := s.getConfS(r)
	c, err := cs.Open()
	if err != nil {
		panic(err)
	}
	if err = transaction.GarbageCollect(db, rs, c.GetTransactionTTL(), nil); err != nil {
		panic(err)
	}
	if err = prune.Prune(db, rs, nil); err != nil {
		panic(err)
	}
}
