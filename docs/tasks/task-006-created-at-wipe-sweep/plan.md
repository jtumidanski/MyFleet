# `created_at` Wipe Sweep — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close every live instance of the full-column-write column-wipe defect across the four Go
services, restore correct vehicle status derivation after an edit, add a static guard so the defect
class cannot silently return, and ship an operator runbook that repairs the rows already zeroed.

**Architecture:** Two fix layers per affected domain — `gorm:"<-:create"` protects the database
column from a full-column `Save`, and assigning the field in `ToEntity()` protects the `Model`
returned by `Make(e)` after the write. A new `go/ast` analyzer in `shared-go`
(`database/entityguard`) reports any package that combines a `.Save(` call site with an `Entity`
field that `ToEntity()` neither assigns nor protects by tag; it is invoked from one three-line test
per service, so every package under a service's `internal/` is covered with no per-domain
registration. The already-corrupted production rows are repaired by raw SQL in a runbook, not by an
ORM migration — GORM silently discards writes to a `<-:create` column.

**Tech Stack:** Go 1.25, GORM v1.31.1 (`gorm.io/gorm`, `gorm.io/driver/sqlite` for tests),
`go/ast` + `go/parser` from the standard library, six-module `go.work` workspace, Postgres in
production / in-memory SQLite in tests.

## Global Constraints

Copied from the PRD and design; every task's requirements implicitly include these.

- **No schema change.** No column added, dropped, renamed, or retyped. `AutoMigrate` output must be
  unchanged. `<-:create` is a write-permission tag and takes no part in DDL generation.
- **No API change.** No JSON:API attribute is added, removed, or altered. `created_at` stays
  unexposed on `user`, `fleet`, `vehicle`, and `mediaobject`. The only externally observable change
  is a *corrected* value in the existing derived `status` attribute on `GET /vehicles` and
  `GET /vehicles/:id`.
- **No frontend change.** No `apps/web` file is touched.
- **No `Administrator` interface reshaping.** The defect is fixed inside the existing
  `Model`/`Entity`/builder architecture. Migrating `Update` to
  `db.Model(...).Select(cols).Updates(...)` is an explicit non-goal (PRD §2) and stays a follow-up.
- **Never tag a `gorm.DeletedAt` field `<-:create`.** Measured (design §2, V6): a restore via
  `Updates(map{...})` then returns `err=nil, RowsAffected=1` while the row stays deleted. Carry the
  value through `ToEntity()` instead.
- **Tests assert database behaviour, not struct tags.** A `ToEntity()`-only assertion passes while
  the column still gets wiped, which is precisely the failure mode this task exists to rule out.
  Every fix is proven by a test that persists a row, updates it through the real `Administrator`,
  and re-reads the column.
- **Test DBs use `ATTACH DATABASE ':memory:' AS <schema>` plus explicit DDL, never `AutoMigrate`.**
  GORM emits `CREATE INDEX` with the schema prefix stripped under SQLite, which cannot resolve
  against an attached schema. Every hand-written DDL block carries a `KEEP IN SYNC WITH entity.go`
  comment.
- **Baseline to hold:** `make build`, `make vet`, `make lint-check` clean; `make test` green at
  37 packages ok / 0 failures plus the new packages.
- **Formatting:** run `./tools/lint.sh --fmt` (or `make lint`) after editing Go files rather than
  hand-aligning struct fields — gofumpt + goimports are the authority and CI runs `--check`.
- **Every task ends with a commit.** Conventional-commit style, matching the repo's history
  (`fix(...)`, `test(...)`, `feat(...)`, `docs(...)`).

## Task Order and Rationale

1. **Task 1** builds the analyzer in isolation with its own fixture tests — self-contained and
   green on its own, and its fixtures already prove the guard *fires*.
2. **Tasks 2–4** fix the three affected domains, each driven by a behavioural test written first.
3. **Task 5** pins the already-shipped auth-service fix.
4. **Task 6** wires the analyzer into all four services. It comes *after* the fixes deliberately:
   wiring it earlier would leave the tree red between tasks.
5. **Task 7** writes the repair runbook.
6. **Task 8** is whole-repo verification and the PR write-up.

---

### Task 1: The `entityguard` static analyzer

**Files:**
- Create: `packages/shared-go/database/entityguard/entityguard.go`
- Test: `packages/shared-go/database/entityguard/entityguard_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `func Analyze(root string) ([]Finding, error)` — walks every directory under `root`, returns
    one `Finding` per unprotected column and per package it cannot decide about. Returns a non-nil
    error only for I/O or parse failures.
  - `type Finding struct { Dir, Package, Field, Column, EntityPos, SavePos, Detail string; Reason Reason }`
  - `func (f Finding) String() string` — the actionable failure message.
  - `type Reason string` with `ReasonUnassigned` and `ReasonUnverifiable`.

  Task 6 depends on exactly these names.

**Status of the code below:** the analyzer and its tests in this task were compiled, `go vet`-ed,
and run in a scratch module while this plan was written — all eight tests pass, and Step 5 records
the output measured against the real service trees. Copy them as written; if something does not
compile, the transcription is wrong rather than the design.

**Why an analyzer and not a reflective round-trip helper:** a helper needs each domain to opt in,
and the seven latent domains are latent precisely because nobody was thinking about this defect when
they were written. The eighth will be written by someone in the same position. The analyzer covers
every package that exists today and every package added tomorrow, at zero marginal cost per domain
(design §6.1).

- [ ] **Step 1: Write the failing test**

Create `packages/shared-go/database/entityguard/entityguard_test.go`. Fixtures are written to
`t.TempDir()` as source text rather than committed under `testdata/`, so they are never compiled,
never linted, and the whole guard lives in one readable file.

```go
package entityguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture materialises a single-package fixture tree under root and
// returns root. The fixtures are never compiled — the analyzer only parses —
// so they carry no imports and no gorm dependency.
func writeFixture(t *testing.T, pkg, src string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, pkg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "domain.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return root
}

// lossySrc is the exact shape of the bug: a Save call site plus a ToEntity that
// leaves a persisted column unassigned and untagged.
const lossySrc = `package widget

type Entity struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Model struct {
	id   string
	name string
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, Name: m.name}
}

func (a *admin) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Save(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
`

func TestAnalyze_flagsUnassignedColumnBehindASave(t *testing.T) {
	got, err := Analyze(writeFixture(t, "widget", lossySrc))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(got), got)
	}
	f := got[0]
	if f.Reason != ReasonUnassigned {
		t.Fatalf("Reason = %q, want %q", f.Reason, ReasonUnassigned)
	}
	if f.Field != "CreatedAt" {
		t.Fatalf("Field = %q, want CreatedAt", f.Field)
	}
	if f.Column != "created_at" {
		t.Fatalf("Column = %q, want created_at", f.Column)
	}
	if f.Package != "widget" {
		t.Fatalf("Package = %q, want widget", f.Package)
	}
	if !strings.Contains(f.EntityPos, "domain.go:") || !strings.Contains(f.SavePos, "domain.go:") {
		t.Fatalf("positions must name a file and line, got EntityPos=%q SavePos=%q", f.EntityPos, f.SavePos)
	}
}

// FR-GUARD-2: a developer seeing this message must be able to act on it without
// reading the PRD. It names the package, the field, the column, both source
// locations, both remedies, and the gorm.DeletedAt trap.
func TestFindingString_namesDomainColumnAndBothRemedies(t *testing.T) {
	got, err := Analyze(writeFixture(t, "widget", lossySrc))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	msg := got[0].String()
	for _, want := range []string{
		"widget", "Entity.CreatedAt", `"created_at"`, "db.Save",
		"assigning it in ToEntity()", `gorm:"<-:create"`, "gorm.DeletedAt",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message must mention %q; got:\n%s", want, msg)
		}
	}
}

// Layer one of the fix: the tag. Accepted as sufficient so the analyzer and the
// code agree on what "protected" means (design §4).
func TestAnalyze_tagIsSufficientProtection(t *testing.T) {
	src := strings.Replace(lossySrc,
		"CreatedAt time.Time\n", "CreatedAt time.Time `gorm:\"<-:create\"`\n", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a `<-:create` column must not be flagged; got %v", got)
	}
}

// Layer two of the fix: assignment in ToEntity.
func TestAnalyze_assignmentIsSufficientProtection(t *testing.T) {
	src := strings.Replace(lossySrc,
		"return Entity{ID: m.id, Name: m.name}",
		"return Entity{ID: m.id, Name: m.name, CreatedAt: m.createdAt}", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("an assigned column must not be flagged; got %v", got)
	}
}

