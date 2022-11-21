package probes

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Probable interface {
	Ready() bool
}

func StartServer(s Probable) {
	sm := http.NewServeMux()
	sm.HandleFunc("/ready", func(rw http.ResponseWriter, r *http.Request) {
		if s.Ready() {
			_, err := rw.Write([]byte("ready"))
			if err != nil {
				panic(err)
			}
		} else {
			http.Error(rw, "not ready", http.StatusTooEarly)
		}
	})
	sm.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		_, err := rw.Write([]byte("ok"))
		if err != nil {
			panic(err)
		}
	})
	sm.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":2112", sm)
	if err != nil {
		panic(err)
	}
}
