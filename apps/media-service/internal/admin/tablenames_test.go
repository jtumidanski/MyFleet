package admin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// collectTableNames parses every non-test .go file under root and returns the
// string literal each `func (X) TableName() string` returns.
//
// It lives in its own file rather than as a closure inside the arch test so
// that the walk itself can be tested against a fixture. FR-ADMIN-5's acceptance
// criterion — "adding a table in a file not named entity.go fails the arch
// test" — is only demonstrable that way. It is a _test.go file because it has
// no production caller: arch_test.go is `package admin`, so an unexported
// helper here is just as reachable, and go/parser, go/ast and filepath.WalkDir
// have no business in the shipped binary.
//
// Two exclusions, both load-bearing:
//
//   - _test.go files are skipped. Test files carry DDL and fixtures, and
//     including them would report tables that do not exist in production.
//   - testdata directories are skipped BY NAME. The fixture proving this walk
//     sees non-entity.go declarations has to live somewhere, and if the
//     production run descended into testdata it would find the fixture's table,
//     report it as uncovered, and fail the build. The fixture test reaches it by
//     naming its path directly instead.
//
// Parsing rather than grepping: table names appear in comments, raw SQL and
// test DDL throughout the service, and a grep would produce false matches the
// first time someone documents one.
func collectTableNames(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			// A parse failure is fatal, never a silent skip: a file the walk
			// cannot read is a file whose tables it cannot see (FR-ADMIN-6).
			return fmt.Errorf("parse %s: %w", path, perr)
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
				table, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return fmt.Errorf("%s: unquote %s: %w", path, lit.Value, uerr)
				}
				found = append(found, table)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
