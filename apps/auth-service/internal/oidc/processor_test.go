package oidc

import "testing"

func TestProfileFromClaims_extractsFields(t *testing.T) {
	gp := profileFromClaims(map[string]any{
		"sub": "g-123", "email": "a@b.com", "name": "Ann", "picture": "http://x/y.png",
		"email_verified": true,
	})
	if gp.Sub != "g-123" || gp.Email != "a@b.com" || gp.Name != "Ann" {
		t.Fatalf("bad profile: %+v", gp)
	}
	if !gp.EmailVerified {
		t.Errorf("EmailVerified = false, want true")
	}
}

// A missing email_verified claim must map to false, not true — the grant
// gate in user.Processor.maybeGrantAdmin treats "unproven" as the safe
// default, so an absent claim must never be treated as an assertion.
func TestProfileFromClaims_missingEmailVerifiedDefaultsFalse(t *testing.T) {
	gp := profileFromClaims(map[string]any{"sub": "g-123", "email": "a@b.com"})
	if gp.EmailVerified {
		t.Errorf("EmailVerified = true for a claim set with no email_verified key, want false")
	}
}
