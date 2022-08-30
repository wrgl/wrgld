package server

import (
	"context"
	"net/http"
)

type Author struct {
	Name  string
	Email string
}

type authorKey struct{}

func SetAuthor(r *http.Request, author *Author) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authorKey{}, author))
}

func GetAuthor(r *http.Request) *Author {
	if i := r.Context().Value(authorKey{}); i != nil {
		return i.(*Author)
	}
	return nil
}
