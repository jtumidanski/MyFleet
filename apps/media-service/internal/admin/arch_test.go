package admin

import (
	"strings"
	"testing"
)

// TestManifestCoversEveryTable turns FR-ADMIN-PURGE-4's "every table, enumerated
// by hand" from a checklist into a compile-time-ish guarantee, exactly as
// fleet-service's twin of this test does for its own manifest.
//
// It walks every non-test .go file under internal/ — not only files named
// entity.go, which is how media.media_variant_failures stayed invisible to this
// check across every task that touched it (FR-ADMIN-5) — and requires each table
// to be in Manifest or in excludedTables with a reason. A new table added
// anywhere in media-service fails here until someone decides whether a purge
// should reach it.
func TestManifestCoversEveryTable(t *testing.T) {
	inManifest := map[string]bool{}
	for _, target := range Manifest {
		inManifest[target.Table] = true
	}

	found, err := CollectTableNames("..") // apps/media-service/internal
	if err != nil {
		// A parse failure fails the test, never a silent skip (FR-ADMIN-6).
		t.Fatalf("collect table names: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("found no TableName declarations — the walk root is wrong, and this test would pass vacuously")
	}

	for _, name := range found {
		if inManifest[name] {
			continue
		}
		if reason, ok := excludedTables[name]; ok {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s is excluded from the purge manifest with an empty reason", name)
			}
			continue
		}
		t.Errorf("%s is in neither admin.Manifest nor admin.excludedTables — "+
			"decide whether a purge should reach it, then add it to one of them", name)
	}
}

// TestManifestKeysAreUnique guards affected_counts: two targets sharing a key
// would silently overwrite each other's count and understate the blast radius.
func TestManifestKeysAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, target := range Manifest {
		if prev, dup := seen[target.Key]; dup {
			t.Errorf("manifest key %q is used by both %s and %s", target.Key, prev, target.Table)
		}
		seen[target.Key] = target.Table
	}
}

// TestCollectTableNames_seesDeclarationsOutsideEntityGo is the whole point of
// FR-ADMIN-5. The previous walk filtered on the file being named entity.go, so
// a table declared anywhere else was invisible — which is exactly how
// media.media_variant_failures ended up in neither Manifest nor excludedTables.
func TestCollectTableNames_seesDeclarationsOutsideEntityGo(t *testing.T) {
	got, err := CollectTableNames("testdata/fixture")
	if err != nil {
		t.Fatalf("CollectTableNames: %v", err)
	}
	if len(got) != 1 || got[0] != "media.fixture_table" {
		t.Fatalf("CollectTableNames(testdata/fixture) = %v, want [media.fixture_table]", got)
	}
}

// TestCollectTableNames_skipsTestdata is the other half. The fixture above is
// demonstrably findable by the test that names its path directly, so this is a
// real check and not a vacuous one: if the walk descended into testdata, the
// production run in TestManifestCoversEveryTable would report a table that does
// not exist and fail the build.
func TestCollectTableNames_skipsTestdata(t *testing.T) {
	got, err := CollectTableNames("..")
	if err != nil {
		t.Fatalf("CollectTableNames: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("found no TableName declarations — the walk root is wrong, and this test would pass vacuously")
	}
	for _, name := range got {
		if name == "media.fixture_table" {
			t.Fatal("the walk descended into testdata and picked up the fixture table")
		}
	}
}
