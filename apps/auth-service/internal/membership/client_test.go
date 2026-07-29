package membership

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActive_parsesFleetAndRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"fleet_id":"f1","role":"owner"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	m, err := c.Active(context.Background(), "u1")
	if err != nil || m.FleetID != "f1" || m.Role != "owner" {
		t.Fatalf("got %+v err %v", m, err)
	}
}

func TestActive_noMembershipReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	m, err := NewClient(srv.URL).Active(context.Background(), "u1")
	if err != nil || m.FleetID != "" {
		t.Fatalf("expected empty membership, got %+v err %v", m, err)
	}
}
