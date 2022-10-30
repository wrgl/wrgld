package wrgld

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-logr/logr"
)

type loggingMiddleware struct {
	handler http.Handler
	logger  logr.Logger
}

func (h *loggingMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.handler.ServeHTTP(rw, r)
	h.logger.Info("request", "method", r.Method, "uri", r.URL.RequestURI(), "elapsed", time.Since(start))
}

func LoggingMiddleware(logger logr.Logger) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return &loggingMiddleware{handler: handler, logger: logger}
	}
}

type recoveryMiddleware struct {
	handler http.Handler
	logger  logr.Logger
}

func (h *recoveryMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			var err error
			if v, ok := r.(error); ok {
				err = v
			} else {
				err = fmt.Errorf("%v", r)
			}
			h.logger.Error(err, "panic recovered", "stack", string(debug.Stack()))
			http.Error(rw, "internal server error", http.StatusInternalServerError)
		}
	}()
	h.handler.ServeHTTP(rw, r)
}

func RecoveryMiddleware(logger logr.Logger) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return &recoveryMiddleware{handler: handler, logger: logger}
	}
}
