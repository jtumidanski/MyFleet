package admin

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// SystemConfirmation is the exact literal a system purge requires. It is
// deliberately not the fleet name of anything, not derivable, and not
// case-insensitive.
const SystemConfirmation = "PURGE EVERYTHING"

// DefaultRecoveryWindow is the 5 days the PRD specifies, matching
// vehicle.recoveryWindow so the two recovery stories tell users the same thing.
const DefaultRecoveryWindow = 5 * 24 * time.Hour

// ErrConfirmationMismatch is the 409 a wrong confirmation phrase produces. It
// carries a detail so the console can say WHY without the client guessing.
var ErrConfirmationMismatch = server.Detailed(server.ErrConflict,
	"confirmation does not match the required phrase")

// MatchConfirmation is the server-side gate on destructive scopes
// (FR-ADMIN-PURGE-7).
//
// Comparison is exact — no trimming, no case folding. Both are tempting and both
// are wrong: the phrase exists to make the operator read what they are about to
// destroy, and a forgiving comparison makes it a formality. The disabled button
// in the console is a courtesy; this is the control (risks.md R9).
func MatchConfirmation(scope Scope, targetLabel, supplied string) error {
	switch scope {
	case ScopeRecord:
		// A single record is recoverable for five days and destroys nothing a
		// user cannot recreate; requiring a phrase here would train the operator
		// to type past the ones that matter.
		return nil
	case ScopeFleet:
		if supplied == targetLabel && targetLabel != "" {
			return nil
		}
	case ScopeSystem:
		if supplied == SystemConfirmation {
			return nil
		}
	}
	return ErrConfirmationMismatch
}

// RecoveryWindow parses ADMIN_PURGE_RECOVERY_WINDOW, falling back to the
// default on anything unparseable or non-positive.
//
// It does not panic. A typo in a ConfigMap must not stop the service booting —
// the same call the COOKIE_SECURE parse already makes in auth-service's
// composition root.
func RecoveryWindow(raw string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultRecoveryWindow
	}
	return d
}