// The defect needs BOTH ingredients. A lossy round-trip with no Save write path
// is one of the seven latent domains — correct today, and not this guard's
// business until someone adds a Save.
func TestAnalyze_lossyRoundTripWithoutASaveIsNotAFinding(t *testing.T) {
	src := strings.Replace(lossySrc, "a.db.Save(&e)", "a.db.Create(&e)", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("no Save means no defect; got %v", got)
	}
}

// UpdatedAt is the single allowlist entry: GORM's autoUpdateTime callback stamps
// it on every write and assigns it back into the struct, so ToEntity leaving it
// unassigned is correct rather than a defect (design §6.3).
func TestAnalyze_updatedAtIsAllowlisted(t *testing.T) {
	got, err := Analyze(writeFixture(t, "widget", lossySrc))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	for _, f := range got {
		if f.Field == "UpdatedAt" {
			t.Fatalf("UpdatedAt must be allowlisted, got finding: %v", f)
		}
	}
}

// A guard that silently passes when it cannot run is worse than no guard. If
// ToEntity's shape defeats analysis, that is a finding, not a skip.
func TestAnalyze_unparseableToEntityIsAFinding(t *testing.T) {
	src := strings.Replace(lossySrc,
		"	return Entity{ID: m.id, Name: m.name}",
		"	var e Entity\n	e.ID = m.id\n	return e", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 || got[0].Reason != ReasonUnverifiable {
		t.Fatalf("want exactly one %q finding, got %v", ReasonUnverifiable, got)
	}
	if !strings.Contains(got[0].String(), "composite literal") {
		t.Fatalf("message must say how to make the package analyzable; got:\n%s", got[0].String())
	}
}

// Tests are not production write paths. A Save in a _test.go file must not make
// the package look defective.
func TestAnalyze_ignoresTestFiles(t *testing.T) {
	root := writeFixture(t, "widget",
		strings.Replace(lossySrc, "a.db.Save(&e)", "a.db.Create(&e)", 1))
	extra := "package widget\n\nfunc TestX() { db.Save(&Entity{}) }\n"
	if err := os.WriteFile(filepath.Join(root, "widget", "domain_test.go"), []byte(extra), 0o644); err != nil {
		t.Fatalf("write test fixture: %v", err)
	}
	got, err := Analyze(root)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a Save in a _test.go file must be ignored; got %v", got)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
go test ./packages/shared-go/database/entityguard/...
```

Expected: build failure — `undefined: Analyze`, `undefined: ReasonUnassigned`,
`undefined: ReasonUnverifiable`.

- [ ] **Step 3: Write the analyzer**

Create `packages/shared-go/database/entityguard/entityguard.go`:

```go
// Package entityguard detects the full-column-write column-wipe defect
// (task-006, issue #7): a package that persists through db.Save while its
// Model.ToEntity() leaves a persisted column unassigned, so GORM's
// UPDATE-every-column write overwrites that column with its zero value.
//
// It is a source analyzer rather than a per-domain test helper on purpose. The
// defect appears in packages nobody thought to test, so a guard that requires
// opt-in would be absent exactly where it is needed. One Analyze call per
// service covers every package under that service's internal/ tree, including
// packages that do not exist yet.
package entityguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gorm.io/gorm/schema"
)

// Reason distinguishes a proven finding from an undecidable one.
type Reason string

const (
	// ReasonUnassigned: the field is a persisted column that ToEntity() never
	// assigns and no gorm tag protects from UPDATE.
	ReasonUnassigned Reason = "unassigned"
	// ReasonUnverifiable: the package has both a Save and a ToEntity, but
	// ToEntity's shape defeats analysis. Reported rather than skipped — a guard
	// that silently passes when it cannot run is worse than no guard.
	ReasonUnverifiable Reason = "unverifiable"
)

// Finding is one unprotected column, or one package that could not be decided.
type Finding struct {
	Dir       string // directory as walked, e.g. "../internal/vehicle"
	Package   string // declared package name, e.g. "vehicle"
	Field     string // Entity field name, empty for ReasonUnverifiable
	Column    string // derived database column name, empty for ReasonUnverifiable
	EntityPos string // file:line of the Entity struct declaration
	SavePos   string // file:line of the Save call that makes it reachable
	Detail    string // why the package could not be decided
	Reason    Reason
}

func (f Finding) String() string {
	if f.Reason == ReasonUnverifiable {
		return fmt.Sprintf(
			"%s: cannot verify the Entity round-trip — %s. %s writes the row with db.Save, which "+
				"UPDATEs every column, so an unassigned column would be silently overwritten with "+
				"its zero value on every update.\n\n"+
				"Rewrite ToEntity() to return a single composite literal so this guard can check it.\n"+
				"Do NOT tag a gorm.DeletedAt `gorm:\"<-:create\"`: it silently breaks restore paths "+
				"(task-006 design §2, V6).",
			f.Package, f.Detail, f.SavePos)
	}
	return fmt.Sprintf(
		"%s: Entity.%s (column %q) is not assigned by ToEntity() (%s), but %s writes the row with "+
			"db.Save, which UPDATEs every column — so this column will be overwritten with its zero "+
			"value on every update.\n\n"+
			"Fix by either:\n"+
			"  - assigning it in ToEntity(), if Model carries the value; or\n"+
			"  - tagging the field `gorm:\"<-:create\"`, if the column must never change after insert.\n"+
			"Do NOT tag a gorm.DeletedAt this way: it silently breaks restore paths "+
			"(task-006 design §2, V6).",
		f.Package, f.Field, f.Column, f.EntityPos, f.SavePos)
}

// autoManaged lists fields GORM stamps itself on every write, so ToEntity
// leaving them unassigned is correct rather than a defect. Exactly one entry,
// justified by GORM's autoUpdateTime callback. Adding an entry is a deliberate
// shared-go change with a comment explaining why — never a per-domain opt-out,
// which is what stops this list from eroding (design §6.3).
var autoManaged = map[string]bool{"UpdatedAt": true}

var namer schema.NamingStrategy

// Analyze walks every directory under root and reports the findings for each.
// Directories are visited in sorted order so output is stable.
func Analyze(root string) ([]Finding, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor") {
			return fs.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)

	var out []Finding
	for _, dir := range dirs {
		found, err := analyzeDir(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

func analyzeDir(dir string) ([]Finding, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	pkgName := ""
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		files = append(files, f)
		pkgName = f.Name.Name
	}
	if len(files) == 0 {
		return nil, nil
	}

	savePos := findSaveCall(fset, files)
	if savePos == "" {
		return nil, nil // no Save write path: the defect needs both ingredients
	}
	entityName, assigned, ok := findToEntity(files)
	if entityName == "" {
		return nil, nil // no Model/Entity round-trip in this package
	}
	base := Finding{Dir: filepath.Clean(dir), Package: pkgName, SavePos: savePos}
	if !ok {
		base.Reason = ReasonUnverifiable
		base.Detail = "ToEntity() does not return a single composite literal"
		return []Finding{base}, nil
	}
	st, stPos := findStruct(fset, files, entityName)
	if st == nil {
		base.Reason = ReasonUnverifiable
		base.Detail = "ToEntity() returns " + entityName + ", but no such struct is declared in this package"
		return []Finding{base}, nil
	}
	base.EntityPos = stPos

	var out []Finding
	for _, field := range st.Fields.List {
		tag := gormTag(field)
		if len(field.Names) == 0 {
			f := base
			f.Reason = ReasonUnverifiable
			f.Detail = "Entity embeds a struct, so this guard cannot enumerate its columns"
			out = append(out, f)
			continue
		}
		for _, n := range field.Names {
			if !n.IsExported() || autoManaged[n.Name] || assigned[n.Name] || writeRestricted(tag) {
				continue
			}
			f := base
			f.Reason = ReasonUnassigned
			f.Field = n.Name
			f.Column = columnName(n.Name, tag)
			out = append(out, f)
		}
	}
	return out, nil
}

// findSaveCall returns the position of the first `x.Save(...)` call in the
// package, or "". Deliberately syntactic and un-typed: any Save-shaped write is
// treated as a full-column write, which errs toward reporting.
func findSaveCall(fset *token.FileSet, files []*ast.File) string {
	pos := ""
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if pos != "" {
				return false
			}
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Save" {
				pos = position(fset, sel.Sel.Pos())
				return false
			}
			return true
		})
		if pos != "" {
			break
		}
	}
	return pos
}

