// Package admin is media-service's slice of the platform purge protocol: stamp,
// restore, reap, and a stats count, all reachable only on the internal route
// tree.
//
// media.media_objects carries a NOT NULL indexed fleet_id, so a fleet-scoped
// purge is simply WHERE fleet_id IN (…) — no id-set passing (design OQ-1).
// Explicit ids survive in exactly one place: a record-scope purge of a single
// vehicle, where fleet-service must name the media objects that vehicle owns.
package admin

// Scope is what a downstream purge is rooted at.
type Scope string

const (
	ScopeSystem   Scope = "system"
	ScopeFleet    Scope = "fleet"
	ScopeMediaIDs Scope = "media_ids"
)

// Root is the resolved purge root for this service.
type Root struct {
	Scope    Scope
	FleetIDs []string
	MediaIDs []string
}

// Target is one purgeable table and how to resolve its rows.
type Target struct {
	Key   string
	Table string
	// Where returns the predicate + args, or ("", nil) when out of scope.
	// It never filters deleted_at on itself or on a parent — see fleet-service's
	// admin.Target for why the ordering property depends on that.
	Where func(Root) (string, []any)
}

const all = "1 = 1"

// Manifest is media-service's purge surface, child to parent.
var Manifest = []Target{
	{
		Key: "media_variants", Table: "media.media_variants",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "media_object_id IN (SELECT id FROM media.media_objects WHERE fleet_id IN ?)",
					[]any{r.FleetIDs}
			case ScopeMediaIDs:
				return "media_object_id IN ?", []any{r.MediaIDs}
			}
			return "", nil
		},
	},
	{
		Key: "media_objects", Table: "media.media_objects",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "fleet_id IN ?", []any{r.FleetIDs}
			case ScopeMediaIDs:
				return "id IN ?", []any{r.MediaIDs}
			}
			return "", nil
		},
	},
}

// excludedTables documents the deliberate omissions.
var excludedTables = map[string]string{
	// This is a finding, not bookkeeping. The PRD's "all of media.*" phrasing,
	// taken literally, would truncate the idempotency ledger — and a Kafka
	// replay would then regenerate variants for media that was just purged,
	// making a system purge undo itself on the next consumer restart.
	"media.processed_events": "idempotency ledger; deleting it lets a Kafka replay resurrect purged media",
	"outbox":                 "transient relay ledger drained by the outbox relay; owned by no fleet",
}
