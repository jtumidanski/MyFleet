package purge

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDomainPackagesDoNotImportEachOther makes FR-TEST-2's invariant a check
// rather than a comment.
//
// mediaobject, mediavariant and variantfailures are siblings: cross-package
// access goes through ports satisfied by composition-root adapters (the
// variantLookup and cardGenerator adapters in cmd/main.go), never through a
// direct import. This package is deliberately exempt — it is a job package with
// no consumers, and importing all three is composition, not coupling (design
// D-1) — which is exactly why the invariant needs enforcing somewhere.
func TestDomainPackagesDoNotImportEachOther(t *testing.T) {
	const prefix = "github.com/jtumidanski/myfleet/apps/media-service/internal/"
	siblings := []string{"mediaobject", "mediavariant", "variantfailures"}

	parsed := 0
	for _, pkg := range siblings {
		dir := filepath.Join("..", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			parsed++
			for _, imp := range f.Imports {
				p, uerr := strconv.Unquote(imp.Path.Value)
				if uerr != nil {
					t.Fatalf("%s: unquote %s: %v", path, imp.Path.Value, uerr)
				}
				for _, other := range siblings {
					if other == pkg || p != prefix+other {
						continue
					}
					t.Errorf("%s imports its sibling %s — cross-package access must go "+
						"through a port satisfied in the composition root (FR-TEST-2)", path, other)
				}
			}
		}
	}
	if parsed == 0 {
		t.Fatal("parsed no files — the walk root is wrong, and this test would pass vacuously")
	}
}