// findToEntity locates `func (…) ToEntity() T` and returns T plus the set of
// field names its returned composite literal assigns. ok is false when the body
// is not a single return of a composite literal — the undecidable case.
func findToEntity(files []*ast.File) (entityName string, assigned map[string]bool, ok bool) {
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Recv == nil || fn.Name.Name != "ToEntity" || fn.Body == nil {
				continue
			}
			if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			ident, isIdent := fn.Type.Results.List[0].Type.(*ast.Ident)
			if !isIdent {
				continue
			}
			entityName = ident.Name
			lit := soleReturnedCompositeLit(fn.Body)
			if lit == nil {
				return entityName, nil, false
			}
			assigned = map[string]bool{}
			for _, elt := range lit.Elts {
				kv, isKV := elt.(*ast.KeyValueExpr)
				if !isKV {
					// A positional literal names no fields, so nothing can be
					// proven about which columns it sets.
					return entityName, nil, false
				}
				if key, isKeyIdent := kv.Key.(*ast.Ident); isKeyIdent {
					assigned[key.Name] = true
				}
			}
			return entityName, assigned, true
		}
	}
	return "", nil, false
}

func soleReturnedCompositeLit(body *ast.BlockStmt) *ast.CompositeLit {
	var returns []*ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if r, isReturn := n.(*ast.ReturnStmt); isReturn {
			returns = append(returns, r)
		}
		return true
	})
	if len(returns) != 1 || len(returns[0].Results) != 1 {
		return nil
	}
	expr := returns[0].Results[0]
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary {
		expr = unary.X
	}
	lit, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return nil
	}
	return lit
}

func findStruct(fset *token.FileSet, files []*ast.File, name string) (*ast.StructType, string) {
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, isTS := spec.(*ast.TypeSpec)
				if !isTS || ts.Name.Name != name {
					continue
				}
				if st, isStruct := ts.Type.(*ast.StructType); isStruct {
					return st, position(fset, ts.Pos())
				}
			}
		}
	}
	return nil, ""
}

func gormTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	// field.Tag.Value keeps its backquotes; StructTag wants them stripped.
	return reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("gorm")
}

// writeRestricted reports whether the gorm tag already stops a full-column
// UPDATE from writing this field. "<-" , "<-:update" and "<-:all" all GRANT
// update permission and therefore do not protect anything.
func writeRestricted(tag string) bool {
	for _, setting := range strings.Split(tag, ";") {
		setting = strings.TrimSpace(setting)
		if setting == "" {
			continue
		}
		key, value := setting, ""
		if i := strings.Index(setting, ":"); i >= 0 {
			key, value = setting[:i], setting[i+1:]
		}
		switch key {
		case "-": // not persisted at all
			return true
		case "->": // read-only
			return true
		case "<-":
			if value == "create" || value == "false" {
				return true
			}
		}
	}
	return false
}

// columnName derives the database column for the message. GORM's own naming
// strategy is used so the reported name matches what actually gets written.
func columnName(field, tag string) string {
	for _, setting := range strings.Split(tag, ";") {
		setting = strings.TrimSpace(setting)
		if i := strings.Index(setting, ":"); i > 0 && strings.EqualFold(setting[:i], "column") {
			return setting[i+1:]
		}
	}
	return namer.ColumnName("", field)
}

func position(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(p.Filename), p.Line)
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```sh
go test ./packages/shared-go/database/entityguard/...
```

Expected: `ok  github.com/jtumidanski/myfleet/packages/shared-go/database/entityguard`, all eight
tests passing.

- [ ] **Step 5: Preflight the analyzer against the real service trees**

The fixtures prove the guard's logic; this proves it agrees with the sweep the PRD did by hand.
Write a throwaway program under the scratch directory (NOT in the repo) that calls `Analyze` on each
service's `internal/` tree and prints the findings, run it, then delete it.

Expected on the **unfixed** tree — exactly three findings, all in `fleet-service`:

```
=== apps/auth-service/internal: 0 findings
=== apps/fleet-service/internal: 3 findings
  [unassigned] fleet   Entity.CreatedAt (created_at)  entity=.../fleet/entity.go:10   save=.../fleet/administrator.go:27
  [unassigned] fleet   Entity.DeletedAt (deleted_at)  entity=.../fleet/entity.go:10   save=.../fleet/administrator.go:27
  [unassigned] vehicle Entity.CreatedAt (created_at)  entity=.../vehicle/entity.go:10 save=.../vehicle/administrator.go:65
=== apps/media-service/internal: 0 findings
=== apps/notification-service/internal: 0 findings
```

This was measured while writing this plan and is the exact expected output. It confirms three things
at once: the analyzer independently reproduces PRD §4.1's hand sweep; `mediaobject` and `user` are
genuinely clean already (so Tasks 4 and 5 really are hardening and pinning, not fixes); and the
seven latent domains correctly produce nothing, because none has a `Save`.

Any *fourth* finding means the tree changed since this plan was written — investigate before
continuing rather than adjusting the guard to match.

- [ ] **Step 6: Confirm no `go.mod` change was needed**

```sh
cd packages/shared-go && go mod tidy && git diff --stat go.mod go.sum
```

Expected: empty diff. `gorm.io/gorm` is already a direct dependency and `gorm.io/gorm/schema` is a
package inside it. If `go.mod` did change, stop and investigate rather than committing the churn.

- [ ] **Step 7: Format, vet, lint**

```sh
./tools/lint.sh --fmt packages/shared-go
go vet ./packages/shared-go/...
./tools/lint.sh --check --go packages/shared-go
```

Expected: all clean.

- [ ] **Step 8: Commit**

```bash
git add packages/shared-go/database/entityguard/
git commit -m "feat(shared-go): add entityguard analyzer for lossy Save round-trips"
```

---

### Task 2: `fleet-service/internal/vehicle` — the live, user-facing fix

**Files:**
- Modify: `apps/fleet-service/internal/vehicle/entity.go:22` (tag) and `:54-70` (`ToEntity`)
- Test: `apps/fleet-service/internal/vehicle/administrator_db_test.go` (create)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: no new exported API. `vehicle.Entity.CreatedAt` becomes insert-only, and
  `vehicle.Model.CreatedAt()` becomes correct after an `Administrator.Update`. Task 6's guard test
  depends on this package being clean.

