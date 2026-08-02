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

// A ToEntity() whose result type is a pointer (or, equally, a qualified
// package.Type) is exactly as undecidable as an unparseable body: this guard
// cannot resolve it to a same-package struct declaration. Silently treating it
// as "no ToEntity here" would be the same silent-pass bug the unparseable-body
// case already guards against.
func TestAnalyze_pointerResultToEntityIsAFinding(t *testing.T) {
	src := strings.Replace(lossySrc, "ToEntity() Entity {", "ToEntity() *Entity {", 1)
	src = strings.Replace(src, "return Entity{ID: m.id, Name: m.name}", "return &Entity{ID: m.id, Name: m.name}", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 || got[0].Reason != ReasonUnverifiable {
		t.Fatalf("want exactly one %q finding, got %v", ReasonUnverifiable, got)
	}
	if !strings.Contains(got[0].String(), "*Entity") {
		t.Fatalf("message must name the unresolved result type; got:\n%s", got[0].String())
	}
}

// A qualified result type (pkg.Entity) shares resultTypeString's rendering
// path with the pointer case above but exercises the SelectorExpr branch
// directly, so it needs its own coverage rather than relying on the pointer
// test to stand in for it.
func TestAnalyze_qualifiedResultToEntityIsAFinding(t *testing.T) {
	src := strings.Replace(lossySrc, "ToEntity() Entity {", "ToEntity() pkg.Entity {", 1)
	src = strings.Replace(src, "return Entity{ID: m.id, Name: m.name}", "return pkg.Entity{ID: m.id, Name: m.name}", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 || got[0].Reason != ReasonUnverifiable {
		t.Fatalf("want exactly one %q finding, got %v", ReasonUnverifiable, got)
	}
	if !strings.Contains(got[0].String(), "pkg.Entity") {
		t.Fatalf("message must name the unresolved result type; got:\n%s", got[0].String())
	}
}

// FR-GUARD-2 for the arity case: a Save call site plus a ToEntity() that
// returns two results (e.g. after a routine validation refactor) is the same
// failure mode as an unresolvable result type — the guard must report it
// rather than silently pass the package as clean.
func TestAnalyze_twoResultToEntityIsAFinding(t *testing.T) {
	src := strings.Replace(lossySrc, "ToEntity() Entity {", "ToEntity() (Entity, error) {", 1)
	src = strings.Replace(src,
		"return Entity{ID: m.id, Name: m.name}",
		"return Entity{ID: m.id, Name: m.name}, nil", 1)
	got, err := Analyze(writeFixture(t, "widget", src))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(got) != 1 || got[0].Reason != ReasonUnverifiable {
		t.Fatalf("want exactly one %q finding, got %v", ReasonUnverifiable, got)
	}
	if !strings.Contains(got[0].String(), "exactly one result") {
		t.Fatalf("message must say why the arity defeats analysis; got:\n%s", got[0].String())
	}
}
