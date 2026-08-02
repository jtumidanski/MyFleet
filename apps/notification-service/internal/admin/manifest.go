// Package admin is notification-service's slice of the platform purge protocol.
//
// A fleet purge is WHERE fleet_id = ? and keys on user_id NOWHERE, at any scope.
// That is what dissolves risks.md R4 — "a user in two fleets loses both
// streams" — structurally rather than by care: there is no predicate in this
// file that could take another fleet's notifications.
package admin

// Scope is what a downstream purge is rooted at. There is no media_ids
// equivalent here: notifications are reachable from a fleet id alone.
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeFleet  Scope = "fleet"
)

// Root is the resolved purge root for this service.
type Root struct {
	Scope    Scope
	FleetIDs []string
}

// Target is one purgeable table and how to resolve its rows.
type Target struct {
	Key   string
	Table string
	Where func(Root) (string, []any)
}

const all = "1 = 1"

// Manifest is notification-service's purge surface.
var Manifest = []Target{
	{
		Key: "notifications", Table: "notification.notifications",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				// Rows with an empty fleet_id are account-level (the builder
				// makes fleet id optional). They survive a fleet purge and are
				// taken only by a system purge.
				return "fleet_id IN ?", []any{r.FleetIDs}
			}
			return "", nil
		},
	},
	{
		Key: "notification_preferences", Table: "notification.notification_preferences",
		Where: func(r Root) (string, []any) {
			// System scope only. The table is keyed (user_id, type) and carries
			// no fleet linkage at all, so there is no correct fleet predicate
			// (design OQ-2). Excluding it is safe: preferences regenerate with
			// defaults on the next read, so nothing a fleet purge should reach
			// lives here.
			if r.Scope == ScopeSystem {
				return all, nil
			}
			return "", nil
		},
	},
}

// excludedTables documents the deliberate omissions.
var excludedTables = map[string]string{
	// A finding, not bookkeeping: the PRD's "all of notification.*", taken
	// literally, truncates the idempotency ledger and lets a Kafka replay
	// regenerate notifications for data that was just purged.
	"notification.processed_events": "idempotency ledger; deleting it lets a Kafka replay resurrect purged notifications",
}
