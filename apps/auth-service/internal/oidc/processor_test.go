package oidc

import "testing"

func TestProfileFromClaims_extractsFields(t *testing.T) {
	gp := profileFromClaims(map[string]any{
		"sub": "g-123", "email": "a@b.com", "name": "Ann", "picture": "http://x/y.png",
	})
	if gp.Sub != "g-123" || gp.Email != "a@b.com" || gp.Name != "Ann" {
		t.Fatalf("bad profile: %+v", gp)
	}
}
