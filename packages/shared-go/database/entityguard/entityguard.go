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
