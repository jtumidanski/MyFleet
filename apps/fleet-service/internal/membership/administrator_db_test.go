package membership

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMembershipDB builds the in-memory harness. TableName is schema-qualified
// ("fleet.fleet_memberships") for Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet" so the qualified name resolves. Explicit
// DDL rather than Migration(db): the uniqueIndex tags on FleetID/UserID make
// GORM emit CREATE UNIQUE INDEX with the schema prefix stripped, which cannot
// resolve against an attached schema. Same workaround as
// auth-service/internal/user/provider_test.go and invite/resource_test.go.
func newMembershipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.fleet_memberships (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT, role TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("create fleet.fleet_memberships: %v", err)
	}
	return db
}

func seedMembership(t *testing.T, db *gorm.DB, userID, role string) Model {
	t.Helper()
	m := NewBuilder().SetFleetID("f1").SetUserID(userID).SetRole(role).Build()
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return created
}

func readRole(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var role string
	if err := db.Raw("SELECT role FROM fleet.fleet_memberships WHERE id = ?", id).Scan(&role).Error; err != nil {
		t.Fatalf("read role: %v", err)
	}
	return role
}

func countRows(t *testing.T, db *gorm.DB, id string) int {
	t.Helper()
	var n int
	if err := db.Raw("SELECT COUNT(*) FROM fleet.fleet_memberships WHERE id = ?", id).Scan(&n).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// recorded captures one ActivityRecorder invocation.
type recorded struct {
	actorUserID string
	eventType   string
	fleetID     string
	vehicleID   *string
	payload     map[string]any
}

// spyRecorder returns a recorder that appends to calls, plus the slice.
func spyRecorder(calls *[]recorded) ActivityRecorder {
	return func(_ *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error {
		*calls = append(*calls, recorded{actorUserID, eventType, fleetID, vehicleID, payload})
		return nil
	}
}

func TestUpdateRole_writesTheRoleAndRecordsRoleChanged(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	updated, err := adm.UpdateRole(target, "owner", "u-actor")
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updated.Role() != "owner" {
		t.Fatalf("returned model role = %q, want owner", updated.Role())
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("persisted role = %q, want owner", got)
	}
	if len(calls) != 1 {
		t.Fatalf("want exactly one activity call, got %d", len(calls))
	}
	c := calls[0]
	if c.eventType != "member.role_changed" {
		t.Errorf("eventType = %q, want member.role_changed", c.eventType)
	}
	if c.actorUserID != "u-actor" || c.fleetID != "f1" {
		t.Errorf("actor/fleet = %q/%q, want u-actor/f1", c.actorUserID, c.fleetID)
	}
	if c.vehicleID != nil {
		t.Errorf("member events are fleet-level; vehicleID must be nil, got %v", *c.vehicleID)
	}
	// from_role/to_role are both recorded so the entry is self-contained — the
	// feed must not have to replay history to say what changed.
	if c.payload["target_user_id"] != "u-target" ||
		c.payload["from_role"] != "member" || c.payload["to_role"] != "owner" {
		t.Errorf("payload = %+v, want target_user_id/from_role/to_role", c.payload)
	}
}

// FR-2.7: a no-op PATCH still writes an audit entry. A role-change log that
// silently omits some role-change requests is worse than one with a redundant
// row, and suppressing it would mean branching on "did anything change".
func TestUpdateRole_recordsEvenWhenTheRoleIsUnchanged(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "owner")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if _, err := adm.UpdateRole(target, "owner", "u-actor"); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("a no-op role change must still be recorded; got %d calls", len(calls))
	}
	if calls[0].payload["from_role"] != "owner" || calls[0].payload["to_role"] != "owner" {
		t.Errorf("payload = %+v, want from_role == to_role == owner", calls[0].payload)
	}
}

