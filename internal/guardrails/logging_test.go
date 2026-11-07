package guardrails

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

// Spec §8/§13: no document content, search snippet or secret value may reach
// a log. The simplest way to keep that true is to have no logging at all:
// internal packages must not import a log package or print to stdout/stderr.
// cmd/wondervision/main.go may report a startup error, which never holds
// document content.
func TestNoLoggingOutsideStartupErrors(t *testing.T) {
	err := filepath.WalkDir("../..", func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && (e.Name() == ".git" || e.Name() == "build" || e.Name() == "scripts") {
			return filepath.SkipDir
		}
		if e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "../../"))
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if name == "log" || name == "log/slog" || name == "log/syslog" {
				t.Errorf("%s imports %s; WonderVision keeps no logs so no document content or secret can leak into one (spec §8). Show errors with a.notify instead", rel, name)
			}
		}
		if rel == "cmd/wondervision/main.go" {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case pkg.Name == "fmt" && (sel.Sel.Name == "Print" || sel.Sel.Name == "Printf" || sel.Sel.Name == "Println"),
				pkg.Name == "fmt" && strings.HasPrefix(sel.Sel.Name, "Fprint") && len(call.Args) > 0 && writesToStdStream(call.Args[0]),
				pkg.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr"):
				t.Errorf("%s prints with %s.%s; WonderVision keeps no logs (spec §8). Show errors with a.notify instead", rel, pkg.Name, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writesToStdStream(arg ast.Expr) bool {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr")
}
