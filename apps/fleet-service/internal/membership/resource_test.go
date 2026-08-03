package membership

import (
	"encoding/json"
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

// newMemberRouter builds the real chi router over a seeded in-memory database
// and returns both, so a test can drive a request and then read the row back
// rather than trusting the response echo.
func newMemberRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newMembershipDB(t)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	// nil recorder: these tests assert authorization and validation, and the
	// activity path has its own coverage in administrator_db_test.go.
	r.Group(InitializeRoutes(log, db, nil))
	return r, db
}

// serveAs drives one request with a validated Identity on context, standing in
// for the JWT middleware the real router mounts upstream.
func serveAs(r chi.Router, method, path, body string, id auth.Identity) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func identity(userID, role, fleetID string) auth.Identity {
	return auth.Identity{UserID: userID, Email: userID + "@example.com", ActiveFleetID: fleetID, Role: role}
}

func patchBody(role string) string {
	return `{"data":{"type":"memberships","attributes":{"role":"` + role + `"}}}`
}

func patchRole(r chi.Router, fleetID, targetUserID, role string, id auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPatch, "/fleets/"+fleetID+"/members/"+targetUserID, patchBody(role), id)
}

func deleteMember(r chi.Router, fleetID, targetUserID string, id auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodDelete, "/fleets/"+fleetID+"/members/"+targetUserID, "", id)
}

func decodeDetail(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Errors []struct {
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if len(env.Errors) == 0 {
		return ""
	}
	return env.Errors[0].Detail
}

// --- PATCH ----------------------------------------------------------------

// FR-2.5: promoting does not demote the promoter. A fleet may hold any number
// of owners.
func TestPatchRole_promotesAMemberAndLeavesThePromoterAnOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")
	target := seedMembership(t, db, "u-member", "member")

	rec := patchRole(r, "f1", "u-member", "owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("target role = %q, want owner", got)
	}
	if got := readRole(t, db, owner.ID()); got != "owner" {
		t.Fatalf("promoter role = %q, want owner — promotion must not demote", got)
	}
	if !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Errorf("response must carry the updated membership; got %s", rec.Body.String())
	}
}

func TestPatchRole_forbiddenForNonOwners(t *testing.T) {
	for _, role := range []string{"member", "viewer"} {
		r, db := newMemberRouter(t)
		seedMembership(t, db, "u-actor", role)
		seedMembership(t, db, "u-target", "member")

		rec := patchRole(r, "f1", "u-target", "owner", identity("u-actor", role, "f1"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("PATCH as %s = %d, want 403", role, rec.Code)
		}
	}
}

// SEC-5. role is a JWT claim minted at login; the database is the authority.
// A token still claiming owner after a demotion must be rejected.
func TestPatchRole_forbiddenWhenTheOwnerClaimIsStale(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-actor", "member") // DB says member...
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "owner", identity("u-actor", "owner", "f1")) // ...token says owner
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stale owner claim = %d, want 403", rec.Code)
	}
}

// FR-2.3. The message names the field and the allow-list without echoing the
// caller's input.
func TestPatchRole_rejectsAnUnknownRoleWithTheAllowList(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "admin", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH role=admin = %d, want 422", rec.Code)
	}
	detail := decodeDetail(t, rec)
	for _, want := range []string{"role", "owner", "member", "viewer"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q must name %q", detail, want)
		}
	}
	if strings.Contains(detail, "admin") {
		t.Errorf("detail %q echoes the caller's input; the message must be a constant", detail)
	}
}

func TestPatchRole_notFoundWhenTheTargetIsNotAMember(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")

	rec := patchRole(r, "f1", "u-stranger", "owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH against a non-member = %d, want 404", rec.Code)
	}
}

// FR-2.6.
func TestPatchRole_conflictWhenDemotingTheSoleOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")

	rec := patchRole(r, "f1", "u-owner", "member", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("demoting the sole owner = %d, want 409", rec.Code)
	}
	if got := readRole(t, db, owner.ID()); got != "owner" {
		t.Fatalf("role = %q after a rejected demotion, want owner", got)
	}
}