**Why this one matters:** `DeriveStatus` (`status.go:49-53`) falls back to the vehicle's creation
time when it has no activity rows. Create a vehicle → `PATCH /vehicles/:id` → `created_at` becomes
`0001-01-01` → an activity-free vehicle reports **"Inactive"** forever, and the true timestamp is
unrecoverable from the row. This is the only defect in the sweep with a user-visible symptom.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/vehicle/administrator_db_test.go`:

```go
package vehicle

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newVehicleDB builds the in-memory harness. Schema-qualified TableNames
// ("fleet.vehicles") target Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet". AutoMigrate is unusable here: Entity
// carries `index` tags and GORM emits CREATE INDEX with the schema prefix
// stripped under SQLite, which cannot resolve against an attached schema. Same
// approach as maintenanceschedule/completion_db_test.go.
func newVehicleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.vehicles (
		id TEXT PRIMARY KEY, fleet_id TEXT, nickname TEXT, make TEXT, model TEXT,
		trim TEXT, year INTEGER, vin TEXT, current_mileage INTEGER,
		primary_image_media_id TEXT, notes TEXT, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME, purge_after DATETIME)`).Error; err != nil {
		t.Fatalf("create fleet.vehicles: %v", err)
	}
	return db
}

func seedVehicle(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetNickname("before").Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; this harness cannot detect the defect it exists for")
	}
	return created
}

// readCreatedAt reads the column directly rather than through Make(), so the
// assertion is about what is IN the row, not about what the round-trip claims.
func readCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM fleet.vehicles WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// The regression test for issue #7's "wider concern". Before the fix ToEntity()
// left CreatedAt at its zero value and Administrator.Update wrote it through
// db.Save, which UPDATEs every column — so one PATCH set created_at to
// 0001-01-01 permanently.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)
	want := readCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithNickname("after"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Update zeroed created_at; a full-column Save must not write this column")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Update: got %v, want %v", got, want)
	}
	// Layer two: the Model handed back must carry it as well, or a caller
	// deriving status from the returned Model sees a zero creation time even
	// though the row is fine.
	if updated.CreatedAt().IsZero() {
		t.Fatal("Update returned a Model with a zero CreatedAt(); ToEntity must assign the field")
	}
	// And the real change still landed.
	if updated.Nickname() != "after" {
		t.Fatalf("Nickname = %q, want %q — the update itself must still apply", updated.Nickname(), "after")
	}
}

type fakeScheduleStates struct{ states []string }

func (f fakeScheduleStates) ScheduleStatesByVehicle(string) ([]string, error) {
	return f.states, nil
}

// noActivity is the case DeriveStatus's created_at fallback exists for.
type noActivity struct{}

func (noActivity) LastActivityByVehicle(string) (time.Time, error) { return time.Time{}, nil }

// FR-FIX-1's user-visible symptom. Asserting created_at != zero alone would not
// prove the bug is gone; this is the acceptance criterion that does.
func TestDeriveStatus_editedVehicleWithoutActivityStaysHealthy(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)

	if _, err := NewAdministrator(db).Update(created.WithNickname("after")); err != nil {
		t.Fatalf("update: %v", err)
	}
	reread, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}

	deps := StatusDeps{Schedules: fakeScheduleStates{states: []string{"ok"}}, Activity: noActivity{}}
	if got := deps.DeriveStatus(reread, time.Now().UTC()); got != "Healthy" {
		t.Fatalf("DeriveStatus after an edit = %q, want %q — a zeroed created_at makes the "+
			"inactivity fallback compare against 0001-01-01 and report Inactive", got, "Healthy")
	}
}

// PRD Q5, resolved by test rather than by assumption: SoftDelete and RestoreRow
// use narrowed Updates(map{...}) writes, which design §2 V7 measured as
// unaffected by a `<-:create` field elsewhere on the struct. "Unaffected" is
// exactly the claim a tag change falsifies quietly, so pin it.
func TestRestoreRow_survivesTheInsertOnlyCreatedAtTag(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)
	a := NewAdministrator(db)
	want := readCreatedAt(t, db, created.ID())

	if _, err := a.SoftDelete(created.ID()); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	restored, err := a.RestoreRow(created.ID())
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.DeletedAt() != nil {
		t.Fatalf("RestoreRow left deleted_at set: %v", restored.DeletedAt())
	}
	if _, err := NewProvider(db).GetByID(created.ID()); err != nil {
		t.Fatalf("restored vehicle is invisible to GetByID: %v", err)
	}
	if got := readCreatedAt(t, db, created.ID()); !got.Equal(want) {
		t.Fatalf("created_at changed across soft-delete/restore: got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail for the right reason**

```sh
go test -run 'TestUpdate_preservesCreatedAt|TestDeriveStatus_editedVehicle|TestRestoreRow_survives' \
  ./apps/fleet-service/internal/vehicle/... -v
```

Expected:
- `TestUpdate_preservesCreatedAt` — FAIL: `Update zeroed created_at; a full-column Save must not
  write this column`.
- `TestDeriveStatus_editedVehicleWithoutActivityStaysHealthy` — FAIL:
  `DeriveStatus after an edit = "Inactive", want "Healthy"`.
- `TestRestoreRow_survivesTheInsertOnlyCreatedAtTag` — PASS already (no tag exists yet); it becomes
  meaningful in Step 4.

This is the SQLite harness reproducing the production bug (design §2, V3). If
`TestUpdate_preservesCreatedAt` passes here, **stop** — the harness is not exercising the real write
path and the rest of this task proves nothing.

- [ ] **Step 3: Apply both fix layers**

In `apps/fleet-service/internal/vehicle/entity.go`, replace the `CreatedAt` field line inside
`Entity` (currently line 22, `CreatedAt time.Time`) with:

```go
	// Both layers are deliberate (task-006 design §4). `<-:create` protects the
	// COLUMN: db.Save in Administrator.Update UPDATEs every column, and GORM
	// gives CreatedAt none of the auto-management it gives UpdatedAt, so an
	// untagged column is written as 0001-01-01 on every PATCH. Assigning the
	// field in ToEntity() protects the MODEL that Make(e) returns after the
	// write — DeriveStatus falls back to CreatedAt(), so a zero there reports a
	// healthy vehicle as "Inactive" even when the row is fine.
	CreatedAt time.Time `gorm:"<-:create"`
```

and add the assignment to `ToEntity` (after `Notes: m.notes,`):

```go
		CreatedAt:           m.createdAt,
```

`UpdatedAt` stays absent from `ToEntity`: GORM owns it, stamps it on every write, and assigns it
back into the struct, so `Make(e)` returns the correct value. This is the analyzer's single
allowlist entry and the only reason for it.

- [ ] **Step 4: Run the tests and verify they pass**

```sh
go test -race ./apps/fleet-service/internal/vehicle/... -v
```

Expected: all three new tests PASS, plus the pre-existing `TestBuild_*` and `TestPurgeAfter_*` /
`TestIsPurgeable_*` tests still passing.

- [ ] **Step 5: Verify the whole service is still green**

```sh
go test -race ./apps/fleet-service/...
```

Expected: ok across every fleet-service package. `maintenanceschedule/completion_db_test.go` inserts
vehicles through this same path, so it is the canary for an `Insert`-path regression from the tag.

- [ ] **Step 6: Format, vet, lint, commit**

```sh
./tools/lint.sh --fmt apps/fleet-service
go vet ./apps/fleet-service/...
./tools/lint.sh --check --go apps/fleet-service
```

```bash
git add apps/fleet-service/internal/vehicle/entity.go apps/fleet-service/internal/vehicle/administrator_db_test.go
git commit -m "fix(fleet-service): stop vehicle updates from wiping created_at"
```

---

### Task 3: `fleet-service/internal/fleet` — `CreatedAt` and the `DeletedAt` round-trip

**Files:**
- Modify: `apps/fleet-service/internal/fleet/model.go:6-18` (add `deletedAt` + accessor)
- Modify: `apps/fleet-service/internal/fleet/entity.go:14` (tag), `:24-41` (`Make`/`ToEntity`)
- Test: `apps/fleet-service/internal/fleet/administrator_db_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func (m Model) DeletedAt() *time.Time` on `fleet.Model`. `fleet.Entity.CreatedAt`
  becomes insert-only. No other package consumes these yet; Task 6's guard test depends on this
  package being clean.

**Why `DeletedAt` is carried rather than tagged (PRD Q2, resolved in design §5.2):** tagging a
`gorm.DeletedAt` `<-:create` does not break soft *delete* (V5) but breaks *restore* (V6) — and
breaks it silently, returning `err=nil` and `RowsAffected=1` on a row that stays deleted. `vehicle`
already has a restore path, so a future `fleet` restore is a plausible change written by someone
using `vehicle` as the template. Shipping a tag that turns that change into a silent no-op trades an
unreachable bug for a reachable one. The `*time.Time` domain type matches `vehicle.Model.deletedAt`
exactly and keeps the GORM type out of the domain layer; the conversion is confined to `entity.go`,
already the only file in the package that knows about GORM.

**Reachability, honestly:** nothing in this package calls `Delete` or `Unscoped` today, so
FR-FIX-3 closes a hole that cannot currently be reached. It is closed anyway because design §2's V4
proves the shape is real, and because the analyzer would otherwise flag the package — the guard
should not need an exemption on day one.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/fleet/administrator_db_test.go`:

```go
package fleet

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newFleetDB builds the in-memory harness. Schema-qualified TableNames
// ("fleet.fleets") target Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet". Explicit DDL rather than AutoMigrate:
// Entity carries an `index` tag on DeletedAt and GORM emits CREATE INDEX with
// the schema prefix stripped under SQLite, which cannot resolve against an
// attached schema.
func newFleetDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.fleets (
		id TEXT PRIMARY KEY, name TEXT, created_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`).Error; err != nil {
		t.Fatalf("create fleet.fleets: %v", err)
	}
	return db
}

func seedFleet(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetName("before").SetCreatedByUserID("u1").Build()
	if err != nil {
		t.Fatalf("build fleet: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert fleet: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; this harness cannot detect the defect it exists for")
	}
	return created
}

func readFleetCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM fleet.fleets WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// FR-FIX-2. Processor.Rename is Update(m.WithName(name)) — so before the fix,
// renaming a fleet destroyed its creation date. No read consumes it today, but
// Model.CreatedAt() is exported and the loss is unrecoverable once it happens.
func TestUpdate_preservesCreatedAtAcrossRename(t *testing.T) {
	db := newFleetDB(t)
	created := seedFleet(t, db)
	want := readFleetCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithName("renamed"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readFleetCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Rename zeroed created_at; a full-column Save must not write this column")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Rename: got %v, want %v", got, want)
	}
	if updated.CreatedAt().IsZero() {
		t.Fatal("Update returned a Model with a zero CreatedAt(); ToEntity must assign the field")
	}
	if updated.Name() != "renamed" {
		t.Fatalf("Name = %q, want %q — the rename itself must still apply", updated.Name(), "renamed")
	}
	// deleted_at must remain NULL: a live fleet must not acquire a tombstone.
	var deleted *time.Time
	if err := db.Raw("SELECT deleted_at FROM fleet.fleets WHERE id = ?", created.ID()).
		Scan(&deleted).Error; err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	if deleted != nil {
		t.Fatalf("Rename set deleted_at to %v on a live fleet", *deleted)
	}
}

