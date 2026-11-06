package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "report.md", "# Report\nQuarterly budget\n")
	write(t, root, "todo.md", "- [ ] review budget\n")
	write(t, root, ".env", "API_KEY=secret\n")
	write(t, root, "docs/next.md", "Next steps\n")
	write(t, root, "notes/deep/inner/report-draft.md", "draft\n")
	write(t, root, "node_modules/pkg/report.md", "ignored budget\n")
	write(t, root, ".git/config", "budget\n")
	write(t, root, "build/out.md", "budget\n")
	return root
}

func TestScoreLevels(t *testing.T) {
	order := []string{"report.md", "report.md.bak", "my-report.md", "myreport.md", "r_e_p_o_r_t.md"}
	q := "report.md"
	for i := 1; i < len(order); i++ {
		if a, b := Score(q, order[i-1]), Score(q, order[i]); a <= b {
			t.Errorf("Score(%q, %q)=%d should beat Score(%q, %q)=%d", q, order[i-1], a, q, order[i], b)
		}
	}
	if Score("xyz", "report.md") != 0 {
		t.Error("non-match scored")
	}
	if Score("REP", "report.md") != scorePrefix {
		t.Error("score is not case-insensitive")
	}
	if Score("", "anything") != 1 {
		t.Error("empty query should match everything")
	}
}

func TestRankPrefersFileNameAndRecent(t *testing.T) {
	paths := []string{"reports/summary.md", "notes/report.md", "a/b/report-draft.md", "zzz.txt"}
	got := Rank("report", paths, nil, 10)
	want := []string{"notes/report.md", "a/b/report-draft.md", "reports/summary.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rank = %v, want %v", got, want)
	}
	if got := Rank("report", paths, []string{"a/b/report-draft.md"}, 10); got[0] != "a/b/report-draft.md" {
		t.Fatalf("recent file not boosted: %v", got)
	}
	if got := Rank("", paths, []string{"zzz.txt", "notes/report.md"}, 2); !reflect.DeepEqual(got, []string{"zzz.txt", "notes/report.md"}) {
		t.Fatalf("empty query should list recent files first, in order: %v", got)
	}
	if got := Rank("report", paths, nil, 1); len(got) != 1 {
		t.Fatalf("limit ignored: %v", got)
	}
}

func TestIndexScansLazilyWithIgnoreAndSymlinkPolicy(t *testing.T) {
	root := workspace(t)
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("todo.md", filepath.Join(root, "linked-todo.md")); err != nil {
		t.Fatal(err)
	}
	ix := NewIndex(context.Background(), root, nil)
	if err := ix.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ix.Find("", nil, 100).Paths
	// An empty query ranks everything equally: shorter paths first, then by name.
	// The symlinked directory is not entered; the relative file symlink is kept.
	want := []string{".env", "todo.md", "report.md", "docs/next.md", "linked-todo.md", "notes/deep/inner/report-draft.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if got := ix.Find("innerdraft", nil, 5).Paths; len(got) != 1 || got[0] != "notes/deep/inner/report-draft.md" {
		t.Fatalf("nested fuzzy match failed: %v", got)
	}
	abs := filepath.Join(root, "docs", "next.md")
	if got := ix.Find("", []string{abs, "/elsewhere/x.md"}, 1).Paths; got[0] != "docs/next.md" {
		t.Fatalf("recent absolute path not used: %v", got)
	}
}

func TestIndexServesOldPathsUntilRescanCompletes(t *testing.T) {
	root := workspace(t)
	changed := make(chan struct{}, 100)
	ix := NewIndex(context.Background(), root, func() { changed <- struct{}{} })
	if f := ix.Find("", nil, 100); !f.Scanning {
		t.Fatal("first Find did not start a scan")
	}
	if err := ix.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := len(ix.Find("", nil, 100).Paths)
	write(t, root, "fresh.md", "new\n")
	if got := len(ix.Find("", nil, 100).Paths); got != before {
		t.Fatalf("cache changed without Invalidate: %d -> %d", before, got)
	}
	ix.Invalidate()
	f := ix.Find("fresh", nil, 100)
	if !f.Scanning {
		t.Fatal("Find after Invalidate did not rescan")
	}
	if err := ix.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := ix.Find("fresh", nil, 100).Paths; len(got) != 1 || got[0] != "fresh.md" {
		t.Fatalf("rescan missed the new file: %v", got)
	}
}

func TestIndexDoesNotRetryFailedScanUntilInvalidate(t *testing.T) {
	ix := NewIndex(context.Background(), filepath.Join(t.TempDir(), "missing"), nil)
	if err := ix.Wait(context.Background()); err == nil {
		t.Fatal("scan of a missing root succeeded")
	}
	for range 3 {
		if f := ix.Find("", nil, 10); f.Scanning || f.Err == nil {
			t.Fatalf("failed scan restarted on Find: %+v", f)
		}
	}
	ix.Invalidate()
	if f := ix.Find("", nil, 10); f.Err != nil {
		t.Fatalf("Invalidate kept the old error: %+v", f)
	}
}

func TestIndexStopsWithItsContext(t *testing.T) {
	root := workspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ix := NewIndex(ctx, root, nil)
	if f := ix.Find("", nil, 10); f.Scanning {
		t.Fatal("cancelled Index started a scan")
	}
	if err := ix.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want context.Canceled", err)
	}
}