// FR-2.7.
func TestPatchRole_noOpSucceeds(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	target := seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "member", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("no-op PATCH = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := readRole(t, db, target.ID()); got != "member" {
		t.Fatalf("role = %q, want member", got)
	}
}

// RequireSameFleet runs first, so a cross-fleet request 404s before any role
// check can leak whether the fleet or the target exists.
func TestPatchRole_notFoundAcrossFleets(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "owner", identity("u-owner", "owner", "other-fleet"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-fleet PATCH = %d, want 404", rec.Code)
	}
}

// --- DELETE ---------------------------------------------------------------

// FR-3.1. Before this change the endpoint required owner at both layers, so a
// member or viewer had no way to leave a fleet at all.
func TestDeleteMember_selfRemovalIsAllowedForEveryRole(t *testing.T) {
	for _, role := range []string{"member", "viewer", "owner"} {
		r, db := newMemberRouter(t)
		// A co-owner so the "owner" case is not blocked by the sole-owner guard.
		seedMembership(t, db, "u-other-owner", "owner")
		self := seedMembership(t, db, "u-self", role)

		rec := deleteMember(r, "f1", "u-self", identity("u-self", role, "f1"))
		if rec.Code != http.StatusNoContent {
			t.Errorf("self-leave as %s = %d, want 204. Body: %s", role, rec.Code, strings.TrimSpace(rec.Body.String()))
			continue
		}
		if countRows(t, db, self.ID()) != 0 {
			t.Errorf("membership row still present after self-leave as %s", role)
		}
	}
}

// SEC-4. The relaxed branch must apply ONLY when the actor names their own row.
func TestDeleteMember_forbiddenWhenANonOwnerRemovesSomeoneElse(t *testing.T) {
	for _, role := range []string{"member", "viewer"} {
		r, db := newMemberRouter(t)
		seedMembership(t, db, "u-actor", role)
		other := seedMembership(t, db, "u-other", "member")

		rec := deleteMember(r, "f1", "u-other", identity("u-actor", role, "f1"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s removing another member = %d, want 403", role, rec.Code)
		}
		if countRows(t, db, other.ID()) != 1 {
			t.Errorf("%s deleted another member's row; self-leave must not be a privilege escalation", role)
		}
	}
}

// FR-3.2: the sole-owner guard is unchanged and still reachable.
func TestDeleteMember_conflictWhenTheSoleOwnerLeaves(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")

	rec := deleteMember(r, "f1", "u-owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("sole-owner self-leave = %d, want 409", rec.Code)
	}
	if countRows(t, db, owner.ID()) != 1 {
		t.Fatal("sole owner's row was deleted despite the 409")
	}
}

// FR-3.3: an owner may remove another owner. This cannot orphan the fleet —
// the actor is themselves an owner and remains.
func TestDeleteMember_ownerCanRemoveAnotherOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner-a", "owner")
	b := seedMembership(t, db, "u-owner-b", "owner")

	rec := deleteMember(r, "f1", "u-owner-b", identity("u-owner-a", "owner", "f1"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("owner removing a co-owner = %d, want 204", rec.Code)
	}
	if countRows(t, db, b.ID()) != 0 {
		t.Fatal("co-owner's row still present")
	}
}

// RequireSameFleet stays OUTSIDE the isSelf branch: identity.UserID ==
// targetUserID is necessary but not sufficient.
func TestDeleteMember_selfRemovalDoesNotBypassTheSameFleetCheck(t *testing.T) {
	r, db := newMemberRouter(t)
	self := seedMembership(t, db, "u-self", "member")

	rec := deleteMember(r, "f1", "u-self", identity("u-self", "member", "other-fleet"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-fleet self-leave = %d, want 404", rec.Code)
	}
	if countRows(t, db, self.ID()) != 1 {
		t.Fatal("row deleted through a fleet the actor is not scoped to")
	}
}

func TestDeleteMember_notFoundWhenTheTargetIsNotAMember(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")

	rec := deleteMember(r, "f1", "u-stranger", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE against a non-member = %d, want 404", rec.Code)
	}
}
