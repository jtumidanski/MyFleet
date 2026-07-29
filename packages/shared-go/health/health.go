package health

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Liveness() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
}

// Readiness fails (503) if any dependency check errors (design §15: DB ping + deps).
func Readiness(checks ...func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for _, c := range checks {
			if err := c(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// Metrics returns the default Prometheus metrics handler (design §10: /metrics).
func Metrics() http.Handler {
	return promhttp.Handler()
}
