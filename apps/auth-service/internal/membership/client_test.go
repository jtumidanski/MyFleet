package membership

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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

// TestFleetMemberIDs_transportErrorCarriesNoID covers the path
// TestFleetMemberIDs_errorCarriesNoIDAndNoBody cannot reach: that one only
// exercises the non-2xx branch, which was already safe. On the TRANSPORT branch
// the bare error from http.Client.Do is a *url.Error whose message embeds the
// request URL — and this request's URL carries the fleet id in its path. The
// user resource logs this error verbatim, so the id landed in the logs as an
// address on every fleet-service outage.
func TestFleetMemberIDs_transportErrorCarriesNoID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // nothing is listening on that port now

	_, err := NewClient(base).FleetMemberIDs(context.Background(), "fleet-abc")

	if err == nil {
		t.Fatal("an unreachable fleet-service must not resolve to a member list")
	}
	for _, secret := range []string{"fleet-abc", "/internal/fleets/"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("the error carries %q — the request URL rode into it, and with it the fleet id: %q", secret, err)
		}
	}
	// Redaction must not cost the diagnostic: an operator needs to see "connection
	// refused" / "no such host" / "context deadline exceeded", just not the address.
	const prefix = "fleet member lookup transport failure: "
	if strings.TrimPrefix(err.Error(), prefix) == "" {
		t.Fatalf("the redaction dropped the transport diagnostic entirely: %q", err)
	}
}

// TestActive_classifiesEveryResponseShape is the table the whole task rests on.
// Each row asserts the CLASSIFICATION, not merely that an error occurred: the
// two buckets have opposite correct responses — a transient error becomes a 503
// that preserves the session, a permanent one becomes a 401 that ends it — so a
// test that only checked `err != nil` would pass while logging every user out.
func TestActive_classifiesEveryResponseShape(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		body          string
		wantErr       bool
		wantTransient bool
		wantFleetID   string
	}{
		{name: "success", status: 200, body: `{"fleet_id":"f1","role":"owner"}`, wantFleetID: "f1"},
		// Load-bearing: a user with no fleet is a real state, and the OIDC
		// callback keys its onboarding redirect off the empty fleet id.
		{name: "no membership", status: 404, body: ""},
		{name: "upstream 500", status: 500, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "upstream 502", status: 502, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "upstream 503", status: 503, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "rate limited", status: 429, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		// A 4xx that is not 404 or 429 is a contract or authorization fault
		// between the two services. Retrying cannot fix it, so it must NOT keep
		// the session alive on a promise of recovery.
		{name: "bad request", status: 400, body: fleetErrorEnvelope, wantErr: true},
		{name: "unauthorized", status: 401, body: fleetErrorEnvelope, wantErr: true},
		{name: "forbidden", status: 403, body: fleetErrorEnvelope, wantErr: true},
		// A 2xx whose body will not parse is a garbled or truncated response,
		// not a definitive answer.
		{name: "unparseable 2xx body", status: 200, body: `{"fleet_id":`, wantErr: true, wantTransient: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := serving(t, tc.status, tc.body).Active(context.Background(), "u1")

			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got := errors.Is(err, server.ErrServiceUnavailable); got != tc.wantTransient {
				t.Fatalf("transient = %v, want %v (err = %v) — this classification is the difference "+
					"between a 503 that preserves the session and a 401 that ends it", got, tc.wantTransient, err)
			}
			if tc.wantErr && m != (Membership{}) {
				t.Fatalf("membership = %+v alongside an error, want the zero value", m)
			}
			if !tc.wantErr && m.FleetID != tc.wantFleetID {
				t.Fatalf("FleetID = %q, want %q", m.FleetID, tc.wantFleetID)
			}
		})
	}
}

// TestActive_classifiesATransportFailureAsTransient covers the shape the
// acceptance criteria name second: fleet-service unreachable, connection
// refused. It also pins the disclosure rule on the one path where it is easy to
// break — url.Error's message embeds the request URL, and this request's URL
// carries the user id as a query parameter.
func TestActive_classifiesATransportFailureAsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // nothing is listening on that port now

	_, err := NewClient(base).Active(context.Background(), "user-42")

	if err == nil {
		t.Fatal("an unreachable fleet-service must not resolve to a membership")
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("connection refused must classify transient, got %v", err)
	}
	for _, secret := range []string{"user-42", "user_id"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("the error carries %q — the request URL rode into it, and with it the user id: %q", secret, err)
		}
	}
}

// TestActive_boundsAHangAndClassifiesItTransient is the criterion the other
// tests cannot reach: a hang is the most likely outage shape and the worst,
// because Client shares http.DefaultClient, which has no timeout of its own.
// Without the deadline this test does not fail — it never returns.
func TestActive_boundsAHangAndClassifiesItTransient(t *testing.T) {
	prev := fleetLookupTimeout
	t.Cleanup(func() { fleetLookupTimeout = prev })
	fleetLookupTimeout = 20 * time.Millisecond

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	start := time.Now()
	m, err := NewClient(srv.URL).Active(context.Background(), "u1")

	if err == nil {
		t.Fatalf("a hanging fleet-service returned membership %+v with no error", m)
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a timeout must classify transient — this is what turns a hang into a 503 rather than a logout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Active took %v; the handler was pinned open rather than bounded by fleetLookupTimeout", elapsed)
	}
}

// Pins the escaping Active does. main added url.QueryEscape (03da0f9) without a
// test that fails if it is removed: every other test here passes a plain UUID,
// for which escaping is a no-op. This one passes a userID containing `&`, which
// under raw concatenation terminates user_id and injects the remainder as its
// own parameter.
func TestActive_escapesTheUserIDIntoTheQueryString(t *testing.T) {
	const hostile = "u1&role=owner"

	var seen, seenRole string
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
