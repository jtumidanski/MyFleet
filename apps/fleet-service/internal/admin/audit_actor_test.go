package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/systemactor"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// FR-ADMIN-UI-13 as the console actually consumes it: AdminAuditPage compares
// actor_user_id against the word "system" to decide whether to render an email.
// The database stores a uuid, so the substitution has to happen here.
func TestTransformAuditEvents_rendersTheSystemSentinelAsAWord(t *testing.T) {
	events := []admin.AuditEvent{
		{ID: "a1", ActorUserID: systemactor.ID, ActorEmail: systemactor.Label, Action: admin.ActionPurgeReaped},
		{ID: "a2", ActorUserID: "7a186017-d27e-4d65-90e3-6b240bf9880a", ActorEmail: "op@example.com", Action: admin.ActionPurgeCreated},
	}
	out := admin.TransformAuditEvents(events)
	if len(out) != 2 {
		t.Fatalf("got %d resources, want 2", len(out))
	}

	got := actorOf(t, out[0])
	if got != systemactor.Label {
		t.Errorf("reaper row actor_user_id = %q, want %q", got, systemactor.Label)
	}
	if got := actorOf(t, out[1]); got != "7a186017-d27e-4d65-90e3-6b240bf9880a" {
		t.Errorf("human row actor_user_id = %q, want it untouched", got)
	}
}

func actorOf(t *testing.T, r server.Resource) string {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Attributes struct {
			ActorUserID string `json:"actor_user_id"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc.Attributes.ActorUserID
}

// The filter box on the audit page is free text, and it sits directly under a
// column that now reads "system". Typing that back in has to work — and has to
// reach the sentinel, since the column it filters on is a uuid.
func TestAuditEventsFilter_acceptsTheWordTheConsoleDisplays(t *testing.T) {
	if code := auditFilterStatus(t, systemactor.Label); code != http.StatusOK {
		t.Errorf("?actor=system returned %d, want 200", code)
	}
	if code := auditFilterStatus(t, systemactor.ID); code != http.StatusOK {
		t.Errorf("?actor=<sentinel> returned %d, want 200", code)
	}
	if code := auditFilterStatus(t, "7a186017-d27e-4d65-90e3-6b240bf9880a"); code != http.StatusOK {
		t.Errorf("?actor=<uuid> returned %d, want 200", code)
	}
}

// Anything else is the caller's mistake. It used to reach Postgres as a
// comparison against a uuid column and come back a 500.
func TestAuditEventsFilter_rejectsAnActorThatIsNotAnID(t *testing.T) {
	code := auditFilterStatus(t, "not-a-uuid")
	if code == http.StatusInternalServerError {
		t.Fatal("a malformed actor filter must not surface as a server error")
	}
	if code != http.StatusUnprocessableEntity {
		t.Errorf("?actor=not-a-uuid returned %d, want 422", code)
	}
}

func auditFilterStatus(t *testing.T, actor string) int {
	t.Helper()
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	r := chi.NewRouter()
	admin.InitializeRoutes(discardLogger(), newBrowseProcessor(t, db))(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-events?actor="+actor, strings.NewReader(""))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID: "admin-1", Email: "admin@example.com", PlatformAdmin: true,
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}
