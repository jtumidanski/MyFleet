package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveness_returns200(t *testing.T) {
	rec := httptest.NewRecorder()
	Liveness()(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
}

func TestReadiness_503WhenCheckFails(t *testing.T) {
	h := Readiness(func() error { return http.ErrServerClosed })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", rec.Code)
	}
}

func TestMetrics_nonNilAnd200(t *testing.T) {
	h := Metrics()
	if h == nil {
		t.Fatal("Metrics() returned nil handler")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
}
