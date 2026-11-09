package srs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// allowedImports keeps domain and srs pure (spec S3): no I/O, network or UI.
var allowedImports = map[string]bool{
	"fmt":  true,
	"math": true,
	"time": true,
	"github.com/kevnnard/english-teacher/internal/domain": true,
}

func TestPackagesArePure(t *testing.T) {
	for _, dir := range []string{".", "../domain"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			checkPure(t, path)
		}
	}
}

func checkPure(t *testing.T, path string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if !allowedImports[p] {
			t.Errorf("%s imports %q, which is not allowed in pure packages", path, p)
		}
	}

	full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(full, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Now" {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" {
				t.Errorf("%s calls time.Now; time must be injected", path)
			}
		}
		return true
	})
}
