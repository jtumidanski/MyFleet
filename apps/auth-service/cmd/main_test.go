package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/membership"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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

func (f *fakeUsers) ListByIDs([]string) ([]user.Model, error) {
	return nil, nil
}

func usersWith(id, email string) *fakeUsers {
	return &fakeUsers{byID: map[string]user.Model{
		id: user.NewBuilder().SetEmail(email).Build(),
	}}
}

// fakeAdmins is a platformadmin.Provider stand-in so the resolver's third
// source of identity can be driven without a database.
type fakeAdmins struct {
	isAdmin bool
	err     error
}

func (f *fakeAdmins) IsAdmin(string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.isAdmin, nil
}

func (f *fakeAdmins) IsRevoked(string) (bool, error) { return false, nil }

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
//
// Table-driven over both true AND false admin answers (not just true): a
// version of this test that only ever fed the provider `true` would stay green
// even if newPrincipalResolver dropped the `isAdmin` variable entirely and
// hardcoded `PlatformAdmin: true` — minting the platform tier for every user
// on the system. This is the only place that decides the value, so it is the
// only place that can catch that regression; the mint layer
// (session.TestMintAccess_setsPlatformAdminClaimAsBoolean) and the middleware
// (TestJWT_parsesPlatformAdminClaim) both already cover true and false, but
// neither of them can see whether the VALUE fed into them was ever computed
// correctly in the first place.
func TestNewPrincipalResolver_fillsEveryField(t *testing.T) {
	for _, isAdmin := range []bool{true, false} {
		t.Run(fmt.Sprintf("isAdmin=%v", isAdmin), func(t *testing.T) {
			resolve := newPrincipalResolver(
				usersWith("user-1", "a@b.com"),
				fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
				&fakeAdmins{isAdmin: isAdmin},
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
			if p.PlatformAdmin != isAdmin {
				t.Fatalf("PlatformAdmin = %v, want %v — the resolver must carry the provider's "+
					"exact answer, not a hardcoded value", p.PlatformAdmin, isAdmin)
			}
		})
	}
}

// TestNewPrincipalResolver_leavesNoPrincipalFieldEmpty is the same guarantee as
// the test above, expressed so that nobody has to remember to update it. It
// deliberately names NO field: an enumerating test only covers the fields
// somebody thought to enumerate.
//
// The scenario it closes: someone adds Principal.TenantID and wires it into
// MintAccess — so session's TestMintAccess_mapsEveryPrincipalField stays green
// — but forgets the literal in newPrincipalResolver. Every token then carries
// "tenant_id": "", exactly the defect this branch fixed, and every other test
// in the repository stays green.
//
// Sibling guards, each covering a link this one does not:
//   - session.TestMintAccess_mapsEveryPrincipalField — every field reaches a claim.
//   - arch.TestNoPrincipalLiteralOutsideResolver — no other site builds a Principal.
//   - TestNewPrincipalResolver_fillsEveryField above — pins the actual VALUES,
//     which a zero-value check cannot; a resolver that filled every field with
//     the wrong data would satisfy this test and fail that one.
//
// Both inputs are fully populated, so after resolution every field must be
// non-empty. A field that is still zero was dropped by the literal.
func TestNewPrincipalResolver_leavesNoPrincipalFieldEmpty(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
		&fakeAdmins{isAdmin: true},
	)

	p, err := resolve(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	v := reflect.ValueOf(p)
	var boolFields int
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			if v.Field(i).String() == "" {
				t.Errorf("session.Principal.%s is empty after resolution — newPrincipalResolver's "+
					"literal does not set it, so every token this service mints will carry an "+
					"empty claim for it.", f.Name)
			}
		case reflect.Bool:
			// A bool has no "empty" value to infer dropped-ness from, so this
			// test only asserts it was actually threaded through: the fake
			// admin provider above reports true, and the resolved principal
			// must agree.
			boolFields++
			if !v.Field(i).Bool() {
				t.Errorf("session.Principal.%s is false after resolution even though the "+
					"admin lookup reported true — newPrincipalResolver's literal does not set it.",
					f.Name)
			}
		default:
			// Not a skip: an unhandled field kind means this test's scheme no
			// longer holds, and it must say so rather than quietly cover less
			// than it appears to.
			t.Fatalf("session.Principal.%s is a %s — this test infers 'dropped' from the "+
				"zero value, so extend its scheme before adding a field of another kind",
				f.Name, f.Type.Kind())
		}
	}
	if boolFields != 1 {
		t.Fatalf("session.Principal now has %d bool fields, want exactly 1 — extend this "+
			"test's scheme, which can only attribute a single bool to the fake admin provider",
			boolFields)
	}
}

// TestNewPrincipalResolver_failsClosedWhenTheUserIsGone covers FR-5: a refresh
// token can outlive its user row. Returning an error here is what makes the
// handler answer 401 instead of minting a token with absent identity.
func TestNewPrincipalResolver_failsClosedWhenTheUserIsGone(t *testing.T) {
	resolve := newPrincipalResolver(
		&fakeUsers{byID: map[string]user.Model{}},
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
		&fakeAdmins{},
	)

	if _, err := resolve(context.Background(), "ghost"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want user.ErrNotFound", err)
	}
}

