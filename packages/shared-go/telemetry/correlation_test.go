package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorrelationID_generatesWhenAbsentAndEchoes(t *testing.T) {
	var seen string
	h := CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = CorrelationIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" {
		t.Fatal("expected a generated correlation id on context")
	}
	if rec.Header().Get("X-Correlation-ID") != seen {
		t.Fatalf("response header %q != context id %q", rec.Header().Get("X-Correlation-ID"), seen)
	}
}

func TestCorrelationID_preservesInbound(t *testing.T) {
	h := CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := CorrelationIDFromContext(r.Context()); got != "abc-123" {
			t.Fatalf("want abc-123, got %q", got)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Correlation-ID", "abc-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
}
