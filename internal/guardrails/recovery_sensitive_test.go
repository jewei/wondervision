package guardrails

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Rule: recovery drafts are never written for sensitive paths (.env, keys,
// credentials, ...). The check lives in internal/documents, not in callers:
// documents.RecoveryPlan has only unexported fields, so a plan can only come
// from Document.RecoverySnapshot, and both RecoverySnapshot and
// RecoveryStore.Write refuse sensitive paths. App code just calls
// writeRecovery / RecoverySnapshot and needs no Sensitive check of its own.
//
// This test keeps that true:
//   - every function in internal/documents that writes to disk or builds a
//     RecoveryPlan must test Sensitive(...) in an if condition before doing so;
//   - RecoveryPlan keeps only unexported fields, so no other package can forge
//     a plan that skips RecoverySnapshot.

// Functions in internal/documents that write to disk but never write a
// recovery draft. Keys are "Receiver.func" or "func". Every entry needs a
// reason. The list may only shrink: an entry that no longer writes fails.
var sensitiveWriteAllowlist = map[string]string{
	"SavePlan.write": "explicit Save of the user's own file to the path they chose; not a recovery copy",
	"recoveryLock":   "creates the empty .lock file in the recovery dir; never holds document content",
}

// Selector names that create, open for writing, or move files.
var diskWriteOps = map[string]bool{
	"OpenFile": true, "Create": true, "CreateTemp": true, "WriteFile": true,
	"Rename": true, "Link": true, "Symlink": true,
}

const documentsDir = "../documents"

func TestRecoveryWritesCheckSensitiveFirst(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(documentsDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	writers := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			name := "package-level declaration"
			var body ast.Node = decl
			if fn, ok := decl.(*ast.FuncDecl); ok {
				name = funcName(fn)
				if fn.Body == nil {
					continue
				}
				body = fn.Body
			}
			op, opPos := firstWrite(body)
			if op == "" {
				continue
			}
			writers[name] = true
			if _, ok := sensitiveWriteAllowlist[name]; ok {
				continue
			}
			check := firstSensitiveCheck(body)
			if check.IsValid() && check < opPos {
				continue
			}
			pos := fset.Position(opPos)
			where := "never checks Sensitive(path) in an if condition first"
			if check.IsValid() {
				where = "checks Sensitive only after it, at line " + strconv.Itoa(fset.Position(check).Line) + ", which is too late"
			}
			t.Errorf("%s:%d: %s %s but %s.\n"+
				"  Rule: recovery drafts must never be written for sensitive paths (.env, *.key, *.pem, credentials, secrets).\n"+
				"  Fix: refuse before touching disk, e.g.\n"+
				"      if Sensitive(path) { return errors.New(\"recovery is disabled for sensitive filenames\") }\n"+
				"  If this function never writes document content to the recovery store, add %q to\n"+
				"  sensitiveWriteAllowlist in internal/guardrails/recovery_sensitive_test.go with a reason.",
				repoPath(pos.Filename), pos.Line, name, op, where, name)
		}
	}
	for name := range sensitiveWriteAllowlist {
		if !writers[name] {
			t.Errorf("sensitiveWriteAllowlist entry %q no longer writes to disk (or was renamed); remove it from internal/guardrails/recovery_sensitive_test.go", name)
		}
	}
}

func TestRecoveryPlanCannotBeForged(t *testing.T) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(documentsDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || spec.Name.Name != "RecoveryPlan" {
				return true
			}
			found = true
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				t.Errorf("%s:%d: RecoveryPlan must stay a struct with unexported fields so only Document.RecoverySnapshot (which refuses sensitive paths) can build one", repoPath(fset.Position(spec.Pos()).Filename), fset.Position(spec.Pos()).Line)
				return false
			}
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s:%d: RecoveryPlan embeds a field; keep every field named and unexported so other packages cannot build a plan that skips the Sensitive check in RecoverySnapshot", repoPath(fset.Position(field.Pos()).Filename), fset.Position(field.Pos()).Line)
				}
				for _, n := range field.Names {
					if n.IsExported() {
						t.Errorf("%s:%d: RecoveryPlan.%s is exported; keep it unexported so other packages cannot build a plan that skips the Sensitive check in Document.RecoverySnapshot — expose a read-only method instead", repoPath(fset.Position(n.Pos()).Filename), fset.Position(n.Pos()).Line, n.Name)
					}
				}
			}
			return false
		})
	}
	if !found {
		t.Error("internal/documents no longer defines RecoveryPlan; update internal/guardrails/recovery_sensitive_test.go to guard whatever type now carries recovery drafts")
	}
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// firstWrite returns a description and position of the earliest disk write or
// non-empty RecoveryPlan literal under node. Method values (f := root.OpenFile)
// count too, so storing the function first does not hide the write.
func firstWrite(node ast.Node) (string, token.Pos) {
	op, pos := "", token.NoPos
	record := func(desc string, p token.Pos) {
		if !pos.IsValid() || p < pos {
			op, pos = desc, p
		}
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if diskWriteOps[n.Sel.Name] {
				record("writes to disk ("+exprString(n)+")", n.Pos())
			}
		case *ast.CompositeLit:
			if id, ok := n.Type.(*ast.Ident); ok && id.Name == "RecoveryPlan" && len(n.Elts) > 0 {
				record("builds a RecoveryPlan", n.Pos())
			}
		}
		return true
	})
	return op, pos
}

// firstSensitiveCheck returns the position of the earliest Sensitive(...) call
// used inside an if condition. A bare call whose result is ignored does not count.
func firstSensitiveCheck(node ast.Node) token.Pos {
	pos := token.NoPos
	ast.Inspect(node, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		ast.Inspect(ifStmt.Cond, func(c ast.Node) bool {
			call, ok := c.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			if name == "Sensitive" && (!pos.IsValid() || call.Pos() < pos) {
				pos = call.Pos()
			}
			return true
		})
		return true
	})
	return pos
}

func exprString(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

// repoPath turns "../documents/x.go" into "internal/documents/x.go".
func repoPath(name string) string {
	return filepath.ToSlash(filepath.Join("internal", strings.TrimPrefix(filepath.ToSlash(name), "../")))
}