// FR-FIX-3, the design §2 V4 shape: ToEntity dropped DeletedAt, so a
// full-column Save wrote NULL over any soft-delete tombstone. Unreachable today
// (nothing loads a deleted fleet), which is why the test drives GORM's soft
// delete directly rather than through a domain method that does not exist yet.
func TestUpdate_doesNotResurrectASoftDeletedFleet(t *testing.T) {
	db := newFleetDB(t)
	created := seedFleet(t, db)

	if err := db.Delete(&Entity{ID: created.ID()}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var e Entity
	if err := db.Unscoped().First(&e, "id = ?", created.ID()).Error; err != nil {
		t.Fatalf("unscoped read: %v", err)
	}
	m := Make(e)
	if m.DeletedAt() == nil {
		t.Fatal("Make() dropped the soft-delete tombstone; the Model must carry deletedAt")
	}
	if _, err := NewAdministrator(db).Update(m.WithName("renamed")); err != nil {
		t.Fatalf("update: %v", err)
	}

	var visible int64
	if err := db.Model(&Entity{}).Where("id = ?", created.ID()).Count(&visible).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if visible != 0 {
		t.Fatalf("a full-column Save resurrected the soft-deleted fleet: it is visible to a "+
			"scoped read again (count=%d)", visible)
	}
}
```

- [ ] **Step 2: Add the READ half of the round-trip so the tests compile**

The tests reference `Model.DeletedAt()`, which does not exist yet, so they cannot even build. Add
only the read half now — the write half is the fix, and adding it here would skip the red phase.

In `apps/fleet-service/internal/fleet/model.go`, add the field to `Model` (after `updatedAt`):

```go
	deletedAt       *time.Time
```

and the accessor after `UpdatedAt()`:

```go
// DeletedAt returns the soft-delete tombstone, or nil for a live fleet. The
// value is carried through the Entity round-trip rather than protected by a
// `<-:create` tag: tagging a gorm.DeletedAt makes a restore via Updates(map)
// report success while the row stays deleted (task-006 design §2, V6).
func (m Model) DeletedAt() *time.Time { return m.deletedAt }
```

In `apps/fleet-service/internal/fleet/entity.go`, populate it in `Make` and add the two conversion
helpers. Leave `ToEntity` and the `CreatedAt` tag untouched:

```go
// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:              e.ID,
		name:            e.Name,
		createdByUserID: e.CreatedByUserID,
		createdAt:       e.CreatedAt,
		updatedAt:       e.UpdatedAt,
		deletedAt:       fromGormDeletedAt(e.DeletedAt),
	}
}

// fromGormDeletedAt / toGormDeletedAt keep gorm.DeletedAt out of the domain
// layer: Model carries a *time.Time, matching vehicle.Model.deletedAt, and the
// conversion lives in entity.go, already the only file here that knows GORM.
func fromGormDeletedAt(d gorm.DeletedAt) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func toGormDeletedAt(t *time.Time) gorm.DeletedAt {
	if t == nil {
		return gorm.DeletedAt{}
	}
	return gorm.DeletedAt{Time: *t, Valid: true}
}
```

`toGormDeletedAt` is unused until Step 4. Go permits an unused function (only unused *locals* and
*imports* are errors), and `unused` from golangci's `standard` group flags unexported unused
functions — so do not run `lint-check` between here and Step 4; Step 6 is where lint must be clean.

- [ ] **Step 3: Run the tests and verify they fail for the right reason**

```sh
go test -run 'TestUpdate_preservesCreatedAtAcrossRename|TestUpdate_doesNotResurrect' \
  ./apps/fleet-service/internal/fleet/... -v
```

Expected:
- `TestUpdate_preservesCreatedAtAcrossRename` — FAIL: `Rename zeroed created_at; a full-column Save
  must not write this column`.
- `TestUpdate_doesNotResurrectASoftDeletedFleet` — FAIL: `a full-column Save resurrected the
  soft-deleted fleet`.

**Honest note on the resurrection test:** design §2 V4 measured that a full-column `Save` *does*
resurrect a soft-deleted row. If this one passes here instead — because GORM scoped the UPDATE to
`deleted_at IS NULL` and the write silently did nothing — record that observation in the commit
message rather than deleting the test or forcing a failure. The test stays either way: it pins the
behaviour, and FR-FIX-3 is explicitly about closing a shape that is currently unreachable.

- [ ] **Step 4: Apply the fix — the tag and the write half of the round-trip**

In `apps/fleet-service/internal/fleet/entity.go`, replace `CreatedAt time.Time` in `Entity` with:

```go
	// `<-:create` protects the COLUMN from db.Save's UPDATE-every-column write;
	// assigning it in ToEntity() protects the MODEL that Make(e) returns after
	// the write. Both layers are deliberate (task-006 design §4).
	CreatedAt time.Time `gorm:"<-:create"`
```

and complete `ToEntity`. It must stay a **single composite literal** — `entityguard` reports a
package it cannot decide about, and a build-then-mutate body is exactly that case:

```go
// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		Name:            m.name,
		CreatedByUserID: m.createdByUserID,
		CreatedAt:       m.createdAt,
		DeletedAt:       toGormDeletedAt(m.deletedAt),
	}
}
```

`UpdatedAt` stays unassigned — GORM stamps it on every write and assigns it back into the struct.
`DeletedAt` is carried, never tagged, for the reason in this task's preamble.

- [ ] **Step 5: Run the tests and verify they pass**

```sh
go test -race ./apps/fleet-service/internal/fleet/... -v
```

Expected: both new tests PASS, and the existing `builder_test.go` tests still pass.

- [ ] **Step 6: Verify the service is still green**

```sh
go test -race ./apps/fleet-service/...
```

Expected: ok across every package. `membership`'s `CreateFleetWithOwner` builds and inserts fleets
through `fleet.ToEntity`, so it is the canary for an `Insert`-path regression.

- [ ] **Step 7: Format, vet, lint, commit**

```sh
./tools/lint.sh --fmt apps/fleet-service
go vet ./apps/fleet-service/...
./tools/lint.sh --check --go apps/fleet-service
```

```bash
git add apps/fleet-service/internal/fleet/
git commit -m "fix(fleet-service): carry created_at and deleted_at through the fleet round-trip"
```

---

### Task 4: `media-service/internal/mediaobject` — hardening

**Files:**
- Modify: `apps/media-service/internal/mediaobject/entity.go:20` (tag only)
- Test: `apps/media-service/internal/mediaobject/administrator_db_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `mediaobject.Entity.CreatedAt` becomes insert-only. Reuses `newConfirmTestDB` from
  `processor_test.go` (same package) — do not write a second harness.

**This must not change behaviour, and the tests must confirm that.** `ToEntity()` is already
complete here; its correctness is incidental rather than structural, and it sits behind two `Save`
call sites. `NewBuilder()` sets `createdAt: time.Now().UTC()` (`builder.go:20`), so the value is
non-zero at insert, and design §2 V1 confirms `<-:create` does not interfere with `Create`. The
tests assert the value is *preserved*, which is simultaneously the no-behaviour-change assertion.

- [ ] **Step 1: Write the tests**

Create `apps/media-service/internal/mediaobject/administrator_db_test.go`:

