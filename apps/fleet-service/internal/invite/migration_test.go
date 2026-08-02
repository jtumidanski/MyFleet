package invite

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newInviteMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	ddl := `CREATE TABLE fleet.fleet_invites (
		id TEXT PRIMARY KEY, fleet_id TEXT, email TEXT, role TEXT, token TEXT,
		expires_at DATETIME, accepted_at DATETIME, invited_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// A purged invite must release its token. Tokens are generated, so a collision
// is not the worry — the worry is a re-invite failing with an unexplained
// constraint error days after an unrelated purge.
func TestPartialTokenIndex_freesTheTokenAfterSoftDelete(t *testing.T) {
	db := newInviteMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO fleet.fleet_invites (id, fleet_id, email, role, token, invited_by_user_id)
	           VALUES (?, 'fleet-1', 'a@example.com', 'member', 'tok-1', 'owner-1')`
	if err := db.Exec(insert, "i1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Exec(insert, "i2").Error; err == nil {
		t.Fatal("a second LIVE invite with the same token must be rejected")
	}

	if err := db.Exec(`UPDATE fleet.fleet_invites SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'i1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "i3").Error; err != nil {
		t.Fatalf("re-issuing the token after a purge must succeed, got %v", err)
	}
}

func TestInviteApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newInviteMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
