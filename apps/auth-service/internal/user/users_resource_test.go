package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// newUsersRouter builds the real router with a STUB gatherer, so the scoping
// rules are unit-testable at the handler without standing up fleet-service.
// That injectability is the whole point of the function-value parameter.
func newUsersRouter(t *testing.T, members FleetMemberGatherer) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, members))
	return r, db
}

func gatherer(ids ...string) FleetMemberGatherer {
	return func(context.Context, string) ([]string, error) { return ids, nil }
}

func getUsers(r chi.Router, query, activeFleetID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/auth/users"+query, nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "caller",
		Email:         "caller@example.com",
		ActiveFleetID: activeFleetID,
		Role:          "owner",
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// decodeIDs pulls the resource ids out of the JSON:API document, and fails if
// `data` is missing entirely — `{"data":[]}` and `{}` are different answers.
func decodeIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var doc struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if doc.Data == nil {
		t.Fatalf("response has no `data` key; an empty result must marshal as [], not be omitted. Body: %s",
			strings.TrimSpace(rec.Body.String()))
	}
	out := make([]string, 0, len(*doc.Data))
	for _, d := range *doc.Data {
		out = append(out, d.ID)
	}
	return out
}

func TestAuthUsers_returnsRequestedFleetMembers(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1", "u2"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "u2", "sub-2", "two@example.com", "Two")

	rec := getUsers(r, "?ids=u1,u2", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/users = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	// Captured before decodeIDs: json.Decoder.Decode drains the underlying
	// bytes.Buffer as it reads, so rec.Body.String() reads back empty once the
	// document has been decoded.
	body := rec.Body.String()
	ids := decodeIDs(t, rec)
	if len(ids) != 2 {
		t.Fatalf("got %v, want both users", ids)
	}
	if !strings.Contains(body, "One") || !strings.Contains(body, "one@example.com") {
		t.Errorf("attributes must carry displayName and email; got %s", body)
	}
}

// SEC-1, the security property of the whole endpoint. A caller in fleet A
// asking about a user in fleet B must get a response INDISTINGUISHABLE from
// asking about an id that does not exist: 200 with the id absent. Not 403 (that
// confirms the user exists), not 404 (same), not an error of any kind — any of
// those turn this into a membership oracle.
func TestAuthUsers_silentlyOmitsUsersOutsideTheCallersFleet(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1")) // only u1 is in the caller's fleet
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "other", "sub-o", "other@example.com", "Other Fleet Person")

	rec := getUsers(r, "?ids=u1,other", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 — a foreign id must not change the status", rec.Code)
	}
	// Captured before decodeIDs: json.Decoder.Decode drains the underlying
	// bytes.Buffer as it reads, so rec.Body.String() reads back empty once the
	// document has been decoded.
	body := rec.Body.String()
	ids := decodeIDs(t, rec)
	if len(ids) != 1 || ids[0] != "u1" {
		t.Fatalf("got %v, want only u1", ids)
	}
	if strings.Contains(body, "other@example.com") {
		t.Fatal("a user outside the caller's fleet leaked into the response")
	}
}

// The other half of SEC-1: an id nobody has must produce the SAME response
// shape as an id belonging to another fleet.
func TestAuthUsers_foreignAndNonexistentIDsAreIndistinguishable(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "other", "sub-o", "other@example.com", "Other")

	foreign := getUsers(r, "?ids=other", "f1")
	ghost := getUsers(r, "?ids=does-not-exist", "f1")

	if foreign.Code != ghost.Code {
		t.Fatalf("status differs: foreign %d vs nonexistent %d", foreign.Code, ghost.Code)
	}
	if foreign.Body.String() != ghost.Body.String() {
		t.Fatalf("body differs, which makes this a membership oracle:\nforeign: %s\nghost:   %s",
			foreign.Body.String(), ghost.Body.String())
	}
}

// FR-1.4: a fleet member with no users row is omitted, not an error.
func TestAuthUsers_omitsFleetMembersWithNoUserRow(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1", "orphan"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1,orphan", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 1 || ids[0] != "u1" {
		t.Fatalf("got %v, want only u1", ids)
	}
}

// FR-1.3. The gatherer must not even be consulted — there is no fleet to ask
// about, and calling it would be a pointless round trip on every fleetless load.
func TestAuthUsers_emptyDataWhenTheCallerHasNoActiveFleet(t *testing.T) {
	called := false
	r, db := newUsersRouter(t, func(context.Context, string) ([]string, error) {
		called = true
		return nil, nil
	})
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 0 {
		t.Fatalf("got %v, want an empty data array", ids)
	}
	if called {
		t.Error("the fleet lookup ran for a caller with no fleet")
	}
}

func TestAuthUsers_rejectsMissingOrEmptyIDs(t *testing.T) {
	r, _ := newUsersRouter(t, gatherer("u1"))

	for _, query := range []string{"", "?ids=", "?ids=%20", "?ids=,,,"} {
		rec := getUsers(r, query, "f1")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("query %q = %d, want 422", query, rec.Code)
		}
	}
}

// SEC-3. The cap bounds both the response size and the work per request.
func TestAuthUsers_rejectsMoreThan100IDs(t *testing.T) {
	r, _ := newUsersRouter(t, gatherer("u1"))

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "u" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
	}
	rec := getUsers(r, "?ids="+strings.Join(ids, ","), "f1")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("101 ids = %d, want 422", rec.Code)
	}
}

// De-duplication happens BEFORE the cap, so a repeated id cannot inflate the
// work done or trip the limit on its own.
func TestAuthUsers_deduplicatesBeforeApplyingTheCap(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	repeated := make([]string, 300)
	for i := range repeated {
		repeated[i] = "u1"
	}
	rec := getUsers(r, "?ids="+strings.Join(repeated, ","), "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("300 copies of one id = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 1 {
		t.Fatalf("got %v, want a single u1", ids)
	}
}

// D4. Returning an empty 200 on a downstream failure would make a fleet-service
// outage indistinguishable from a fleet with no members — exactly the class of
// bug the membership.Client comment was written after. A 500 is visible in
// metrics and logs; a silent empty array is not. FR-1.7 still holds either way:
// the SPA renders id fallbacks regardless of which it gets.
func TestAuthUsers_returns500WhenTheFleetLookupFails(t *testing.T) {
	r, db := newUsersRouter(t, func(context.Context, string) ([]string, error) {
		return nil, errors.New("fleet-service is down")
	})
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1", "f1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "fleet-service is down") {
		t.Fatal("the downstream error text reached the client; errInternal must render a bare 500")
	}
}
