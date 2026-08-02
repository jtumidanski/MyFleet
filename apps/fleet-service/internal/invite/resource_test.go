package invite

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// inviteTestRouter mounts the real JWT-protected invite routes over an
// in-memory sqlite DB (newInviteDB, from administrator_test.go). The
// authoritative owner check is stubbed to always pass (stubOwnerChecker,
// defined in internal_test.go) — these tests exercise the resource layer's
// own authz/validation/rate-limit wiring, not the membership package's DB
// lookup, which has its own coverage.
func inviteTestRouter(t *testing.T, db *gorm.DB, limits Limits) chi.Router {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := chi.NewRouter()
	InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil, nil, limits)(r)
	return r
}

// ownerRequest builds a request carrying the identity the JWT middleware
// would normally attach for an owner of fleetID — mirrors
// mediaobject/resource_test.go's memberRequest for this package's routes.
func ownerRequest(method, target, fleetID string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	return req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "owner-1",
		ActiveFleetID: fleetID,
		Role:          "owner",
	}))
}

// createInviteBody builds the JSON:API envelope POST /fleets/{id}/invites expects.
func createInviteBody(email, role string) io.Reader {
	body, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"attributes": map[string]any{
				"email": email,
				"role":  role,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return strings.NewReader(string(body))
}

// createInvite issues a real POST through the router and returns the new
// invite's ID, failing the test outright if the create itself doesn't 201 —
// every test below builds on a known-good create.
func createInvite(t *testing.T, r chi.Router, fleetID, email, role string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/"+fleetID+"/invites", fleetID, createInviteBody(email, role)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return doc.Data.ID
}

// TestResend_crossFleetInviteIs404NotForbidden pins the behavior called out
// by the code review: an invite that EXISTS but belongs to a different fleet
// than the one named in the path must return the exact same 404 a
// nonexistent invite ID returns — not a 403. A 403 would tell a caller who is
// a legitimate owner of THEIR OWN fleet that the ID they guessed belongs to
// someone else's fleet, which is an existence oracle; this endpoint was
// changed specifically to close it (design §9, resource.go's "Path-pair
// mismatch" comment).
func TestResend_crossFleetInviteIs404NotForbidden(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour, ResendCooldown: 0})

	// The invite really exists, but in fleet f1.
	otherFleetInviteID := createInvite(t, r, "f1", "victim@example.com", "member")

	// The caller is a legitimate owner, but of a DIFFERENT fleet (f2), and
	// names their own fleet in the path — the shape of an attacker probing
	// invite IDs from their own authenticated session.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f2/invites/"+otherFleetInviteID+"/resend", "f2", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-fleet resend status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	crossFleetBody := rec.Body.String()

	// A genuinely nonexistent ID, requested the same way, must produce the
	// byte-for-byte identical response — that identity is the whole point.
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, ownerRequest(http.MethodPost, "/fleets/f2/invites/does-not-exist/resend", "f2", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("nonexistent-id resend status = %d, want 404; body: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Body.String() != crossFleetBody {
		t.Fatalf("cross-fleet 404 body %q differs from nonexistent-id 404 body %q — existence is leaking", crossFleetBody, rec2.Body.String())
	}
}

// TestCreateInvite_perFleetLimitIs429 pins the FR-RATE-1 wiring: once a
// fleet's per-window creation count is at the limit, the NEXT create is
// rejected with 429, over the real HTTP route (not just at the processor
// unit level — processor_test.go already covers CheckCreateLimit in
// isolation).
func TestCreateInvite_perFleetLimitIs429(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 1, CreateWindow: time.Hour, ResendCooldown: 0})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f1/invites", "f1", createInviteBody("a@example.com", "member")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, ownerRequest(http.MethodPost, "/fleets/f1/invites", "f1", createInviteBody("b@example.com", "member")))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second create (over the per-fleet window limit) status = %d, want 429; body: %s", rec2.Code, rec2.Body.String())
	}
}

// TestResendInvite_cooldownIs429 pins the FR-RATE-2 wiring: a resend issued
// before the cooldown elapses is rejected with 429 over the real HTTP route.
// The invite's updated_at is stamped by Insert moments earlier, so an
// immediate resend is well inside any nonzero cooldown window.
func TestResendInvite_cooldownIs429(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour, ResendCooldown: time.Hour})

	id := createInvite(t, r, "f1", "a@example.com", "member")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f1/invites/"+id+"/resend", "f1", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("resend inside the cooldown window status = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateInvite_malformedEmailIs422 pins the ValidateInviteEmail wiring
// over the real HTTP route. A display-name-wrapped address is rejected
// because ValidateInviteEmail requires the raw input to equal
// mail.ParseAddress's parsed addr-spec exactly — the check that closes off
// header-injection-adjacent inputs (PRD §8 Security); processor_test.go
// covers ValidateInviteEmail itself in isolation.
func TestCreateInvite_malformedEmailIs422(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour, ResendCooldown: 0})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f1/invites", "f1", createInviteBody("Bob <bob@example.com>", "member")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("malformed email status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}