```go
package mediaobject

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// Reuses newConfirmTestDB from processor_test.go — same package, same DDL, and
// duplicating a KEEP-IN-SYNC schema block is how the two drift apart.
func seedMediaObject(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetFleetID("f1").SetUploadedByUserID("u1").
		SetBucket("bucket").SetObjectKey("key").Build()
	if err != nil {
		t.Fatalf("build media object: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert media object: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; the `<-:create` tag must not block the insert path")
	}
	return created
}

func readMediaCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM media.media_objects WHERE id = ?", id).
		Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// FR-FIX-4, call site one (administrator.go:35). Hardening: this path is
// correct today only because Model happens to carry createdAt. The test pins
// both the behaviour and the absence of any change to it.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newConfirmTestDB(t)
	created := seedMediaObject(t, db)
	want := readMediaCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithStatus(StatusProcessing))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readMediaCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Update zeroed created_at")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Update: got %v, want %v", got, want)
	}
	if updated.Status() != StatusProcessing {
		t.Fatalf("Status = %q, want %q — the update itself must still apply", updated.Status(), StatusProcessing)
	}
}

// FR-FIX-4, call site two (administrator.go:47, inside the transaction).
func TestUpdateInTx_preservesCreatedAt(t *testing.T) {
	db := newConfirmTestDB(t)
	created := seedMediaObject(t, db)
	want := readMediaCreatedAt(t, db, created.ID())

	hookRan := false
	updated, err := NewAdministrator(db).UpdateInTx(created.WithStatus(StatusProcessing),
		func(tx *gorm.DB) error {
			hookRan = true
			return nil
		})
	if err != nil {
		t.Fatalf("update in tx: %v", err)
	}
	if !hookRan {
		t.Fatal("UpdateInTx did not run its hook")
	}

	got := readMediaCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("UpdateInTx zeroed created_at")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across UpdateInTx: got %v, want %v", got, want)
	}
	if updated.Status() != StatusProcessing {
		t.Fatalf("Status = %q, want %q", updated.Status(), StatusProcessing)
	}
}
```

- [ ] **Step 2: Run the tests and verify they pass BEFORE the tag**

```sh
go test -run 'TestUpdate_preservesCreatedAt|TestUpdateInTx_preservesCreatedAt' \
  ./apps/media-service/internal/mediaobject/... -v
```

Expected: **PASS**. Unlike Tasks 2 and 3 this is not a red-then-green cycle — there is no live
defect here, and a failure would mean the round-trip is lossy in a way the sweep missed. If either
test fails, stop and report it: the PRD's classification of this site as "correct today" would be
wrong.

- [ ] **Step 3: Add the tag**

In `apps/media-service/internal/mediaobject/entity.go`, replace `CreatedAt time.Time` in `Entity`
with:

```go
	// Standing protection, not a bug fix: ToEntity() already assigns this, so
	// nothing here was ever corrupted. That correctness is incidental — it holds
	// only while Model happens to carry createdAt — and two db.Save call sites
	// (Update and UpdateInTx) depend on it. `<-:create` makes it structural
	// (task-006 design §5.3).
	CreatedAt time.Time `gorm:"<-:create"`
```

- [ ] **Step 4: Run the tests again and verify nothing changed**

```sh
go test -race ./apps/media-service/... -v
```

Expected: both new tests still PASS with identical assertions, and every other media-service package
still ok. That the results are unchanged *is* the no-behaviour-change evidence required by FR-FIX-4.

- [ ] **Step 5: Format, vet, lint, commit**

```sh
./tools/lint.sh --fmt apps/media-service
go vet ./apps/media-service/...
./tools/lint.sh --check --go apps/media-service
```

```bash
git add apps/media-service/internal/mediaobject/
git commit -m "test(media-service): pin created_at across both mediaobject save paths"
```

---

### Task 5: `auth-service/internal/user` — pin the shipped fix

**Files:**
- Test: `apps/auth-service/internal/user/administrator_db_test.go` (create)
- Modify: nothing. `Entity.CreatedAt` already carries `gorm:"<-:create"` with a comment explaining
  why (`entity.go:15-27`), added by commit `7642e28` on `main`.

**Interfaces:**
- Consumes: `newTestDB(t)` from `provider_test.go` (same package) — reuse it, do not duplicate the
  DDL.
- Produces: nothing.

**Why a test at all:** the existing `entity_test.go` tests round-trip *fields* (`ThemePreference`)
rather than database behaviour. A `ToEntity()` assertion would pass while the column still got
wiped — exactly the failure mode FR-GUARD-3 rules out. This test goes through `db.Save`, so the
shipped fix cannot be reverted by someone tidying struct tags.

- [ ] **Step 1: Write the test**

Create `apps/auth-service/internal/user/administrator_db_test.go`:

```go
package user

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func readUserCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM auth.users WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// Pins the fix already shipped on main (Entity.CreatedAt `gorm:"<-:create"`).
// The originally reported defect: ProvisionFromGoogle calls Administrator.Update
// on every re-login, ToEntity() carries no createdAt, and db.Save UPDATEs every
// column — so created_at became 0001-01-01 on each login. This test drives the
// real write path; an entity round-trip assertion would pass while the column
// still got wiped.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newTestDB(t)
	m := NewBuilder().SetGoogleSub("sub-1").SetEmail("a@b.com").SetDisplayName("A").Build()
	a := NewAdministrator(db)
	if _, err := a.Insert(m); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	want := readUserCreatedAt(t, db, m.ID())
	if want.IsZero() {
		t.Fatal("Insert left created_at zero; `<-:create` must not block the insert path")
	}

	// The re-login write.
	if _, err := a.Update(m.WithLogin("A2", "https://example.test/a.png", time.Now().UTC())); err != nil {
		t.Fatalf("update user: %v", err)
	}

	got := readUserCreatedAt(t, db, m.ID())
	if got.IsZero() {
		t.Fatal("re-login zeroed created_at; Entity.CreatedAt must stay `gorm:\"<-:create\"`")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across a re-login: got %v, want %v", got, want)
	}

	// The update itself must still land.
	reread, err := NewProvider(db).GetByID(m.ID())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.DisplayName() != "A2" {
		t.Fatalf("DisplayName = %q, want %q", reread.DisplayName(), "A2")
	}
}
```

- [ ] **Step 2: Run the test and verify it passes**

```sh
go test -race -run TestUpdate_preservesCreatedAt ./apps/auth-service/internal/user/... -v
```

Expected: PASS — the fix is already on `main`.

- [ ] **Step 3: Prove the test is not vacuous**

Temporarily delete the ` \`gorm:"<-:create"\` ` tag from `Entity.CreatedAt` in
`apps/auth-service/internal/user/entity.go`, re-run:

```sh
go test -run TestUpdate_preservesCreatedAt ./apps/auth-service/internal/user/... -v
```

Expected: FAIL with `re-login zeroed created_at`. Then restore the tag:

```sh
git checkout -- apps/auth-service/internal/user/entity.go
go test -run TestUpdate_preservesCreatedAt ./apps/auth-service/internal/user/... -v   # PASS again
```

Record the observed failure output — it goes in the PR description. A pin nobody has seen fail is
an untested pin.

- [ ] **Step 4: Verify the service is green, then format, vet, lint, commit**

```sh
go test -race ./apps/auth-service/...
./tools/lint.sh --fmt apps/auth-service
go vet ./apps/auth-service/...
./tools/lint.sh --check --go apps/auth-service
git status --short   # must show ONLY the new test file
```

```bash
git add apps/auth-service/internal/user/administrator_db_test.go
git commit -m "test(auth-service): pin created_at across the re-login save path"
```

---

### Task 6: Wire the guard into all four services and prove it fires

**Files:**
- Create: `apps/auth-service/cmd/entityguard_test.go`
- Create: `apps/fleet-service/cmd/entityguard_test.go`
- Create: `apps/media-service/cmd/entityguard_test.go`
- Create: `apps/notification-service/cmd/entityguard_test.go`

**Interfaces:**
- Consumes: `entityguard.Analyze(root string) ([]Finding, error)` and `Finding.String()` from
  Task 1; the clean state of `vehicle`, `fleet`, `mediaobject`, and `user` from Tasks 2–5.
- Produces: nothing consumed by later tasks.

Each service's entrypoint is `apps/<svc>/cmd/main.go`, `package main`. The test lives beside it so
`Analyze("../internal")` resolves from the test's working directory, and so the guard runs under the
existing `make test` with no new CI wiring.

**Coverage boundary (design §6.4):** the per-service test covers every package in that service,
forever. It does not cover a *fifth* service created later with no such test. That gap is accepted
deliberately — a new Go service is a large, deliberate, reviewed event, unlike a new domain package,
which is routine — and it is recorded in Task 8's follow-ups so the omission is a decision rather
than an oversight.

- [ ] **Step 1: Confirm the package clause of each entrypoint**

```sh
head -1 apps/auth-service/cmd/main.go apps/fleet-service/cmd/main.go \
        apps/media-service/cmd/main.go apps/notification-service/cmd/main.go
```

Expected: `package main` for all four. If any differs, use its package clause in the file below.

- [ ] **Step 2: Write the guard test in each service**

Create the same file in all four `cmd/` directories, changing nothing between them:

```go
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
```

- [ ] **Step 3: Run the guard in every service and verify it is green**

