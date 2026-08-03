// Package fixture exists only for TestCollectTableNames_seesDeclarationsOutsideEntityGo.
//
// It lives under testdata/ for two reasons: the go tool ignores testdata
// directories entirely, so this never enters a build or a vet run, and
// CollectTableNames skips them by name, so the production walk in
// TestManifestCoversEveryTable never reports this table and fails the build.
package fixture

// Entity stands in for an entity declared somewhere other than entity.go —
// the exact shape that kept media.media_variant_failures invisible to the arch
// test across every task that touched it.
type Entity struct{}

func (Entity) TableName() string { return "media.fixture_table" }
