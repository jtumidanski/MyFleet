package mediaclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestValidateOwnership_fullMatchPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fleet_id"); got != "fleet-a" {
			t.Errorf("fleet_id = %q", got)
		}
		if got := r.URL.Query().Get("ids"); got != "m1,m2" {
			t.Errorf("ids = %q", got)
		}
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"ready","content_type":"application/pdf"},{"id":"m2","status":"uploaded","content_type":"image/jpeg"}]}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1", "m2"}); err != nil {
		t.Fatalf("ValidateOwnership: %v", err)
	}
}

// A missing ID is 422 and is indistinguishable between "does not exist",
// "was deleted" and "belongs to another fleet" — 403 would confirm the ID
// exists somewhere else (api-contracts §3).
func TestValidateOwnership_shortSetIsValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"ready","content_type":"application/pdf"}]}`))
	}))
	defer srv.Close()

	err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1", "m2"})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// Ownership is the security property; readiness is a UX property. Requiring
// ready here would reject a legitimate save when a JPEG's variant worker is a
// second behind the user's click (design D8).
func TestValidateOwnership_doesNotRequireReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"processing","content_type":"image/jpeg"}]}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1"}); err != nil {
		t.Fatalf("a processing object was rejected: %v", err)
	}
}

// media-service unreachable means the record is not created: the alternative
// trades a visible failure for a silent one on the exact path the check exists
// to protect (design D7). The transport error propagates, so StatusFor maps it
// to 500 — NOT to a validation error.
func TestValidateOwnership_transportErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1"})
	if err == nil {
		t.Fatal("a 500 from media-service must not be treated as success")
	}
	if errors.Is(err, server.ErrValidation) {
		t.Fatal("a transport failure must not masquerade as a validation error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error does not name the status: %v", err)
	}
}

// The common case — logging an oil change with no receipt — makes no
// cross-service call at all.
func TestValidateOwnership_emptyInputMakesNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", nil); err != nil {
		t.Fatalf("ValidateOwnership(nil): %v", err)
	}
	if called {
		t.Fatal("an empty id list must not issue a request")
	}
}
