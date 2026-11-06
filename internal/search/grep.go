package search

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	// DefaultLimit is the number of hits Grep returns when Query.Limit is 0.
	DefaultLimit = 500
	readers      = 4   // fixed reader goroutines per Grep
	window       = 64  // files in flight or waiting to be emitted in order
	emitBatch    = 128 // hits per emit when results are flowing fast
	excerptMax   = 200 // bytes of line kept around a match
)

// Query is one content search. Text is a literal substring; there is no regex.
type Query struct {
	Text          string
	CaseSensitive bool
	Limit         int // maximum hits; 0 means DefaultLimit
}

// Hit is one matching line.
type Hit struct {
	Path       string // relative to the root, '/'-separated
	Line       int    // 1-based
	Column     int    // 0-based byte offset of the match in the line
	Excerpt    string // the line, trimmed around the match, valid UTF-8
	Start, End int    // the match's byte range within Excerpt
}

// Summary describes a finished Grep.
type Summary struct {
	Hits      int
	Files     int  // files searched
	Skipped   int  // unreadable, binary, invalid UTF-8, or larger than MaxFileSize
	Truncated bool // stopped at the limit; more matches may exist
}

// Text is an open document's current contents, searched instead of the disk.
// editor.Snapshot satisfies it. It must be safe to read from another
// goroutine and must not change while Grep runs.
type Text interface {
	LineCount() int
	Line(i int) string // 0-based, without the line ending
}

// Grep searches every non-ignored regular file under root for q, reading
// unsaved[absolute path] instead of the disk when present. It calls emit on
// the calling goroutine, with hits in path order, in batches. It returns after
// every goroutine it started has exited: when the search is finished, cut at
// the limit (Summary.Truncated), or cancelled (ctx.Err()).
func Grep(ctx context.Context, root string, q Query, unsaved map[string]Text, emit func([]Hit)) (Summary, error) {
	var sum Summary
	if q.Text == "" {
		return sum, errors.New("empty search")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return sum, err
	}
	defer r.Close()
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type job struct {
		seq int
		rel string
	}
	type result struct {
		seq     int
		hits    []Hit
		skipped bool
	}
	slots := make(chan struct{}, window)
	jobs := make(chan job, readers)
	out := make(chan result, readers)

	var walkErr error
	walked := make(chan struct{})
	go func() {
		defer close(walked)
		defer close(jobs)
		seq := 0
		walkErr = walk(ctx, r, func(rel string) error {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case jobs <- job{seq, rel}:
				seq++
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				hits, skipped := searchFile(ctx, r, root, j.rel, unsaved, q)
				select {
				case out <- result{j.seq, hits, skipped}:
				case <-ctx.Done():
				}
			}
		}()
	}
	go func() { wg.Wait(); close(out) }()

	pending := make(map[int]result)
	next := 0
	var batch []Hit
	for res := range out {
		if sum.Truncated {
			continue // drain until the readers have exited
		}
		pending[res.seq] = res
		for {
			p, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			next++
			<-slots
			if p.skipped {
				sum.Skipped++
				continue
			}
			sum.Files++
			for _, h := range p.hits {
				if sum.Hits == limit {
					sum.Truncated = true
					cancel()
					break
				}
				batch = append(batch, h)
				sum.Hits++
			}
			if sum.Truncated {
				break
			}
		}
		if len(batch) >= emitBatch || (len(batch) > 0 && len(out) == 0) {
			emit(batch)
			batch = nil
		}
	}
	if len(batch) > 0 {
		emit(batch)
	}
	<-walked
	if err := parent.Err(); err != nil {
		return sum, err
	}
	if walkErr != nil && !sum.Truncated {
		return sum, walkErr
	}
	return sum, nil
}

// searchFile returns the hits in one file, or skipped for a file that can't be
// read as UTF-8 text within MaxFileSize.
func searchFile(ctx context.Context, r *os.Root, root, rel string, unsaved map[string]Text, q Query) ([]Hit, bool) {
	if t, ok := unsaved[filepath.Join(root, filepath.FromSlash(rel))]; ok {
		var hits []Hit
		for i := range t.LineCount() {
			if i%1024 == 0 && ctx.Err() != nil {
				return nil, false
			}
			hits = appendHits(hits, rel, i+1, t.Line(i), q)
		}
		return hits, false
	}
	f, err := r.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, true
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
		return nil, true
	}
	br := bufio.NewReaderSize(f, 64<<10)
	if head, _ := br.Peek(8 << 10); bytes.IndexByte(head, 0) >= 0 {
		return nil, true // binary
	}
	var hits []Hit
	for n := 1; ; n++ {
		if n%1024 == 0 && ctx.Err() != nil {
			return nil, false
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if n == 1 {
				line = strings.TrimPrefix(line, "\uFEFF")
			}
			if !utf8.ValidString(line) || strings.IndexByte(line, 0) >= 0 {
				return nil, true
			}
			hits = appendHits(hits, rel, n, line, q)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, true
			}
			return hits, false
		}
	}
}

// appendHits appends a Hit for each match of q in line.
func appendHits(hits []Hit, rel string, n int, line string, q Query) []Hit {
	for from := 0; from <= len(line); {
		i, size := index(line[from:], q)
		if i < 0 {
			break
		}
		col := from + i
		hits = append(hits, hit(rel, n, line, col, col+size))
		from = col + max(size, 1)
	}
	return hits
}

// index finds q.Text in s, returning the byte offset and byte length of the
// match in s, or -1.
func index(s string, q Query) (int, int) {
	if q.CaseSensitive {
		return strings.Index(s, q.Text), len(q.Text)
	}
	return indexFold(s, q.Text)
}

// indexFold is a case-insensitive strings.Index using Unicode simple folding.
// The match length is measured in s, which may differ from len(sub).
func indexFold(s, sub string) (int, int) {
	for i := 0; i <= len(s); {
		if n, ok := hasPrefixFold(s[i:], sub); ok {
			return i, n
		}
		if i == len(s) {
			break
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return -1, 0
}

// hasPrefixFold reports whether s starts with sub ignoring case, and how many
// bytes of s matched.
func hasPrefixFold(s, sub string) (int, bool) {
	i := 0
	for _, sr := range sub {
		if i >= len(s) {
			return 0, false
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if !equalFold(r, sr) {
			return 0, false
		}
		i += size
	}
	return i, true
}

func equalFold(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

// hit builds a Hit, trimming long lines to about excerptMax bytes around the
// match on rune boundaries.
func hit(rel string, n int, line string, start, end int) Hit {
	from, to := 0, len(line)
	if to > excerptMax {
		from = max(0, start-excerptMax/3)
		to = min(len(line), from+excerptMax)
		for from > 0 && !utf8.RuneStart(line[from]) {
			from--
		}
		for to < len(line) && !utf8.RuneStart(line[to]) {
			to++
		}
	}
	return Hit{Path: rel, Line: n, Column: start, Excerpt: line[from:to], Start: start - from, End: min(end, to) - from}
}