// FR-5.2. The activity row is the ONLY evidence a membership changed —
// memberships are hard-deleted with no tombstone — so the write and the record
// must share a transaction. A recorder failure must roll the domain write back.
func TestUpdateRole_rollsBackTheRoleWhenRecordingFails(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	boom := errors.New("recorder exploded")
	adm := NewAdministrator(db).WithActivityRecorder(
		func(*gorm.DB, string, string, string, *string, map[string]any) error { return boom },
	)

	if _, err := adm.UpdateRole(target, "owner", "u-actor"); !errors.Is(err, boom) {
		t.Fatalf("UpdateRole error = %v, want the recorder's error", err)
	}
	if got := readRole(t, db, target.ID()); got != "member" {
		t.Fatalf("role = %q after a failed record; the write must roll back", got)
	}
}

// A bare administrator (no recorder) must keep working — every existing
// construction site does exactly that.
func TestUpdateRole_worksWithoutARecorder(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	if _, err := NewAdministrator(db).UpdateRole(target, "owner", "u-actor"); err != nil {
		t.Fatalf("UpdateRole without a recorder: %v", err)
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("persisted role = %q, want owner", got)
	}
}

// D6: member.removed vs member.left is decided by actor == target, the same
// predicate that relaxes the authorization guard, so the two cannot disagree.
func TestRemove_recordsMemberRemovedWhenAnOwnerRemovesSomeoneElse(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if err := adm.Remove(target, "u-owner"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if countRows(t, db, target.ID()) != 0 {
		t.Fatal("membership row still present; Remove must hard-delete it")
	}
	if len(calls) != 1 || calls[0].eventType != "member.removed" {
		t.Fatalf("want one member.removed call, got %+v", calls)
	}
	if calls[0].actorUserID != "u-owner" {
		t.Errorf("actor = %q, want u-owner", calls[0].actorUserID)
	}
	if calls[0].payload["target_user_id"] != "u-target" || calls[0].payload["role"] != "member" {
		t.Errorf("payload = %+v, want target_user_id and role", calls[0].payload)
	}
}

func TestRemove_recordsMemberLeftWhenTheActorRemovesThemselves(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-self", "viewer")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if err := adm.Remove(target, "u-self"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(calls) != 1 || calls[0].eventType != "member.left" {
		t.Fatalf("want one member.left call, got %+v", calls)
	}
	if calls[0].payload["role"] != "viewer" {
		t.Errorf("payload = %+v, want role", calls[0].payload)
	}
}

func TestRemove_rollsBackTheDeleteWhenRecordingFails(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	boom := errors.New("recorder exploded")
	adm := NewAdministrator(db).WithActivityRecorder(
		func(*gorm.DB, string, string, string, *string, map[string]any) error { return boom },
	)

	if err := adm.Remove(target, "u-owner"); !errors.Is(err, boom) {
		t.Fatalf("Remove error = %v, want the recorder's error", err)
	}
	if countRows(t, db, target.ID()) != 1 {
		t.Fatal("membership row gone after a failed record; the delete must roll back")
	}
}

func TestRemove_worksWithoutARecorder(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	if err := NewAdministrator(db).Remove(target, "u-owner"); err != nil {
		t.Fatalf("Remove without a recorder: %v", err)
	}
	if countRows(t, db, target.ID()) != 0 {
		t.Fatal("membership row still present")
	}
}

// --- races against a row read outside the transaction ------------------------

// deleteRow removes a membership behind the caller's back, standing in for a
// concurrent request that won the race.
func deleteRow(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	if err := db.Exec("DELETE FROM fleet.fleet_memberships WHERE id = ?", id).Error; err != nil {
		t.Fatalf("delete row: %v", err)
	}
}

// The model handed to UpdateRole is read OUTSIDE the transaction, so the row can
// be gone by the time the write runs. An update matching zero rows must not
// commit an activity event for a membership that no longer exists.
func TestUpdateRole_returnsNotFoundAndRecordsNothingWhenTheRowIsGone(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")
	deleteRow(t, db, target.ID())

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if _, err := adm.UpdateRole(target, "owner", "u-actor"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateRole against a deleted row = %v, want ErrNotFound", err)
	}
	if len(calls) != 0 {
		t.Fatalf("activity recorded for a membership that no longer exists: %+v", calls)
	}
}