type text []string

func (t text) LineCount() int    { return len(t) }
func (t text) Line(i int) string { return t[i] }

func grep(t *testing.T, root string, q Query, unsaved map[string]Text) ([]Hit, Summary) {
	t.Helper()
	var hits []Hit
	sum, err := Grep(context.Background(), root, q, unsaved, func(b []Hit) { hits = append(hits, b...) })
	if err != nil {
		t.Fatal(err)
	}
	return hits, sum
}

func TestGrepFindsLinesInPathOrderAndSkipsIgnoredDirs(t *testing.T) {
	root := workspace(t)
	hits, sum := grep(t, root, Query{Text: "BUDGET"}, nil)
	var got []string
	for _, h := range hits {
		got = append(got, fmt.Sprintf("%s:%d:%d:%s", h.Path, h.Line, h.Column, h.Excerpt[h.Start:h.End]))
	}
	want := []string{"report.md:2:10:budget", "todo.md:1:13:budget"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hits = %v, want %v", got, want)
	}
	if sum.Hits != 2 || sum.Truncated {
		t.Fatalf("summary = %+v", sum)
	}
	if hits, _ := grep(t, root, Query{Text: "BUDGET", CaseSensitive: true}, nil); len(hits) != 0 {
		t.Fatalf("case-sensitive search matched: %v", hits)
	}
}

func TestGrepSearchesUnsavedBufferInsteadOfDisk(t *testing.T) {
	root := workspace(t)
	unsaved := map[string]Text{filepath.Join(root, "todo.md"): text{"- [x] nothing here", "unsaved budget line"}}
	hits, _ := grep(t, root, Query{Text: "budget"}, unsaved)
	var todo []Hit
	for _, h := range hits {
		if h.Path == "todo.md" {
			todo = append(todo, h)
		}
	}
	if len(todo) != 1 || todo[0].Line != 2 || todo[0].Excerpt != "unsaved budget line" {
		t.Fatalf("unsaved buffer not searched instead of disk: %+v", todo)
	}
}

func TestGrepSkipsBinaryInvalidAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "needle\n")
	write(t, root, "b.bin", "needle\x00\x01\x02")
	write(t, root, "c.txt", "needle\n\xff\xfe broken\n")
	hits, sum := grep(t, root, Query{Text: "needle"}, nil)
	if len(hits) != 1 || hits[0].Path != "a.txt" || sum.Skipped != 2 {
		t.Fatalf("hits=%v summary=%+v", hits, sum)
	}
}

func TestGrepFoldsNonASCIIAndTrimsLongLines(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("x", 500) + "ÄRGER here" + strings.Repeat("y", 500)
	write(t, root, "a.txt", long+"\n")
	hits, _ := grep(t, root, Query{Text: "ärger"}, nil)
	if len(hits) != 1 {
		t.Fatalf("hits = %v", hits)
	}
	h := hits[0]
	if h.Column != 500 || h.Excerpt[h.Start:h.End] != "ÄRGER" || len(h.Excerpt) > excerptMax+4 {
		t.Fatalf("bad hit: col=%d match=%q excerpt=%d bytes", h.Column, h.Excerpt[h.Start:h.End], len(h.Excerpt))
	}
}

func TestGrepStopsAtLimitWithExactCount(t *testing.T) {
	root := t.TempDir()
	for i := range 40 {
		write(t, root, fmt.Sprintf("f%02d.txt", i), strings.Repeat("hit\n", 20))
	}
	hits, sum := grep(t, root, Query{Text: "hit", Limit: 500}, nil)
	if len(hits) != 500 || sum.Hits != 500 || !sum.Truncated {
		t.Fatalf("got %d hits, summary %+v", len(hits), sum)
	}
	if hits[0].Path != "f00.txt" || hits[499].Path != "f24.txt" || hits[499].Line != 20 {
		t.Fatalf("truncation is not in path order: first=%+v last=%+v", hits[0], hits[499])
	}
}

func TestGrepCancellationReturnsPromptlyWithoutLeaks(t *testing.T) {
	root := t.TempDir()
	for i := range 300 {
		write(t, root, fmt.Sprintf("d%d/f%d.txt", i%10, i), strings.Repeat("some text line\n", 200))
	}
	base := runtime.NumGoroutine()
	for range 50 {
		ctx, cancel := context.WithCancel(context.Background())
		emitted := 0
		_, err := Grep(ctx, root, Query{Text: "text"}, nil, func(b []Hit) {
			emitted += len(b)
			cancel()
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Grep after cancel = %v", err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("goroutines leaked: %d before, %d after", base, n)
	}
}

// BenchmarkRank10k checks the spec's Quick Open budget (<=100 ms for a cached
// query) with ten times the reference workload.
func BenchmarkRank10k(b *testing.B) {
	paths := make([]string, 10000)
	for i := range paths {
		paths[i] = fmt.Sprintf("area%02d/section%03d/note-%05d.md", i%50, i%200, i)
	}
	recent := []string{paths[17], paths[4242]}
	b.ResetTimer()
	for range b.N {
		for _, q := range []string{"n", "note-04", "sec12note", "area07/section"} {
			Rank(q, paths, recent, 200)
		}
	}
}
