package invite

import (
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// validBuilder returns a Builder with every invariant satisfied, so each case
// below can knock exactly one of them out and prove that field alone is
// load-bearing.
func validBuilder() *Builder {
	return NewBuilder().
		SetFleetID("fleet-1").
		SetEmail("jane@example.com").
		SetRole("member").
		SetToken("tok-1").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetInvitedByUserID("owner-1")
}

func TestBuild_acceptsACompleteInvite(t *testing.T) {
	m, err := validBuilder().Build()
	if err != nil {
		t.Fatalf("Build() err = %v, want nil", err)
	}
	if m.ID() == "" {
		t.Fatal("Build() produced a Model with no id")
	}
	if m.FleetID() != "fleet-1" || m.Email() != "jane@example.com" ||
		m.Role() != "member" || m.Token() != "tok-1" || m.InvitedByUserID() != "owner-1" {
		t.Fatalf("Build() lost a field: %+v", m)
	}
}

// Every field below is NOT NULL in fleet.fleet_invites and load-bearing at
// runtime: a blank token collides with every other blank token on the accept
// route, a blank fleet id mints a membership in nothing, a blank role is copied
// onto a membership no authz gate understands. Build must refuse all of them.
func TestBuild_rejectsAnInviteMissingAnInvariant(t *testing.T) {
	cases := []struct {
		name string
		b    *Builder
	}{
		{"no fleet id", validBuilder().SetFleetID("")},
		{"no email", validBuilder().SetEmail("")},
		{"no role", validBuilder().SetRole("")},
		{"no token", validBuilder().SetToken("")},
		{"no inviting user", validBuilder().SetInvitedByUserID("")},
		{"no expiry", validBuilder().SetExpiresAt(time.Time{})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := c.b.Build()
			if !errors.Is(err, server.ErrValidation) {
				t.Fatalf("Build() err = %v, want server.ErrValidation", err)
			}
			if m != (Model{}) {
				t.Fatalf("Build() returned %+v alongside an error; it must return the zero Model", m)
			}
		})
	}
}

// Build asserts an address is PRESENT, not that it is well-formed.
// ValidateInviteEmail owns syntax at the resource boundary; duplicating the
// addr-spec rule here would give the domain two sources of truth that can drift.
func TestBuild_doesNotRevalidateEmailSyntax(t *testing.T) {
	if _, err := validBuilder().SetEmail("Bob <bob@example.com>").Build(); err != nil {
		t.Fatalf("Build() err = %v; syntax is ValidateInviteEmail's job, not the builder's", err)
	}
	if err := ValidateInviteEmail("Bob <bob@example.com>"); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("ValidateInviteEmail must still own the syntax rule, got %v", err)
	}
}