func TestRemove_returnsNotFoundAndRecordsNothingWhenTheRowIsGone(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")
	deleteRow(t, db, target.ID())

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if err := adm.Remove(target, "u-owner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove against a deleted row = %v, want ErrNotFound", err)
	}
	if len(calls) != 0 {
		t.Fatalf("departure recorded for a membership that was already gone: %+v", calls)
	}
}

// --- CountOwners ------------------------------------------------------------

// CountOwners is the zero-owner guard. ValidateRoleChange refuses to act on a
// non-active target, so counting a revoked owner would make the last real owner
// demotable.
func TestCountOwners_countsOnlyActiveOwners(t *testing.T) {
	db := newMembershipDB(t)
	seedMembership(t, db, "u-owner", "owner")
	seedMembership(t, db, "u-member", "member")
	if err := db.Exec(`INSERT INTO fleet.fleet_memberships (id, fleet_id, user_id, role, status)
		VALUES ('m-revoked', 'f1', 'u-gone', 'owner', 'revoked')`).Error; err != nil {
		t.Fatalf("seed revoked owner: %v", err)
	}

	n, err := NewProvider(db).CountOwners("f1")
	if err != nil {
		t.Fatalf("CountOwners: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountOwners = %d, want 1 — a revoked owner is not an owner", n)
	}
}

func TestCountOwners_isScopedToTheFleet(t *testing.T) {
	db := newMembershipDB(t)
	seedMembership(t, db, "u-owner", "owner")
	if err := db.Exec(`INSERT INTO fleet.fleet_memberships (id, fleet_id, user_id, role, status)
		VALUES ('m-other', 'other-fleet', 'u-elsewhere', 'owner', 'active')`).Error; err != nil {
		t.Fatalf("seed foreign owner: %v", err)
	}

	n, err := NewProvider(db).CountOwners("f1")
	if err != nil {
		t.Fatalf("CountOwners: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountOwners = %d, want 1 — another fleet's owner must not count", n)
	}
}

// ToEntity() carries only the five columns Model knows about, so a full-column
// db.Save built from it writes zeroes over created_at, deleted_at and
// purge_operation_id. Today NewAdministrator.UpdateRole uses Update("role", …)
// and never takes that path, which is also why the entityguard sweep in
// cmd/entityguard_test.go stays silent for this package — it only reports a
// package that ALREADY combines a Save with an unprotected column.
//
// That makes the safety a property of one call site rather than of the schema.
// `gorm:"<-:create"` on CreatedAt makes it structural: the column is written on
// INSERT and excluded from every UPDATE, so the day someone adds a Save here
// they do not silently reset every membership's creation date.
//
// DeletedAt and PurgeOperationID are deliberately NOT tagged — fleet/model.go:27-31
// records that tagging a soft-delete column makes a restore via Updates(map)
// report success while the row stays deleted.
func TestSaveFromToEntity_doesNotClobberCreatedAt(t *testing.T) {
	db := newMembershipDB(t)
	m := seedMembership(t, db, "u1", "owner")

	var before Entity
	if err := db.First(&before, "id = ?", m.ID()).Error; err != nil {
		t.Fatalf("read back seeded row: %v", err)
	}
	if before.CreatedAt.IsZero() {
		t.Fatal("precondition: insert left created_at zero")
	}

	// The write path this guards against: a full-column save built from ToEntity().
	if err := db.Save(m.ToEntity()).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	var after Entity
	if err := db.First(&after, "id = ?", m.ID()).Error; err != nil {
		t.Fatalf("read back after save: %v", err)
	}
	if after.CreatedAt.IsZero() {
		t.Fatal("db.Save zeroed created_at; Entity.CreatedAt must stay `gorm:\"<-:create\"`")
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Fatalf("db.Save rewrote created_at: %v -> %v", before.CreatedAt, after.CreatedAt)
	}
}
