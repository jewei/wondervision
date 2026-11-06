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

	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/search"
)

// goFiles parses the non-test Go files under internal/<dir>.
func goFiles(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	files := make(map[string]*ast.File)
	err := filepath.WalkDir(filepath.Join("..", dir), func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		files[filepath.ToSlash(strings.TrimPrefix(path, "../"))] = f
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// search is shared with a future non-UI tool, so it must stay ordinary Go.
func TestSearchImportsOnlyTheStandardLibrary(t *testing.T) {
	for path, f := range goFiles(t, "search") {
		for _, imp := range f.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(strings.SplitN(name, "/", 2)[0], ".") {
				t.Errorf("internal/%s imports %s; internal/search must import only the standard library so non-UI tools can reuse it (pass UI data in through search.Text)", path, name)
			}
		}
	}
}

// Command shortcuts come from the command table, so they can't drift from
// the menu, toolbar and palette.
func TestShortcutsAreBoundOnlyFromTheCommandTable(t *testing.T) {
	allowed := map[string]string{
		"app/commands.go:bindShortcuts": "binds every command's shortcut from the table",
		"app/overlay.go:listKeys":       "moves an overlay's list selection with Up and Down",
	}
	for path, f := range goFiles(t, "app") {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "OnShortcut" {
					if _, ok := allowed[path+":"+fn.Name.Name]; !ok {
						t.Errorf("internal/%s: %s calls OnShortcut; give the action a commandID in internal/app/commands.go (title, shortcut, available, run) so menu, toolbar and palette stay in step", path, fn.Name.Name)
					}
				}
				return true
			})
		}
	}
}

// The ignore list has one owner, so Quick Open and content search agree.
func TestOneIgnoreList(t *testing.T) {
	for _, dir := range []string{"app", "documents", "workspace", "preferences", "search"} {
		for path, f := range goFiles(t, dir) {
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "node_modules") && path != "search/walk.go" {
					t.Errorf("internal/%s lists ignored directories; use the single list in internal/search/walk.go (ignoredDirs)", path)
				}
				return true
			})
		}
	}
}

// Scans and searches run on their own goroutines: on the serial file worker
// they would make saves wait behind them. Only the job body is checked; the
// apply closure it returns runs on the UI thread.
func TestSearchWorkNeverRunsOnTheSerialWorker(t *testing.T) {
	for path, f := range goFiles(t, "app") {
		var check func(n ast.Node) bool
		check = func(n ast.Node) bool {
			if ret, ok := n.(*ast.ReturnStmt); ok {
				for _, r := range ret.Results {
					if _, apply := r.(*ast.FuncLit); !apply {
						ast.Inspect(r, check)
					}
				}
				return false
			}
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if (ok && pkg.Name == "search") || sel.Sel.Name == "Find" || sel.Sel.Name == "Wait" {
				t.Errorf("internal/%s: %s is called inside a.enqueue; run search work on its own goroutine (see startTextSearch) so saves never wait behind it", path, sel.Sel.Name)
			}
			return true
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "enqueue" {
				for _, arg := range call.Args {
					ast.Inspect(arg, check)
				}
			}
			return true
		})
	}
}

func TestSearchSizeLimitMatchesDocuments(t *testing.T) {
	if search.MaxFileSize != documents.MaxFileSize {
		t.Fatalf("search.MaxFileSize = %d but documents.MaxFileSize = %d; content search must skip exactly the files the editor refuses", search.MaxFileSize, documents.MaxFileSize)
	}
}
