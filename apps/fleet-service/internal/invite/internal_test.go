package invite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

type stubFleetNamer struct {
	names map[string]string
	err   error
}

func (s stubFleetNamer) GetByID(id string) (fleet.Model, error) {
	if s.err != nil {
		return fleet.Model{}, s.err
	}
	name, ok := s.names[id]
	if !ok {
		return fleet.Model{}, fleet.ErrNotFound
	}
	// fleet.Builder.Build() returns (Model, error) since fleet-name
	// non-emptiness is enforced there; the stub names are always non-empty so
	// the error is never populated.
	m, err := fleet.NewBuilder().SetName(name).Build()
	if err != nil {
		return fleet.Model{}, err
	}
	return m, nil
}

func internalRouter(t *testing.T, db *gorm.DB, namer FleetNamer) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	InitializeInternalRoutes(logrus.New(), db, namer)(r)
	return r
}

func TestInternalGetInvite_returnsTokenAndFleetName(t *testing.T) {
	db := newInviteDB(t)
	adm := NewAdministrator(db)
	created, err := adm.Insert(context.Background(), newInvite(t, "f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{"f1": "The Smiths"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got InternalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.InviteID != created.ID() || got.FleetID != "f1" || got.FleetName != "The Smiths" {
		t.Fatalf("identity fields wrong: %+v", got)
	}
	// The whole point of this endpoint: the consumer cannot compose the email
	// without the token, and the token is deliberately absent from the event.
	if got.Token != "tok-1" {
		t.Fatalf("token=%q want tok-1", got.Token)
	}
	if got.Email != "a@b.com" || got.Role != "member" || got.InvitedByUserID != "user-1" {
		t.Fatalf("payload fields wrong: %+v", got)
	}
	if got.AcceptedAt != nil {
		t.Fatalf("accepted_at should be null, got %v", *got.AcceptedAt)
	}
	if _, err := time.Parse(time.RFC3339, got.ExpiresAt); err != nil {
		t.Fatalf("expires_at %q is not RFC3339: %v", got.ExpiresAt, err)
	}
}

func TestInternalGetInvite_unknownIDIs404(t *testing.T) {
	r := internalRouter(t, newInviteDB(t), stubFleetNamer{names: map[string]string{}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

// FR-INT-3: the row is returned even when accepted; the CALLER decides whether
// to send. Returning 404 here would make a stale redelivery indistinguishable
// from a deleted invite and cost the consumer four pointless retries.
func TestInternalGetInvite_returnsAcceptedInvites(t *testing.T) {
	db := newInviteDB(t)
	adm := NewAdministrator(db)
	created, err := adm.Insert(context.Background(), newInvite(t, "f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	accepted := time.Now().UTC()
	if err := db.Model(&Entity{}).Where("id = ?", created.ID()).Update("accepted_at", &accepted).Error; err != nil {
		t.Fatalf("stamp accepted_at: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{"f1": "The Smiths"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var got InternalResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.AcceptedAt == nil {
		t.Fatal("accepted_at must be populated")
	}
}

// A missing fleet row degrades to an empty name rather than a 500 — the email
// then falls back to a generic subject (design §4.5).
func TestInternalGetInvite_missingFleetDegradesToEmptyName(t *testing.T) {
	db := newInviteDB(t)
	created, err := NewAdministrator(db).Insert(context.Background(), newInvite(t, "f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var got InternalResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.FleetName != "" {
		t.Fatalf("fleet_name=%q want empty", got.FleetName)
	}
}

// FR-INT-4. The tree is WALKED rather than probed with one URL, so a future
// internal route registered on the wrong initializer also fails here.
func TestInternalRouteAbsentFromJWTTree(t *testing.T) {
	db := newInviteDB(t)
	r := chi.NewRouter()
	InitializeRoutes(logrus.New(), db, stubOwnerChecker{}, nil, nil, nil, Limits{})(r)

	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.Contains(route, "internal") {
			t.Errorf("JWT-protected tree registers an internal route: %s %s", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

type stubOwnerChecker struct{}

func (stubOwnerChecker) RequireOwnerInFleet(string, string) error { return nil }
