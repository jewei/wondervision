package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/workspace"
)

// Positive steps issue requests in user order; negative steps deliver any
// already-computed callback. This includes I/O finishing before cancellation
// while its Window.Update closure is still waiting on the UI thread.
func requestSchedules(n int, visit func([]int)) int {
	count := 0
	var walk func(int, []int, []int)
	walk = func(issued int, pending, steps []int) {
		if issued == n && len(pending) == 0 {
			count++
			visit(append([]int(nil), steps...))
			return
		}
		if issued < n {
			walk(issued+1, append(append([]int(nil), pending...), issued+1), append(steps, issued+1))
		}
		for index, id := range pending {
			rest := append([]int(nil), pending[:index]...)
			rest = append(rest, pending[index+1:]...)
			walk(issued, rest, append(steps, -id))
		}
	}
	walk(0, nil, nil)
	return count
}

func TestFileCompletionSchedules(t *testing.T) {
	root := fixture(t)
	first, second := filepath.Join(root, "report.md"), filepath.Join(root, "todo.md")
	alias := filepath.Join(root, "alias.md")
	if err := os.Symlink(first, alias); err != nil {
		t.Skip(err)
	}
	paths := []string{first, second, alias, second}
	identities := []string{first, second, first, second}
	for n := 1; n <= len(paths); n++ {
		count := requestSchedules(n, func(steps []int) {
			a := newApp()
			defer a.cancel()
			callbacks := map[int]func(){}
			known := map[string]*editor.Buffer{}
			latest := 0
			expectedActive := ""
			for _, step := range steps {
				id := step
				if id < 0 {
					id = -id
				}
				identity := identities[id-1]
				if step > 0 {
					latest = id
					alreadyOpen := known[paths[id-1]] != nil
					a.openFile(paths[id-1], root, "")
					if alreadyOpen {
						expectedActive = identity
					}
					select {
					case job := <-a.jobs:
						callbacks[id] = job()
					default:
					}
				} else if callback := callbacks[id]; callback != nil {
					callback()
					if id == latest {
						expectedActive = identity
						if _, exists := known[identity]; !exists {
							known[identity] = nil
						}
					}
				}
				if len(a.docs) != len(known) {
					t.Fatalf("stale file completion changed tabs; schedule %v step %d: got %d want %d", steps, step, len(a.docs), len(known))
				}
				if d := a.active(); (d == nil && expectedActive != "") || (d != nil && d.Path != expectedActive) {
					t.Fatalf("stale file completion changed active buffer; schedule %v step %d", steps, step)
				}
				for _, d := range a.docs {
					previous, exists := known[d.Path]
					if !exists {
						t.Fatalf("unexpected buffer; schedule %v", steps)
					}
					if previous == nil {
						known[d.Path] = d.Buffer
						d.Buffer.Edit(editor.Edit{Start: 0, Text: "retained draft\n"})
					} else if previous != d.Buffer || !d.Buffer.Modified() {
						t.Fatalf("alias discarded unsaved buffer; schedule %v", steps)
					}
				}
			}
		})
		t.Logf("file requests=%d schedules=%d", n, count)
	}
}

func TestFolderCompletionSchedules(t *testing.T) {
	var roots []string
	for range 3 {
		root, err := workspace.Canonical(t.TempDir()) // the app reports resolved roots
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
	}
	for n := 1; n <= len(roots); n++ {
		count := requestSchedules(n, func(steps []int) {
			a := newApp()
			defer a.cancel()
			callbacks := map[int]func(){}
			latest := 0
			expectedRoot := ""
			for _, step := range steps {
				id := step
				if id < 0 {
					id = -id
				}
				if step > 0 {
					latest = id
					a.openFolder(roots[id-1])
					callbacks[id] = (<-a.jobs)()
				} else {
					callbacks[id]()
					if id == latest {
						expectedRoot = roots[id-1]
					}
				}
				got := ""
				if a.workspace != nil {
					got = a.workspace.Root
				}
				if got != expectedRoot {
					t.Fatalf("stale folder completion changed workspace; schedule %v step %d: %s", steps, step, fmt.Sprint(got))
				}
			}
		})
		t.Logf("folder requests=%d schedules=%d", n, count)
	}
}
