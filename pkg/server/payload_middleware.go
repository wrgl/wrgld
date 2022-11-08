package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
)

type payloadRecorderKey struct{}

type payloadRecorder struct {
	requestInfo  interface{}
	responseInfo interface{}
}

func setPayloadRecorder(r *http.Request, pr *payloadRecorder) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), payloadRecorderKey{}, pr))
}

func getPayloadRecorder(r *http.Request) *payloadRecorder {
	if v := r.Context().Value(payloadRecorderKey{}); v != nil {
		return v.(*payloadRecorder)
	}
	return nil
}

func setRequestInfo(r *http.Request, info interface{}) {
	if v := getPayloadRecorder(r); v != nil {
		v.requestInfo = info
	}
}

func setResponseInfo(r *http.Request, info interface{}) {
	if v := getPayloadRecorder(r); v != nil {
		v.responseInfo = info
	}
}

func PayloadMiddleware(logger logr.Logger) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pr := &payloadRecorder{}
			handler.ServeHTTP(w, setPayloadRecorder(r, pr))
			if pr.requestInfo != nil {
				b, err := json.MarshalIndent(pr.requestInfo, "    ", "  ")
				if err != nil {
					panic(err)
				}
				logger.Info("request payload", "method", r.Method, "url", r.URL, "payload", string(b))

				if pr.responseInfo != nil {
					b, err := json.MarshalIndent(pr.responseInfo, "    ", "  ")
					if err != nil {
						panic(err)
					}
					logger.Info("response payload", "payload", string(b))
				}
			}
		})
	}
}
