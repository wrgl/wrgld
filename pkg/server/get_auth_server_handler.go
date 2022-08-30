package server

import "net/http"

func (s *Server) handleGetAuthServer(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, r, s.getAuthServer(r))
}
