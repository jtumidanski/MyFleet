package membership

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serving(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL)
}

// fleetErrorEnvelope is fleet-service's real error body. That it is JSON is
// exactly why a non-2xx used to decode cleanly: none of its keys match
// Membership's, so encoding/json fills in nothing and reports no error.
const fleetErrorEnvelope = `{"errors":[{"status":"500","code":"internal","title":"internal server error"}]}`

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

// TestActive_failsClosedOnANon404Error guards the failure the caller cannot
// survive. Active special-cased 404 only, so any OTHER non-2xx whose body
// happened to be JSON — which fleet-service's error envelope is — decoded into
// a zero Membership with err == nil. newPrincipalResolver then minted a
// perfectly valid token claiming the user has no fleet, and the SPA sent them
// to /onboarding to create a duplicate one.
//
// TestActive_noMembershipReturnsEmpty above must keep passing beside this: 404
// stays the one status that is not an error.
func TestActive_failsClosedOnANon404Error(t *testing.T) {
	for _, status := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusBadRequest,
	} {
		m, err := serving(t, status, fleetErrorEnvelope).Active(context.Background(), "u1")
		if err == nil {
			t.Fatalf("status %d returned no error and membership %+v — a token would be minted claiming no fleet", status, m)
		}
		if m != (Membership{}) {
			t.Fatalf("status %d returned membership %+v, want the zero value alongside the error", status, m)
		}
	}
}

// TestActive_errorDisclosesNeitherBodyNorUser keeps the failure loggable. The
// response body is upstream-controlled and could carry anything, and the user
// id must not travel inside a message an operator might read as an address.
// Status code plus a fixed description only.
func TestActive_errorDisclosesNeitherBodyNorUser(t *testing.T) {
	body := `{"errors":[{"title":"leaked@example.com had a bad day"}]}`
	_, err := serving(t, http.StatusInternalServerError, body).Active(context.Background(), "user-42")
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "leaked@example.com") || strings.Contains(msg, "bad day") {
		t.Fatalf("error echoes the response body: %q", msg)
	}
	if strings.Contains(msg, "user-42") {
		t.Fatalf("error carries the user id: %q", msg)
	}
	if !strings.Contains(msg, "500") {
		t.Fatalf("error must name the status code so the failure is diagnosable: %q", msg)
	}
}
