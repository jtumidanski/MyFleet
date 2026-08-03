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

func TestFleetMemberIDs_projectsUserIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/fleets/f1/members" {
			t.Errorf("path = %q, want /internal/fleets/f1/members", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"user_id":"u1","role":"owner"},{"user_id":"u2","role":"viewer"}]`))
	}))
	defer srv.Close()

	ids, err := NewClient(srv.URL).FleetMemberIDs(context.Background(), "f1")
	if err != nil {
		t.Fatalf("FleetMemberIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "u1" || ids[1] != "u2" {
		t.Fatalf("got %v, want [u1 u2]", ids)
	}
}

func TestFleetMemberIDs_returnsEmptyForAFleetWithNoMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ids, err := NewClient(srv.URL).FleetMemberIDs(context.Background(), "f1")
	if err != nil || len(ids) != 0 {
		t.Fatalf("got %v err %v, want an empty slice and no error", ids, err)
	}
}

// The failure Active was written to catch, in a place where it bites harder:
// fleet-service's error envelope is JSON, so without an explicit status check a
// 500 decodes into an empty slice with err == nil, and every member name
// silently disappears from the settings card with nothing in the logs.
func TestFleetMemberIDs_failsClosedOnANon2xx(t *testing.T) {
	for _, status := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusUnauthorized,
	} {
		c := serving(t, status, fleetErrorEnvelope)
		ids, err := c.FleetMemberIDs(context.Background(), "f1")
		if err == nil {
			t.Errorf("status %d returned ids %v with no error; it must fail closed", status, ids)
		}
	}
}

// Active maps 404 to a zero value because "this user has no fleet" is a real
// state. Here the fleet id came off a validated token, so a 404 means something
// is wrong — it must NOT become "this fleet has no members".
func TestFleetMemberIDs_treats404AsAnError(t *testing.T) {
	c := serving(t, http.StatusNotFound, "")
	if ids, err := c.FleetMemberIDs(context.Background(), "f1"); err == nil {
		t.Fatalf("404 returned ids %v with no error; a fleet id from a valid token must exist", ids)
	}
}

// Status code only: the body is upstream-controlled and the fleet id must not
// ride along into a log line as an address.
func TestFleetMemberIDs_errorCarriesNoIDAndNoBody(t *testing.T) {
	c := serving(t, http.StatusInternalServerError, `{"errors":[{"detail":"secret-internal-detail"}]}`)
	_, err := c.FleetMemberIDs(context.Background(), "fleet-abc")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "fleet-abc") || strings.Contains(err.Error(), "secret-internal-detail") {
		t.Fatalf("error %q must carry neither the fleet id nor the upstream body", err)
	}
}

// Active builds its query string by concatenation, so a userID carrying a `&`
// would end the user_id parameter and inject whatever follows as a separate
// one. Not currently reachable — the caller passes a server-generated UUID —
// but client.go:80-82 already calls out "Active's raw-concatenation habit" as
// something not to inherit, and the next caller may not be so careful.
func TestActive_escapesTheUserIDIntoTheQueryString(t *testing.T) {
	const hostile = "u1&role=owner"

	var seen string
	var seenRole string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("user_id")
		seenRole = r.URL.Query().Get("role")
		_, _ = w.Write([]byte(`{"fleet_id":"f1","role":"viewer"}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Active(context.Background(), hostile); err != nil {
		t.Fatalf("Active: %v", err)
	}
	if seen != hostile {
		t.Fatalf("user_id arrived as %q, want %q — the value was not escaped", seen, hostile)
	}
	if seenRole != "" {
		t.Fatalf("a second parameter role=%q was injected from the user id", seenRole)
	}
}