```sh
go test -race ./apps/auth-service/cmd/... ./apps/fleet-service/cmd/... \
              ./apps/media-service/cmd/... ./apps/notification-service/cmd/... -v -run TestNoLossySaveRoundTrips
```

Expected: PASS in all four. `notification-service` passes because its single write path is an
`OnConflict.DoNothing` `Create` and its `ToEntity()` already carries `CreatedAt` — the audit in PRD
§7 is now continuously enforced rather than a one-time claim.

If `fleet-service` reports findings, Task 2 or Task 3 is incomplete — fix the domain, do not relax
the guard.

- [ ] **Step 4: Prove the guard fires on real code (design §6.5)**

FR-GUARD's acceptance criterion is that the guard *fires*, not that it exists. Break `vehicle`
deliberately:

```sh
# Remove BOTH protections from vehicle.Entity.CreatedAt:
#   - delete the line `CreatedAt:           m.createdAt,` from ToEntity()
#   - delete the `gorm:"<-:create"` tag from the CreatedAt field
$EDITOR apps/fleet-service/internal/vehicle/entity.go

go test -run TestNoLossySaveRoundTrips ./apps/fleet-service/cmd/... -v
go test -run 'TestUpdate_preservesCreatedAt|TestDeriveStatus_editedVehicle' ./apps/fleet-service/internal/vehicle/... -v
```

Expected: the analyzer test FAILS naming `vehicle`, `Entity.CreatedAt`, column `"created_at"`, both
file positions, and both remedies; and the behavioural tests FAIL with the zeroed-column and
`"Inactive"` messages. **Copy both outputs verbatim** — they go in the PR description.

Then revert and re-confirm green:

```sh
git checkout -- apps/fleet-service/internal/vehicle/entity.go
go test -race ./apps/fleet-service/... -run 'TestNoLossySaveRoundTrips|TestUpdate_preservesCreatedAt|TestDeriveStatus_editedVehicle' -v
git status --short   # must show ONLY the four new cmd/entityguard_test.go files
```

- [ ] **Step 5: Format, vet, lint, commit**

```sh
./tools/lint.sh --fmt apps
go vet github.com/jtumidanski/myfleet/...
./tools/lint.sh --check --go
```

```bash
git add apps/auth-service/cmd/entityguard_test.go apps/fleet-service/cmd/entityguard_test.go \
        apps/media-service/cmd/entityguard_test.go apps/notification-service/cmd/entityguard_test.go
git commit -m "test(services): run entityguard over every service internal tree"
```

---

### Task 7: The data-repair runbook

**Files:**
- Create: `docs/runbooks/created-at-repair.md`

**Interfaces:**
- Consumes: nothing in code. This task adds no Go, no CI wiring, and no automated test — the runbook
  is a document, and its correctness is established by the mandatory dry-run pass.
- Produces: nothing consumed by later tasks.

**Why a runbook rather than a Go migration (design §8):** design §2's V7 measured that GORM
*silently discards* a write to a `<-:create` column — `err=nil`, no rows changed, no warning. An
ORM-based repair would report complete success and change nothing, and no test would naturally catch
that. The repair must be raw SQL; once it is raw SQL, judgement-laden, and one-shot, the argument
for embedding it in every service boot is gone.

`migration-plan.md`'s accuracy notes are **reproduced** in the runbook rather than linked: the
operator running this a year from now will not read the task folder.

- [ ] **Step 1: Write the runbook**

Create `docs/runbooks/created-at-repair.md`:

````markdown
# Runbook — repairing zeroed `created_at` rows

