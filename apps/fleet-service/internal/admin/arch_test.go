package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestManifestCoversEveryTable turns FR-ADMIN-PURGE-4's "every table, enumerated
// by hand" from a checklist into a compile-time-ish guarantee.
//
// It parses every entity.go under internal/, extracts each
// `func (X) TableName() string { return "…" }` literal, and requires the table
// to be in Manifest or in excludedTables with a reason. A new table added
// anywhere in fleet-service fails here until someone decides whether a purge
// should reach it.
//
// Parsing rather than grepping: table names appear in comments, raw SQL and
// test DDL all over the service, and a grep would produce false matches the
// first time someone documents one.
func TestManifestCoversEveryTable(t *testing.T) {
	inManifest := map[string]bool{}
	for _, target := range Manifest {
		inManifest[target.Table] = true
	}

	root := ".." // apps/fleet-service/internal
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "entity.go" {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// A parse failure must fail the test, not silently skip the file.
			t.Fatalf("parse %s: %v", path, perr)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TableName" || fn.Recv == nil || fn.Body == nil {
				continue
			}
			for _, stmt := range fn.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				lit, ok := ret.Results[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				name, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					t.Fatalf("%s: unquote %s: %v", path, lit.Value, uerr)
				}
				found = append(found, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
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

// TestAdminTreeIsSeparate is the structural control behind the whole
// cross-fleet API (risks.md R7, design §5.5).
//
// The admin API bypasses RequireSameFleet. That bypass is safe ONLY because it
// lives in a parallel route tree rather than in a relaxed guard. Two ways that
// could rot:
//
//   - someone "simplifies" an ordinary handler by short-circuiting
//     RequireSameFleet when Identity.PlatformAdmin is true, which would make
//     every ordinary endpoint cross-fleet capable for an admin;
//   - someone adds RequireSameFleet inside /admin, which would silently break
//     the console for every fleet the operator is not a member of and invite the
//     first fix above.
//
// "The admin tier" is internal/admin AND internal/adminclient. The latter is not
// a second home for the privilege — it holds no handlers at all, only the HTTP
// clients for the other services' internal admin routes, including the
// IsPlatformAdmin re-verification the purge path calls before doing anything
// irreversible (FR-ADMIN-AUTH-7). It exists solely to serve the console, so
// naming the privilege there is the package doing its job. Anywhere else, it is
// the leak this test exists to catch.
//
// Two allowlists on top of that, both resting on the same rule: naming a guard
// is not calling it. authz/scope.go DEFINES both guards; this file DEFINES the
// separation rule, so it necessarily spells out the very identifiers it forbids
// — in the doc comment above, in the search literals below, and in the failure
// messages. Without the self-exclusion the test fails on itself.
func TestAdminTreeIsSeparate(t *testing.T) {
	const internalRoot = ".."
	const selfPath = "../admin/arch_test.go"
	adminTier := []string{"../admin/", "../adminclient/"}
	allowedPlatformAdminRefs := map[string]bool{
		"../authz/scope.go":      true,
		"../authz/scope_test.go": true,
	}

	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		slash := filepath.ToSlash(path)
		if slash == selfPath {
			return nil
		}
		inAdmin := false
		for _, prefix := range adminTier {
			if strings.HasPrefix(slash, prefix) {
				inAdmin = true
				break
			}
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		text := string(src)

		if !inAdmin && !allowedPlatformAdminRefs[slash] {
			for _, ref := range []string{"PlatformAdmin", "RequirePlatformAdmin"} {
				if strings.Contains(text, ref) {
					t.Errorf("%s references %s outside the admin tier (internal/admin, internal/adminclient) — the platform tier must not "+
						"leak into an ordinary handler; that is how RequireSameFleet gets relaxed", path, ref)
				}
			}
		}
		// Matched as a call (trailing "("), not a bare mention: this file's own
		// doc comments and error strings name RequireSameFleet in prose (to
		// describe the very thing being guarded against), and manifest.go's
		// package doc does the same. Neither is a use.
		if inAdmin && strings.Contains(text, "RequireSameFleet(") {
			t.Errorf("%s calls RequireSameFleet inside the admin tier — the admin tree is "+
				"deliberately fleet-agnostic; adding the guard here breaks every fleet the "+
				"operator is not a member of", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalRoot, err)
	}
}

// TestNoManifestReachesTheAuthSchema is the standing guard behind the PRD's
// "a system purge must not delete accounts" acceptance criterion.
//
// That criterion was written to be checked by hand against a running stack.
// This checks it by construction instead, across all three purge manifests at
// once: a purge can only ever touch a table some manifest names, so a manifest
// that names no auth.* table cannot reach one. It is the same reasoning the
// separation test above uses — a structural fact beats a remembered
// observation, and this one is otherwise only ever confirmed by someone
// deliberately destroying a platform to look.
//
// If a future purge genuinely needs to reach auth data, this test is where that
// decision gets made explicitly rather than discovered afterwards.
func TestNoManifestReachesTheAuthSchema(t *testing.T) {
	manifests := []string{
		"manifest.go",
		"../../../media-service/internal/admin/manifest.go",
		"../../../notification-service/internal/admin/manifest.go",
	}
	for _, path := range manifests {
		src, err := os.ReadFile(path)
		if err != nil {
			// A moved manifest must fail loudly. A skip here would leave the
			// criterion silently unchecked, which is the failure mode this whole
			// file exists to prevent.
			t.Fatalf("read %s: %v", path, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, "Table:") {
				continue
			}
			if strings.Contains(line, `"auth.`) {
				t.Errorf("%s names an auth schema table in its purge manifest: %s\n"+
					"A purge must not be able to delete user accounts or sign-ins; if that has "+
					"genuinely changed, change it here deliberately.", path, strings.TrimSpace(line))
			}
		}
	}
}
