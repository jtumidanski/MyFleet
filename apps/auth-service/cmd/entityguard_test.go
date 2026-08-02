package main

import (
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/database/entityguard"
)

// Guards the whole service tree rather than a hand-listed set of domains. The
// column-wipe defect (task-006, issue #7) appears in packages nobody thought to
// test, so a guard needing per-package opt-in would be absent exactly where it
// is needed. A new package under internal/ is covered the moment it exists.
//
// A finding means a package combines a db.Save write path with an Entity field
// that ToEntity() neither assigns nor protects with `gorm:"<-:create"`, so that
// column gets overwritten with its zero value on every update. The message says
// which field, which column, and both ways to fix it.
func TestNoLossySaveRoundTrips(t *testing.T) {
	findings, err := entityguard.Analyze("../internal")
	if err != nil {
		t.Fatalf("entityguard.Analyze: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}