A defect in the persistence layer wrote `0001-01-01` over `created_at` on every full-column
`UPDATE` (issue #7). The code fix ships in task-006; this runbook repairs the rows that were
already corrupted before it shipped.

**Run this once, by hand, against the bee cluster's Postgres. It is not part of any deploy.**

## Before you start

**Ordering is a hard constraint: the code fix must already be deployed.** Repairing first means
the next `PATCH /vehicles/:id`, fleet rename, or re-login re-zeroes the row — producing a
repaired-then-recorrupted row that reads as a failed migration. Confirm the running images
include the task-006 fix before continuing.

**Connect as a role that can see all three schemas.** All four services share one database
(`myfleet`) and differ only by `search_path` (`deploy/k8s/secrets.example.yaml:18,34,44,57`), so
the cross-schema transaction below is valid — but it cannot be run through a service's own
connection.

## What a repaired value means, and what it does not

Each backfilled `created_at` is an **upper bound with a known bias**, not a measurement. Nothing
in the schema distinguishes a repaired value from an original one afterwards (adding such a
column would be a schema change, which task-006 forbids), so this document and the output you
capture below are the only record. Do not later treat a repaired timestamp as ground truth.

The proxies work at all for a structural reason rather than a lucky one: the tables that supply
them are insert-only (`fleet.activity_events`, `fleet.fleet_memberships`, `auth.refresh_tokens`),
and insert-only tables were never touched by this bug — GORM populates `CreatedAt` correctly on
`Create`. The corruption was confined to exactly the tables with a `Save`-based update.

| Table | Proxy | What it means |
|---|---|---|
| `fleet.vehicles` | earliest `vehicle.created` row in `fleet.activity_events` | **Strongest.** The event is written in the same transaction as the vehicle insert, so where the event exists the value is exact to within that transaction. Vehicles created before activity recording existed have no such event and are unrecoverable. |
| `fleet.fleets` | earliest `fleet.fleet_memberships` row for the fleet | The owner membership is created with the fleet in one transaction, so this is exact where that membership survives. If the original owner was removed and memberships hard-deleted, the value skews **late**. |
| `auth.users` | earliest `auth.refresh_tokens` row for the user | **Weakest.** A refresh token is minted at first login, which for a Google-provisioned user is the same request that created the row — accurate to sub-second *while the earliest token survives*. Tokens expire and get pruned, so for long-tenured users this can post-date signup by an arbitrary margin, potentially months. |

`media.media_objects` is **not** in scope: its round-trip always carried `CreatedAt`, so no media
row was ever corrupted.

## Pass 1 — count (read-only, mandatory)

Run this first and keep the output. It is the "before" half of the FR-DATA-4 report.

```sql
SELECT 'auth.users'     AS tbl, COUNT(*) AS still_zero FROM auth.users     WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.vehicles',        COUNT(*)               FROM fleet.vehicles  WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.fleets',          COUNT(*)               FROM fleet.fleets    WHERE created_at < '1970-01-01';
```

Also capture the affected ids, since nothing in the data will mark them afterwards:

```sql
SELECT 'auth.users' AS tbl, id FROM auth.users     WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.vehicles',    id FROM fleet.vehicles WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.fleets',      id FROM fleet.fleets   WHERE created_at < '1970-01-01';
```

If every count is zero, stop — there is nothing to repair.

## Pass 2 — the one row to check by hand

User `7a186017-d27e-4d65-90e3-6b240bf9880a` is the only row whose corruption is independently
documented (issue #7), which makes it the only available sanity check on the weakest of the three
proxies. Inspect its token history before the bulk run and satisfy yourself the proxy is
plausible:

```sql
SELECT MIN(created_at) AS earliest_token, MAX(created_at) AS latest_token, COUNT(*) AS tokens
  FROM auth.refresh_tokens
 WHERE user_id = '7a186017-d27e-4d65-90e3-6b240bf9880a';
```

If `earliest_token` is implausibly recent (e.g. this week, for an account you know is older), the
tokens have been pruned and the proxy will overstate the creation date. Decide deliberately
whether to accept that or to leave the row zeroed by removing the `auth.users` statement from
Pass 3.

## Pass 3 — repair

Every statement is idempotent (re-running is a no-op) and guarded to touch only zeroed rows. The
`< '1970-01-01'` predicate is deliberately looser than `= '0001-01-01'` so it also catches any
other pre-epoch garbage, while remaining incapable of matching a legitimate row. Nothing here can
alter identity, ownership, or soft-delete state — only timestamp columns are written.

```sql
BEGIN;

-- fleet.vehicles ← vehicle.created activity event (strongest proxy)
UPDATE fleet.vehicles v
   SET created_at = a.created_at
  FROM (
        SELECT vehicle_id, MIN(created_at) AS created_at
          FROM fleet.activity_events
         WHERE type = 'vehicle.created' AND vehicle_id IS NOT NULL
      GROUP BY vehicle_id
       ) a
 WHERE v.id = a.vehicle_id
   AND v.created_at < '1970-01-01';

-- fleet.fleets ← earliest membership
UPDATE fleet.fleets f
   SET created_at = m.created_at
  FROM (
        SELECT fleet_id, MIN(created_at) AS created_at
          FROM fleet.fleet_memberships
      GROUP BY fleet_id
       ) m
 WHERE f.id = m.fleet_id
   AND f.created_at < '1970-01-01';

-- auth.users ← earliest refresh token (weakest proxy; see the table above)
UPDATE auth.users u
   SET created_at = t.created_at
  FROM (
        SELECT user_id, MIN(created_at) AS created_at
          FROM auth.refresh_tokens
      GROUP BY user_id
       ) t
 WHERE u.id = t.user_id
   AND u.created_at < '1970-01-01';

COMMIT;
```

Note each statement's reported row count as you go — that is the "repaired" half of the report.

## Pass 4 — report

Re-run the Pass 1 counting query. Rows still counted are **genuinely unrecoverable**: their proxy
table has no surviving row. Report them as such; do not retry them with a weaker proxy.

Record, wherever this cluster's operational history lives:

- repaired count per table (Pass 3 row counts),
- unrecoverable count per table (Pass 4 counts),
- the id list captured in Pass 1.

## If something looks wrong

Pass 3 is a single transaction — if any statement errors, nothing is committed. `ROLLBACK;` and
re-check the proxy tables exist and are populated. Because the repair only ever moves a row from
`0001-01-01` to a plausible timestamp, and only when guarded by `< '1970-01-01'`, a partial or
repeated run cannot make the data worse than it already is.
````

- [ ] **Step 2: Verify the SQL parses against the real schema**

The statements must be validated before anyone runs them for real. With a psql session on a bee
snapshot (or any database carrying the three schemas):

```sql
BEGIN;
-- paste Pass 3 here
ROLLBACK;
```

Expected: no syntax or missing-column errors. Note the row counts the statements report, then
`ROLLBACK` so nothing persists. If a column name is wrong (`fleet.activity_events.type`,
`.vehicle_id`, `fleet.fleet_memberships.fleet_id`, `auth.refresh_tokens.user_id`), correct the
runbook and re-run.

If no database is reachable in this environment, say so explicitly rather than claiming the SQL was
validated — the runbook then ships marked as validated-by-inspection only, and the operator's
mandatory Pass 1 / Pass 2 dry runs are what catch a mistake.

- [ ] **Step 3: Commit**

```bash
git add docs/runbooks/created-at-repair.md
git commit -m "docs(runbooks): add the created_at repair procedure"
```

---

### Task 8: Whole-repo verification and PR write-up

**Files:**
- Modify: none (verification only). Any fix required here belongs to the task that introduced the
  problem — go back and amend there rather than patching over it.

**Interfaces:**
- Consumes: everything from Tasks 1–7.
- Produces: the PR description text.

- [ ] **Step 1: Run the full CI target**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22   # only if npm is missing
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, and
`carfax-template` all clean. `make test` must report **0 failures**, with at least the 37 packages
that passed at baseline plus the new ones (`entityguard`, and the four `cmd` packages).

Capture the `make test` summary line — the PR description quotes it.

- [ ] **Step 2: Verify every `Save` site is covered by a test**

```sh
git grep -n "db.Save\|tx.Save" -- 'apps/**/*.go'
```

Expected: exactly these five lines, and no others:

```
apps/auth-service/internal/user/administrator.go:26
apps/fleet-service/internal/fleet/administrator.go:27
apps/fleet-service/internal/vehicle/administrator.go:65
apps/media-service/internal/mediaobject/administrator.go:35
apps/media-service/internal/mediaobject/administrator.go:47
```

Each is covered by a behavioural test from Tasks 2–5. A sixth line means a new write path appeared
mid-task and needs the same treatment.

- [ ] **Step 3: Verify no schema or API change slipped in**

```sh
git diff main --stat
git diff main -- apps/web/            # must be empty
git diff main -- '*/resource.go' '*/rest.go'   # must be empty
```

Expected: the changed-file list matches `context.md`'s table exactly. No `apps/web` change, no
transport-layer change. `<-:create` takes no part in DDL generation, so `AutoMigrate` output is
unchanged — verified structurally by the absence of any column-definition change in the diff.

- [ ] **Step 4: Confirm the guard's allowlist did not grow**

```sh
git grep -n "autoManaged" -- packages/shared-go/
```

Expected: the map contains exactly one entry, `UpdatedAt`. Any additional entry needs a comment
explaining why GORM manages that field, and is a decision worth surfacing in review.

- [ ] **Step 5: Write the PR description**

It must contain, at minimum:

1. **The user-visible behavioural change, called out explicitly.** `GET /vehicles` and
   `GET /vehicles/:id` return a derived `status`. Vehicles that were edited and have no activity
   history currently report `"Inactive"`; after this change they report their true status
   (`"Healthy"`). This is the intended fix, not a regression, but it is visible and must not
   surprise anyone.
2. **The guard's observed failure output**, copied verbatim from Task 6 Step 4 — both the analyzer
   message and the behavioural test messages. A guard nobody has seen fail is an untested guard.
3. **The auth-service pin's observed failure output**, from Task 5 Step 3.
4. **The deploy ordering constraint:** code fix first, `docs/runbooks/created-at-repair.md` second.
   Repairing first lets the next write re-zero the row.
5. **The follow-ups this task deliberately does not do:**
   - Narrowing `Administrator.Update` to `db.Model(...).Select(cols).Updates(...)` across all four
     services — the root fix for the defect class, excluded by PRD §2 because it changes write
     semantics repo-wide and deserves its own task.
   - A *new service* is not covered by the guard (design §6.4) — accepted deliberately, because a
     new Go service is a reviewed event while a new domain package is routine.
   - The seven latent domains are not pre-emptively tagged (design §7) — with the analyzer in place
     tagging them adds no safety, since none has a `Save`.
   - The SQLite-vs-Postgres harness divergence noted in issue #7 is untouched; design §2's V3 only
     establishes that SQLite reproduces *this* defect faithfully.
6. **That issue #7 can be closed** — both the reported defect and its stated "wider concern" are
   resolved.

- [ ] **Step 6: Run the code review before opening the PR**

Per `CLAUDE.md`, this is not optional even when the plan looks complete:

```
superpowers:requesting-code-review
```

It dispatches `plan-adherence-reviewer` and `backend-guidelines-reviewer` (Go files changed; no
frontend files changed, so the frontend reviewer does not apply). Address findings before opening
the PR.

---

## Acceptance Criteria Traceability

Every checkbox in PRD §10, mapped to the task that satisfies it.

| PRD acceptance criterion | Task |
|---|---|
| `vehicle.Entity.CreatedAt` excluded from UPDATE, proven by a persist→update→re-read test | Task 2, Steps 1–4 |
| `DeriveStatus` test proves an updated, activity-free vehicle derives "Healthy" | Task 2, `TestDeriveStatus_editedVehicleWithoutActivityStaysHealthy` |
| `fleet.Entity.CreatedAt` excluded from UPDATE; rename test asserts survival | Task 3, `TestUpdate_preservesCreatedAtAcrossRename` |
| `fleet.Entity.DeletedAt` no longer clobbered, per the Q2 resolution, with a test | Task 3, `TestUpdate_doesNotResurrectASoftDeletedFleet` |
| `mediaobject` hardened; both `Update` and `UpdateInTx` covered; no behaviour change | Task 4, Steps 2 and 4 |
| `auth-service/internal/user` regression test pinning `<-:create` | Task 5 |
| A guard fails when a lossy round-trip acquires a `Save` write path | Task 1 + Task 6 |
| The failure message names the domain and the dropped column (FR-GUARD-2) | Task 1, `TestFindingString_namesDomainColumnAndBothRemedies` |
| The guard covers all seven latent domains (PRD §4.2) | Task 6 — `Analyze("../internal")` walks every package, no registration |
| Verified by deliberately introducing a lossy `Save` and observing the guard fail, then reverting | Task 6, Step 4 |
| Repair procedure exists, is idempotent, touches only zero `created_at` rows | Task 7, Pass 3 |
| Reports repaired and unrecoverable counts per table | Task 7, Passes 1 and 4 |
| Runbook states what each proxy does and does not mean | Task 7, "What a repaired value means" |
| Deploy-then-repair ordering documented | Task 7, "Before you start"; Task 8, Step 5 |
| `make build` / `make test` / `make vet` / `make lint-check` clean | Task 8, Step 1 |
| `git grep "db.Save\|tx.Save"` returns only sites proven safe by test | Task 8, Step 2 |
| Issue #7 closable | Task 8, Step 5 |
