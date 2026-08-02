package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/membership"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

// fakeUsers is a user.Provider over a map. GetBySub returns a loud error rather
// than data: the resolver must look users up by our internal id, and calling
// GetBySub with a JWT `sub` was the FIRST identity-propagation defect in this
// flow — it silently returned ErrNotFound and logged every user back out.
type fakeUsers struct {
	byID map[string]user.Model
	err  error
}

func (f *fakeUsers) GetByID(id string) (user.Model, error) {
	if f.err != nil {
		return user.Model{}, f.err
	}
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return user.Model{}, user.ErrNotFound
}

func (f *fakeUsers) GetBySub(string) (user.Model, error) {
	return user.Model{}, errors.New("resolver must look users up by internal id, not google_sub")
}

func usersWith(id, email string) *fakeUsers {
	return &fakeUsers{byID: map[string]user.Model{
		id: user.NewBuilder().SetEmail(email).Build(),
	}}
}

// fleetServing stands up a fake fleet-service and returns a membership.Client
// pointed at it, so the real HTTP client — including its 404 handling — is
// under test rather than a stand-in for it.
func fleetServing(t *testing.T, status int, body string) *membership.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return membership.NewClient(srv.URL)
}

// TestNewPrincipalResolver_fillsEveryField is the core guarantee: this closure
// is the only place a session.Principal is built, so if it fills every field,
// every token this service mints on either path carries a complete claim set.
func TestNewPrincipalResolver_fillsEveryField(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
	)

	p, err := resolve(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", p.UserID)
	}
	if p.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com — this is the field the refresh path used to drop", p.Email)
	}
	if p.ActiveFleetID != "fleet-9" {
		t.Fatalf("ActiveFleetID = %q, want fleet-9", p.ActiveFleetID)
	}
	if p.Role != "owner" {
		t.Fatalf("Role = %q, want owner", p.Role)
	}
}

// TestNewPrincipalResolver_failsClosedWhenTheUserIsGone covers FR-5: a refresh
// token can outlive its user row. Returning an error here is what makes the
// handler answer 401 instead of minting a token with absent identity.
func TestNewPrincipalResolver_failsClosedWhenTheUserIsGone(t *testing.T) {
	resolve := newPrincipalResolver(
		&fakeUsers{byID: map[string]user.Model{}},
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
	)

	if _, err := resolve(context.Background(), "ghost"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want user.ErrNotFound", err)
	}
}

func TestNewPrincipalResolver_failsClosedWhenTheMembershipCallFails(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `not json`),
	)

	if _, err := resolve(context.Background(), "user-1"); err == nil {
		t.Fatal("a membership decode failure must fail closed, not mint a partial principal")
	}
}

// TestNewPrincipalResolver_failsClosedWhenFleetServiceErrors is the resolver
// half of the membership client's non-2xx guard, and the reason that guard
// matters. fleet-service's error envelope is JSON, so a 500 from
// /internal/memberships/active used to decode into a zero Membership with no
// error: the resolver returned a Principal with an empty ActiveFleetID and no
// error, a valid token was minted claiming the user has no fleet, and the SPA
// redirected them to /onboarding to create a duplicate one.
//
// Compare with TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError
// below: an empty ActiveFleetID is a legitimate answer for 404 and ONLY for
// 404. Anything else must fail closed, which is what the comment on
// session/resource.go promises.
func TestNewPrincipalResolver_failsClosedWhenFleetServiceErrors(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		// A well-formed JSON error envelope, not garbage: garbage already
		// failed the decode. This is the body that used to slip through.
		fleetServing(t, http.StatusInternalServerError,
			`{"errors":[{"status":"500","code":"internal","title":"internal server error"}]}`),
	)

	p, err := resolve(context.Background(), "user-1")
	if err == nil {
		t.Fatalf("a fleet-service 500 must fail closed; got principal %+v with no error", p)
	}
}

// TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError is load-bearing
// and easy to break. membership.Client maps a 404 to a zero Membership with no
// error, and the OIDC callback keys its onboarding redirect off an empty
// ActiveFleetID. Turning "no membership" into a resolver error would break
// signup on a brand-new user's first login.
func TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusNotFound, ``),
	)

	p, err := resolve(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("a user with no fleet must resolve cleanly, got %v", err)
	}
	if p.ActiveFleetID != "" || p.Role != "" {
		t.Fatalf("fleet/role = %q/%q, want empty so the callback redirects to onboarding", p.ActiveFleetID, p.Role)
	}
	if p.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com even with no membership", p.Email)
	}
}
