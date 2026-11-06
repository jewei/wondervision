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

func TestNativeFilesystemAndIdleBoundaries(t *testing.T) {
	root := ".."
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		service := strings.Contains(path, string(filepath.Separator)+"workspace"+string(filepath.Separator)) || strings.Contains(path, string(filepath.Separator)+"preferences"+string(filepath.Separator))
		app := strings.Contains(path, string(filepath.Separator)+"app"+string(filepath.Separator))
		for _, imp := range file.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if name == "net" || (strings.HasPrefix(name, "net/") && name != "net/url") || name == "os/exec" || strings.HasPrefix(name, "database/") {
				t.Errorf("%s imports an out-of-scope network/process/database dependency %s", path, name)
			}
			if service && strings.HasPrefix(name, "github.com/egoist/mygo") {
				t.Errorf("%s couples a reusable filesystem/preferences service to MyGo", path)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := selector.X.(*ast.Ident); ok {
				if pkg.Name == "time" && (selector.Sel.Name == "NewTicker" || selector.Sel.Name == "Tick" || selector.Sel.Name == "Sleep") {
					t.Errorf("%s adds an application-owned periodic/blocking idle path", path)
				}
				if app && pkg.Name == "os" {
					switch selector.Sel.Name {
					case "Open", "OpenFile", "OpenRoot", "Create", "WriteFile", "ReadFile", "Remove", "RemoveAll", "Rename", "Mkdir", "MkdirAll", "Chmod":
						t.Errorf("%s bypasses the filesystem service boundary with os.%s", path, selector.Sel.Name)
					}
				}
				if pkg.Name == "editor" && selector.Sel.Name == "View" && !app {
					t.Errorf("%s builds an editor widget outside app", path)
				}
			}
			if app {
				if target, ok := selector.X.(*ast.SelectorExpr); ok && target.Sel.Name == "Buffer" {
					switch selector.Sel.Name {
					case "Reset", "Set", "MarkSaved", "WriteTo":
						t.Errorf("%s bypasses the document state boundary with Buffer.%s", path, selector.Sel.Name)
					}
				}
			}
			return true
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if app && selector.Sel.Name == "PrepareOverwrite" && fn.Name.Name != "confirmOverwrite" {
					t.Errorf("%s prepares an overwrite outside its confirmation boundary", path)
				}
				if app && selector.Sel.Name == "PrepareSaveAsReplace" && fn.Name.Name != "confirmReplaceSave" {
					t.Errorf("%s prepares Save As replacement outside confirmation", path)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