func TestNewPrincipalResolver_failsClosedWhenTheMembershipCallFails(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `not json`),
		&fakeAdmins{},
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
		&fakeAdmins{},
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
		&fakeAdmins{isAdmin: true},
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
	// FR-ADMIN-AUTH-9: an admin with no fleet is a normal state, so the
	// platform tier must survive resolution even when ActiveFleetID is empty.
	if !p.PlatformAdmin {
		t.Fatalf("PlatformAdmin = %v, want true — an admin with no fleet is still an admin", p.PlatformAdmin)
	}
}

// TestNewPrincipalResolver_failsClosedWhenTheAdminLookupFails covers the third
// source of identity added for FR-ADMIN-AUTH-4: a lookup error must not mint a
// token that silently claims false, because the console's absence would then
// read as "you are not an admin" rather than "we could not tell".
func TestNewPrincipalResolver_failsClosedWhenTheAdminLookupFails(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
		&fakeAdmins{err: errors.New("db unavailable")},
	)

	if _, err := resolve(context.Background(), "user-1"); err == nil {
		t.Fatal("an admin lookup failure must fail closed, not mint a principal with PlatformAdmin=false")
	}
}

// TestNewPrincipalResolver_carriesTheTransientClassificationThrough is a
// regression test, not a test of new code: main.go returns the membership
// error bare today, so errors.Is already reaches through it. The risk is
// tomorrow's edit — someone annotating it with fmt.Errorf("...: %v", err)
// silently breaks classification for BOTH handlers, every transient failure
// becomes a logout again, and nothing else in the repository goes red.
func TestNewPrincipalResolver_carriesTheTransientClassificationThrough(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusInternalServerError, `{"errors":[{"status":"500"}]}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if err == nil {
		t.Fatal("a 500 from fleet-service must not resolve to a principal")
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("classification did not survive the resolver: %v — both handlers "+
			"branch on errors.Is, so a %%v wrap here turns every outage back into a logout", err)
	}
}

// TestNewPrincipalResolver_keepsAMissingUserPermanent is the other half. A user
// row that is gone is a DEAD credential: it must reach the 401-and-clear path,
// not the 503 that invites the browser to try again forever.
func TestNewPrincipalResolver_keepsAMissingUserPermanent(t *testing.T) {
	resolve := newPrincipalResolver(
		&fakeUsers{byID: map[string]user.Model{}},
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want user.ErrNotFound", err)
	}
	if errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a missing user row classified as transient: %v — the credential is dead and the session must end", err)
	}
}

// TestNewPrincipalResolver_keepsAPermanentUpstreamFaultPermanent: a 403 from
// fleet-service is a misconfiguration between the two services, not an outage.
// Retrying cannot fix it, so it keeps today's fail-closed behaviour.
func TestNewPrincipalResolver_keepsAPermanentUpstreamFaultPermanent(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusForbidden, `{"errors":[{"status":"403"}]}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if err == nil {
		t.Fatal("a 403 must not resolve to a principal")
	}
	if errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a 403 classified as transient: %v", err)
	}
}

// TestNewPrincipalResolver_classifiesLocalInfrastructureFailuresAsTransient
// (design D9). The resolver reads three sources; only one of them is
// fleet-service. An auth-Postgres blip lasting longer than one request logs out
// every active session through this same closure, for the same reason. The
// driver's text is deliberately dropped rather than %w-wrapped so a SQLSTATE or
// table name cannot ride into a log line even in principle.
func TestNewPrincipalResolver_classifiesLocalInfrastructureFailuresAsTransient(t *testing.T) {
	dbDown := errors.New(`pq: relation "auth.users" does not exist (SQLSTATE 42P01)`)

	cases := map[string]func() (user.Provider, *fakeAdmins){
		"users read fails": func() (user.Provider, *fakeAdmins) {
			return &fakeUsers{err: dbDown}, &fakeAdmins{isAdmin: false}
		},
		"platform admin read fails": func() (user.Provider, *fakeAdmins) {
			return usersWith("user-1", "a@b.com"), &fakeAdmins{err: dbDown}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			users, admins := setup()
			resolve := newPrincipalResolver(
				users,
				fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
				admins,
			)

			_, err := resolve(context.Background(), "user-1")

			if err == nil {
				t.Fatal("a failed local read must not resolve to a principal")
			}
			if !errors.Is(err, server.ErrServiceUnavailable) {
				t.Fatalf("err = %v, want a transient classification — a database hiccup must not "+
					"log every active session out", err)
			}
			for _, secret := range []string{"SQLSTATE", "42P01", "auth.users", "pq:"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("the error carries %q from the driver: %q", secret, err)
				}
			}
		})
	}
}
