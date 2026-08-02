package mailer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Outcomes for myfleet_invite_emails_total (FR-OBS-1). Constants rather than
// bare strings so a typo in a rarely-hit branch cannot silently create a new
// time series that no dashboard ever shows.
const (
	OutcomeSent            = "sent"
	OutcomeFailedTransient = "failed_transient"
	OutcomeFailedPermanent = "failed_permanent"
	OutcomeSkippedDisabled = "skipped_disabled"
	OutcomeSkippedStale    = "skipped_stale"
)

// Registered on the default registry, which the existing /metrics route already
// serves — no wiring needed in main.
var inviteEmails = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "myfleet_invite_emails_total",
	Help: "Invite emails by outcome.",
}, []string{"outcome"})

// RecordOutcome increments the invite-email counter. Exported so mailconsumer,
// which owns the outcome decisions, does not import prometheus itself.
func RecordOutcome(outcome string) { inviteEmails.WithLabelValues(outcome).Inc() }
