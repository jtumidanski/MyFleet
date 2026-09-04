// Package systemactor names the actor recorded for writes no human caused —
// the hourly purge reaper, the maintenance recompute sweep, and anything else
// that appends to an audit or activity trail from a background job.
//
// It exists because the sentinel has to be TWO different things at once, and
// conflating them cost the platform its purge reaper for eighteen days.
//
// Every actor_user_id column in fleet-service is `uuid NOT NULL`
// (admin.AuditEntity, activity.Entity). The literal "system" is not a uuid, so
// Postgres rejects the insert outright:
//
//	ERROR: invalid input syntax for type uuid: "system" (SQLSTATE 22P02)
//
// The reaper's audit insert shares a transaction with the hard delete and the
// status update, so that rejection rolled the whole reap back, every hour,
// silently — the operation stayed `pending` and the console kept promising it
// was recoverable long after its window had closed.
//
// The bug survived because every test in this repo runs on SQLite, which has no
// uuid type and stores "system" in a column declared `type:uuid` without
// complaint. A test cannot catch this; only the column type can. So the stored
// value is a uuid (ID) and the human-facing word is applied at the API edge
// (Display), which is also where it belongs — "system" is vocabulary, not data.
package systemactor

// ID is the actor_user_id written for any row a background job produced.
//
// The nil UUID rather than a generated one: it must be stable across
// deployments and databases (audit queries filter on it), it can never collide
// with a real user id issued by auth-service, and it reads as "no user" to
// anyone who encounters it in psql without this package to hand.
const ID = "00000000-0000-0000-0000-000000000000"

// Label is what an operator sees in the console instead of the sentinel. The
// frontend already treats this exact word as the system actor
// (apps/web/src/components/features/activity/activityEventMeta.ts), so
// translating here keeps that contract intact.
const Label = "system"

// Display renders a stored actor id for transport, substituting Label for the
// sentinel. Every REST transform that carries an actor_user_id must call it, or
// the console shows a row of zeroes where it should say "system".
func Display(actorUserID string) string {
	if actorUserID == ID {
		return Label
	}
	return actorUserID
}

// Resolve is Display's inverse, for query parameters: it maps the word an
// operator would type — the same word Display just showed them — back to the
// value actually stored. Without it, filtering the audit log by the actor the
// UI displays matches nothing (and, against a uuid column, errors).
func Resolve(actorUserID string) string {
	if actorUserID == Label {
		return ID
	}
	return actorUserID
}
