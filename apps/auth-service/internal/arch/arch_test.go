// Package arch holds architecture tests. It deliberately contains no production
// code: the invariants here span packages, and putting them in session or oidc
// would mean one package reaching into the other's source directory. `go build
// ./...` and `go vet ./...` both skip a directory with no non-test Go files, so
// a test-only package costs nothing in the build.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestNoPrincipalLiteralOutsideResolver enforces the acceptance criterion that
// session.Principal is constructed in exactly one place —
// newPrincipalResolver in cmd/main.go — so the two token-minting paths cannot
// diverge on a claim.
//
// A composite literal in either resource file means a call site has started
// building its own principal again, which is precisely the defect this task
// fixed: the refresh path built Principal{UserID, ActiveFleetID, Role}, Go
// zero-valued the omitted Email, and every refreshed token carried
// `"email": ""` with nothing anywhere failing loudly.
//
// This parses the files rather than grepping them: `Principal{` appears in
// comments and would produce a false failure the first time someone documents
// the type.
func TestNoPrincipalLiteralOutsideResolver(t *testing.T) {
	files := []string{
		"../session/resource.go",
		"../oidc/resource.go",
	}
	for _, path := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			// A rename or move must fail this test, not silently skip the file.
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if name, ok := literalTypeName(lit.Type); ok && name == "Principal" {
				t.Errorf("%s:%d: session.Principal literal outside newPrincipalResolver — "+
					"obtain the principal from the injected PrincipalResolver instead, "+
					"so this call site cannot omit a claim",
					path, fset.Position(lit.Pos()).Line)
			}
			return true
		})
	}
}

// literalTypeName returns the bare type name of a composite literal, handling
// both the same-package form `Principal{…}` and the qualified form
// `session.Principal{…}`.
func literalTypeName(e ast.Expr) (string, bool) {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.SelectorExpr:
		return t.Sel.Name, true
	}
	return "", false
}
