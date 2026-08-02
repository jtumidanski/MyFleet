package admin

import (
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// FR-ADMIN-PURGE-7: the disabled button in the UI is a courtesy; THIS is the
// control (risks.md R9). A mismatch is 409 with no writes.
func TestMatchConfirmation(t *testing.T) {
	cases := []struct {
		name     string
		scope    Scope
		label    string
		supplied string
		wantErr  bool
	}{
		{"record needs nothing", ScopeRecord, "", "", false},
		{"fleet exact name", ScopeFleet, "The Tumidanski Fleet", "The Tumidanski Fleet", false},
		{"fleet wrong name", ScopeFleet, "The Tumidanski Fleet", "the tumidanski fleet", true},
		{"fleet trailing space", ScopeFleet, "The Tumidanski Fleet", "The Tumidanski Fleet ", true},
		{"fleet empty", ScopeFleet, "The Tumidanski Fleet", "", true},
		{"system exact phrase", ScopeSystem, "", SystemConfirmation, false},
		{"system near miss", ScopeSystem, "", "purge everything", true},
		{"system fleet name", ScopeSystem, "The Tumidanski Fleet", "The Tumidanski Fleet", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := MatchConfirmation(tc.scope, tc.label, tc.supplied)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, server.ErrConflict) {
				t.Errorf("a mismatch must map to 409, got %v", err)
			}
		})
	}
}

// FR-ADMIN-PURGE-11. An unparseable value falls back rather than panicking,
// following the COOKIE_SECURE precedent: a typo in a ConfigMap must not stop
// the service booting.
func TestRecoveryWindow(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"120h", 120 * time.Hour},
		{"1h", time.Hour},
		{"", DefaultRecoveryWindow},
		{"five days", DefaultRecoveryWindow},
		{"-3h", DefaultRecoveryWindow},
		{"0", DefaultRecoveryWindow},
	}
	for _, tc := range cases {
		if got := RecoveryWindow(tc.raw); got != tc.want {
			t.Errorf("RecoveryWindow(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
	if DefaultRecoveryWindow != 5*24*time.Hour {
		t.Errorf("the default must be the 5 days the PRD specifies and the vehicle sweep already uses, got %v",
			DefaultRecoveryWindow)
	}
}
