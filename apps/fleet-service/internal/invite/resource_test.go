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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// stubOwnerChecker satisfies OwnerChecker. The accept route never consults it —
// the invite token is what authorizes acceptance — but InitializeRoutes wires
// it for the create/delete routes.
type stubOwnerChecker struct{}

func (stubOwnerChecker) RequireOwnerInFleet(string, string) error { return nil }

func newInviteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.fleet_invites) for Postgres. SQLite
	// has no schemas, so attach an in-memory database aliased "fleet" so the
	// qualified name resolves.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// Migration(db) (AutoMigrate) fails here: GORM's SQLite driver emits
	// CREATE INDEX with the schema prefix stripped, so the unique index on
	// Entity.Token can't find fleet.fleet_invites (SQLite resolves an
	// unqualified table name against "main", not "fleet"). The schema-qualified
	// table is created with explicit DDL instead, mirroring the same
	// workaround used in maintenancerecord/provider_test.go,
	// maintenanceschedule/completion_db_test.go, dashboard/aggregate_test.go,
	// and activity/processor_test.go.
	ddl := `CREATE TABLE fleet.fleet_invites (
		id TEXT PRIMARY KEY, fleet_id TEXT, email TEXT, role TEXT, token TEXT,
		expires_at DATETIME, accepted_at DATETIME, invited_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// newAcceptRouter builds the real chi router over a seeded database. The
// activity recorder and outbox emitter are nil: every case here fails
// validation before Administrator.Accept runs, so neither is reached.
func newAcceptRouter(t *testing.T, inv Model) chi.Router {
	t.Helper()
	db := newInviteTestDB(t)
	if _, err := NewAdministrator(db).Insert(inv); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil))
	return r
}

// postAccept drives POST /invites/{token}/accept with a validated Identity on
// context, standing in for the JWT middleware the real router mounts upstream.
func postAccept(r chi.Router, token, authedEmail string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/invites/"+token+"/accept", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "user-1",
		Email:         authedEmail,
		ActiveFleetID: "fleet-1",
		Role:          "member",
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeDetail(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(body.Errors))
	}
	if body.Errors[0].Status != "409" {
		t.Fatalf("status field = %q, want 409", body.Errors[0].Status)
	}
	if body.Errors[0].Title != "conflict" {
		t.Fatalf("title = %q, want conflict — the status contract must not change", body.Errors[0].Title)
	}
	return body.Errors[0].Detail
}

// seedInvite uses the builder's unexported setAcceptedAt, which exists for
// exactly this purpose: production code stamps accepted_at inside
// Administrator.Accept's transaction, never by hand.
func seedInvite(email string, expires time.Time, accepted *time.Time) Model {
	return NewBuilder().
		SetFleetID("fleet-1").
		SetEmail(email).
		SetRole("member").
		SetToken("tok-" + email).
		SetExpiresAt(expires).
		SetInvitedByUserID("owner-1").
		setAcceptedAt(accepted).
		Build()
}

// TestAcceptRoute_rendersADistinctDetailPerPrecondition is the user-facing half
// of this task: before it, all three of these returned a body a caller could
// not tell apart, so an invite rejected because the session had refreshed
// looked exactly like one already accepted.
func TestAcceptRoute_rendersADistinctDetailPerPrecondition(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		inv        Model
		as         string
		wantDetail string
	}{
		{"already accepted", seedInvite("a@b.com", now.Add(time.Hour), &now), "a@b.com", "invite has already been accepted"},
		{"expired", seedInvite("a@b.com", now.Add(-time.Hour), nil), "a@b.com", "invite has expired"},
		{"email mismatch", seedInvite("invited@b.com", now.Add(time.Hour), nil), "other@b.com", "invite was issued to a different account"},
		// A corrupt row: unreachable through the create endpoint, and the one
		// case that used to be accepted rather than rejected when the caller's
		// email claim was also empty.
		{"invite row has no email", seedInvite("", now.Add(time.Hour), nil), "", "invite cannot be accepted"},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newAcceptRouter(t, c.inv)
			rec := postAccept(r, c.inv.Token(), c.as)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (the status contract does not change)", rec.Code)
			}
			got := decodeDetail(t, rec)
			if got != c.wantDetail {
				t.Fatalf("detail = %q, want %q", got, c.wantDetail)
			}
			if seen[got] {
				t.Fatalf("detail %q is not unique across preconditions", got)
			}
			seen[got] = true
		})
	}
}

// TestAcceptRoute_mismatchDetailDisclosesNeitherAddress enforces FR-10. The
// invite token is the only credential needed to reach this endpoint, so naming
// the invited address here would turn a leaked invite link into an
// address-disclosure oracle.
func TestAcceptRoute_mismatchDetailDisclosesNeitherAddress(t *testing.T) {
	inv := seedInvite("invited@example.com", time.Now().Add(time.Hour), nil)
	r := newAcceptRouter(t, inv)
	rec := postAccept(r, inv.Token(), "attacker@example.com")

	body := rec.Body.String()
	if strings.Contains(body, "invited@example.com") {
		t.Fatalf("409 body discloses the invited address: %s", body)
	}
	if strings.Contains(body, "attacker@example.com") {
		t.Fatalf("409 body echoes the authenticated address: %s", body)
	}
}
