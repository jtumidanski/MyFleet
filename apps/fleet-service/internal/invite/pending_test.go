package invite

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// GET /invites/pending exists because an invitee had no way to find their
// invite. Nothing delivers invites, so a user who was invited before they ever
// signed in logged in, hit the fleetless bounce, and saw only "create a fleet"
// — the invite waiting for them was unreachable and undiscoverable. This
// endpoint is the discovery path: it answers "what is waiting for ME", scoped
// entirely by the caller's validated `email` claim.

// newPendingRouter seeds invites plus the fleets they point at, so the response
// can name the fleet the caller is being invited to.
func newPendingRouter(t *testing.T, fleets map[string]string, invites ...Model) chi.Router {
	t.Helper()
	db := newInviteTestDB(t)
	seedFleets(t, db, fleets)

	adm := NewAdministrator(db)
	for _, inv := range invites {
		if _, err := adm.Insert(context.Background(), inv, "trace-test"); err != nil {
			t.Fatalf("seed invite: %v", err)
		}
	}

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil, nil, Limits{CreatePerWindow: 1000, CreateWindow: time.Hour}))
	return r
}

// seedFleets creates the fleet rows the pending listing joins against. The
// invite tests' DDL only builds fleet_invites, so the fleets table is local to
// these cases.
func seedFleets(t *testing.T, db *gorm.DB, fleets map[string]string) {
	t.Helper()
	ddl := `CREATE TABLE fleet.fleets (
		id TEXT PRIMARY KEY, name TEXT, created_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("fleets ddl: %v", err)
	}
	for id, name := range fleets {
		if err := db.Exec("INSERT INTO fleet.fleets (id, name) VALUES (?, ?)", id, name).Error; err != nil {
			t.Fatalf("seed fleet %s: %v", id, err)
		}
	}
}

type pendingItem struct {
	ID         string `json:"id"`
	Attributes struct {
		FleetID   string `json:"fleetId"`
		FleetName string `json:"fleetName"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Token     string `json:"token"`
	} `json:"attributes"`
}

func getPending(t *testing.T, r chi.Router, authedEmail string) (*httptest.ResponseRecorder, []pendingItem) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/invites/pending", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID: "user-1",
		Email:  authedEmail,
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /invites/pending = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	var doc struct {
		Data []pendingItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body.String())
	}
	return rec, doc.Data
}

func TestPendingRoute_returnsTheCallersOwnInviteWithItsFleetName(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	r := newPendingRouter(t, map[string]string{"fleet-1": "Tumidanski Household"},
		seedInvite("jane@example.com", future, nil))

	_, items := getPending(t, r, "jane@example.com")

	if len(items) != 1 {
		t.Fatalf("got %d invites, want 1: %+v", len(items), items)
	}
	if items[0].Attributes.FleetName != "Tumidanski Household" {
		t.Fatalf("fleetName = %q, want the fleet's name. Without it the invitee is "+
			"asked to join an opaque uuid — and a fleetless user cannot read "+
			"/fleets/{id} to resolve it themselves", items[0].Attributes.FleetName)
	}
	if items[0].Attributes.Token == "" {
		t.Fatal("token is empty; it is what the accept call is keyed on")
	}
}

// The whole endpoint is an email-scoped lookup, so this is its security
// contract: one caller must never see another's invite.
func TestPendingRoute_omitsInvitesAddressedToSomeoneElse(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	r := newPendingRouter(t, map[string]string{"fleet-1": "Household"},
		seedInvite("jane@example.com", future, nil),
		seedInvite("bob@example.com", future, nil))

	_, items := getPending(t, r, "jane@example.com")

	if len(items) != 1 {
		t.Fatalf("got %d invites, want only the caller's: %+v", len(items), items)
	}
	if !strings.EqualFold(items[0].Attributes.Email, "jane@example.com") {
		t.Fatalf("returned an invite for %q to jane@example.com", items[0].Attributes.Email)
	}
}

// Parity with ValidateAccept, which compares with strings.EqualFold. If the
// listing were case-sensitive it would hide an invite the accept route would
// happily honour — the user would be told they have nothing waiting.
func TestPendingRoute_matchesTheEmailCaseInsensitively(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	r := newPendingRouter(t, map[string]string{"fleet-1": "Household"},
		seedInvite("Jane@Example.COM", future, nil))

	_, items := getPending(t, r, "jane@example.com")

	if len(items) != 1 {
		t.Fatalf("got %d invites, want 1 — accept matches case-insensitively, so listing must too", len(items))
	}
}

func TestPendingRoute_omitsSpentAndExpiredInvites(t *testing.T) {
	now := time.Now()
	accepted := now.Add(-time.Hour)
	// Rows to be written directly, so built via Make like seedInvite — accepted_at
	// is only ever stamped inside Administrator.Accept's transaction.
	r := newPendingRouter(t, map[string]string{"fleet-1": "Household"},
		Make(Entity{
			ID: uuid.NewString(), FleetID: "fleet-1", Email: "jane@example.com", Role: "member",
			Token: "tok-accepted", ExpiresAt: now.Add(24 * time.Hour), AcceptedAt: &accepted,
			InvitedByUserID: "owner-1",
		}),
		Make(Entity{
			ID: uuid.NewString(), FleetID: "fleet-1", Email: "jane@example.com", Role: "member",
			Token: "tok-expired", ExpiresAt: now.Add(-time.Hour), InvitedByUserID: "owner-1",
		}),
	)

	_, items := getPending(t, r, "jane@example.com")

	if len(items) != 0 {
		t.Fatalf("got %d invites, want 0 — an accepted or expired invite can only "+
			"produce a 409 if offered: %+v", len(items), items)
	}
}

// A token that validates but carries no email claim is a known failure mode
// (see the warning in packages/shared-go/auth/middleware.go). Matching a blank
// address would hand every corrupt blank-address row in the table to whoever
// holds such a token.
func TestPendingRoute_returnsNothingForAnEmptyEmailClaim(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	r := newPendingRouter(t, map[string]string{"fleet-1": "Household"},
		seedInvite("", future, nil),
		seedInvite("jane@example.com", future, nil))

	_, items := getPending(t, r, "")

	if len(items) != 0 {
		t.Fatalf("an empty email claim matched %d invites; it must match none: %+v", len(items), items)
	}
}
