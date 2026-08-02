package invite

import (
	"bytes"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
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
	if _, err := NewAdministrator(db).Insert(context.Background(), inv, "trace-test"); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil, nil, Limits{CreatePerWindow: 1000, CreateWindow: time.Hour}))
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

// seedInvite builds a row to be written straight to the table, so it goes
// through Make rather than the Builder. Two reasons, both structural:
//
//   - accepted_at is stamped inside Administrator.Accept's transaction, never by
//     hand, so the Builder has no setter for it — and should not grow one.
//   - one case below is a deliberately CORRUPT row (blank email) that
//     Builder.Build now rejects at construction. It can only exist as a row read
//     back from the database, which is exactly what Make represents.
func seedInvite(email string, expires time.Time, accepted *time.Time) Model {
	return Make(Entity{
		ID:              uuid.NewString(),
		FleetID:         "fleet-1",
		Email:           email,
		Role:            "member",
		Token:           "tok-" + email,
		ExpiresAt:       expires,
		AcceptedAt:      accepted,
		InvitedByUserID: "owner-1",
	})
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

// TestResendInvite_acceptedIs409EvenInsideTheCooldown pins FR-RSND-3's
// ordering at the HTTP layer. The invite was just created, so its updated_at is
// well inside the one-hour cooldown and a naive implementation would answer
// 429. It must answer 409: an accepted invite can never satisfy a cooldown, so
// reporting one would tell the caller to come back later for something that
// will never work.
func TestResendInvite_acceptedIs409EvenInsideTheCooldown(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour, ResendCooldown: time.Hour})

	id := createInvite(t, r, "f1", "a@example.com", "member")
	accepted := time.Now().UTC()
	if err := db.Model(&Entity{}).Where("id = ?", id).Update("accepted_at", &accepted).Error; err != nil {
		t.Fatalf("stamp accepted_at: %v", err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f1/invites/"+id+"/resend", "f1", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("resend of an accepted invite status = %d, want 409 (not the 429 the "+
			"fresh updated_at would produce); body: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateInvite_unknownRoleIs422 pins the role-vocabulary check, which moved
// from the handler into Processor.Create. An unrecognised role would otherwise
// be copied verbatim onto the membership minted at accept time.
func TestCreateInvite_unknownRoleIs422(t *testing.T) {
	db := newInviteDB(t)
	r := inviteTestRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodPost, "/fleets/f1/invites", "f1", createInviteBody("a@example.com", "wizard")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown role status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	if n := countRows(t, db, &Entity{}); n != 0 {
		t.Fatalf("an invite row was written for an unknown role (%d rows)", n)
	}
}

// loggingRouter mounts the real routes with the correlation-id middleware in
// front, and captures everything the handlers log as raw JSON.
//
// The buffer is scanned as TEXT rather than field-by-field on purpose. A
// field-by-field assertion has to decide how to stringify each value, and any
// value it does not know how to render — a struct, a fmt.Stringer — silently
// reads as empty and the assertion passes vacuously. Scanning the serialized
// output cannot be fooled that way: if the token reached the log at all, in any
// field, of any type, it is in these bytes.
func loggingRouter(t *testing.T, db *gorm.DB, limits Limits) (chi.Router, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	log := logrus.New()
	log.SetOutput(buf)
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetLevel(logrus.DebugLevel)

	r := chi.NewRouter()
	r.Use(telemetry.CorrelationID)
	InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil, nil, limits)(r)
	InitializeInternalRoutes(log, db, stubFleetNamer{names: map[string]string{}})(r)
	return r, buf
}

// TestInviteHandlers_neverLogTheTokenAndAlwaysCarryTheCorrelationID is the
// standing guard for the two log disciplines this domain has to hold.
//
// The invite token is a bearer credential — it is the entire authority the
// accept route checks — so it must never reach a log message or a log field,
// however the surrounding code is rearranged. And every unexpected failure must
// carry the correlation id, or an operator holding one end of a user report has
// no way to find the other.
//
// Every route below is driven against a table that has been dropped out from
// under it, which is what forces the unexpected-error branches to run.
func TestInviteHandlers_neverLogTheTokenAndAlwaysCarryTheCorrelationID(t *testing.T) {
	db := newInviteDB(t)
	r, buf := loggingRouter(t, db, Limits{CreatePerWindow: 100, CreateWindow: time.Hour})

	// A real invite, so the token under test is one the domain actually minted.
	id := createInvite(t, r, "f1", "a@example.com", "member")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, ownerRequest(http.MethodGet, "/fleets/f1/invites", "f1", nil))
	var list struct {
		Data []struct {
			Attributes struct {
				Token string `json:"token"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Data) != 1 {
		t.Fatalf("list invites to recover the token: %v (%s)", err, rec.Body.String())
	}
	token := list.Data[0].Attributes.Token
	if token == "" {
		t.Fatal("no token to test with")
	}

	// Pull the table out from under every handler so the unexpected-error
	// branches — the ones that log — actually run.
	if err := db.Exec("DROP TABLE fleet.fleet_invites").Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	buf.Reset()

	requests := []*http.Request{
		ownerRequest(http.MethodGet, "/fleets/f1/invites", "f1", nil),
		ownerRequest(http.MethodDelete, "/invites/"+id, "f1", nil),
		ownerRequest(http.MethodPost, "/fleets/f1/invites/"+id+"/resend", "f1", nil),
		ownerRequest(http.MethodPost, "/fleets/f1/invites", "f1", createInviteBody("b@example.com", "member")),
		httptest.NewRequest(http.MethodGet, "/internal/invites/"+id, nil),
	}
	for _, req := range requests {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
	// The accept route, driven with the real token in the URL — the one place a
	// careless log call would pick it up straight off the request.
	postAccept(r, token, "a@example.com")

	out := buf.String()
	if out == "" {
		t.Fatal("no log output at all; the assertions below would pass vacuously")
	}
	if strings.Contains(out, token) {
		t.Fatalf("the invite token reached the logs:\n%s", out)
	}

	// Every line must be joinable to the request that produced it.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		cid, ok := entry["correlation_id"].(string)
		if !ok || cid == "" {
			t.Fatalf("log line carries no correlation_id: %q", line)
		}
	}
}

// The accept route's own rejection lines are the ones most likely to grow an
// address: they are about WHO the invite was for. Neither address may appear,
// and the line must still be joinable to the request.
func TestAcceptRoute_mismatchLogDisclosesNoAddressButKeepsTheCorrelationID(t *testing.T) {
	db := newInviteTestDB(t)
	inv := seedInvite("invited@example.com", time.Now().Add(time.Hour), nil)
	if _, err := NewAdministrator(db).Insert(context.Background(), inv, "trace-test"); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	buf := &bytes.Buffer{}
	log := logrus.New()
	log.SetOutput(buf)
	log.SetFormatter(&logrus.JSONFormatter{})

	r := chi.NewRouter()
	r.Use(telemetry.CorrelationID)
	InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil, nil, Limits{CreatePerWindow: 100, CreateWindow: time.Hour})(r)

	rec := postAccept(r, inv.Token(), "attacker@example.com")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}

	out := buf.String()
	if !strings.Contains(out, "email mismatch") {
		t.Fatalf("the mismatch was not logged at all; operators need it greppable:\n%s", out)
	}
	for _, forbidden := range []string{inv.Token(), "invited@example.com", "attacker@example.com"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("log discloses %q:\n%s", forbidden, out)
		}
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v", err)
	}
	if cid, _ := entry["correlation_id"].(string); cid == "" {
		t.Fatalf("mismatch log carries no correlation_id: %s", out)
	}
	if entry["invite_id"] != inv.ID() {
		t.Fatalf("mismatch log invite_id = %v, want %q", entry["invite_id"], inv.ID())
	}
}
