package server

import (
	"net/http"
	"strconv"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

func itoa(n int) string { return strconv.Itoa(n) }

func codeFor(status int) string {
	switch status {
	case 400:
		return "bad_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 410:
		return "gone"
	case 413:
		return "payload_too_large"
	case 415:
		return "unsupported_media_type"
	case 422:
		return "validation_error"
	case 429:
		return "too_many_requests"
	case 503:
		return "service_unavailable"
	default:
		return "internal_error"
	}
}

// Run starts the HTTP server on PORT (default 8080).
func (s *Server) Run() error {
	addr := ":" + config.Get("PORT", "8080")
	s.log.Infof("listening on %s", addr)
	return http.ListenAndServe(addr, s.Router())
}
