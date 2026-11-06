package search

import (
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Score levels. A match in a higher level always outranks one in a lower level.
const (
	scoreExact     = 1000
	scorePrefix    = 800
	scoreWordStart = 600
	scoreSubstring = 400
	scoreFuzzy     = 100 // up to scoreSubstring-1 with bonuses
)

// Score rates how well query matches text, ignoring case: 0 for no match,
// otherwise exact > prefix > substring at a word start > substring >
// subsequence. An empty query matches everything with score 1. It is the one
// fuzzy primitive: Rank and the command palette both use it.
func Score(query, text string) int {
	if query == "" {
		return 1
	}
	q, t := strings.ToLower(query), strings.ToLower(text)
	switch {
	case t == q:
		return scoreExact
	case strings.HasPrefix(t, q):
		return scorePrefix
	}
	if i := strings.Index(t, q); i >= 0 {
		for j := i; j >= 0; j = indexFrom(t, q, j+1) {
			if wordStart(t, j) {
				return scoreWordStart
			}
		}
		return scoreSubstring - min(i, 100)
	}
	return fuzzy(q, t)
}

// indexFrom is strings.Index starting at byte from, or -1.
func indexFrom(s, sub string, from int) int {
	if from > len(s) {
		return -1
	}
	if i := strings.Index(s[from:], sub); i >= 0 {
		return from + i
	}
	return -1
}

// wordStart reports whether byte i of s starts a word: the start of s, or
// after a separator such as space, '-', '_', '.', '/'.
func wordStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// fuzzy matches q as a subsequence of t, rewarding consecutive runs and
// matches at word starts. It returns 0 when q is not a subsequence.
func fuzzy(q, t string) int {
	score, run, ti := scoreFuzzy, 0, 0
	for _, qr := range q {
		found := false
		for ti < len(t) {
			tr, size := utf8.DecodeRuneInString(t[ti:])
			start := ti
			ti += size
			if tr == qr {
				run++
				score += run
				if wordStart(t, start) {
					score += 10
				}
				found = true
				break
			}
			run = 0
		}
		if !found {
			return 0
		}
	}
	return min(score, scoreSubstring-1)
}

// Rank returns up to limit of the candidate paths ('/'-separated) that match
// query, best first. A match in the file name beats a match only in the
// directory; recent paths (most recent first) get a boost and, for an empty
// query, lead the list in their own order. Ties go to the shorter path, then
// the lexically smaller one.
func Rank(query string, candidates, recent []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	recentRank := make(map[string]int, len(recent))
	for i, p := range recent {
		if _, ok := recentRank[p]; !ok {
			recentRank[p] = i
		}
	}
	type scored struct {
		path  string
		score int
	}
	var out []scored
	for _, c := range candidates {
		s := Score(query, path.Base(c))
		if s > 0 {
			s += 2 * scoreExact // any file-name match beats a directory-only match
		} else if s = Score(query, c); s == 0 {
			continue
		}
		if i, ok := recentRank[c]; ok {
			if query == "" {
				s += 10*scoreExact - i
			} else {
				s += 150 - min(i, 100)
			}
		}
		out = append(out, scored{c, s})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if len(a.path) != len(b.path) {
			return len(a.path) < len(b.path)
		}
		return a.path < b.path
	})
	if len(out) > limit {
		out = out[:limit]
	}
	paths := make([]string, len(out))
	for i, s := range out {
		paths[i] = s.path
	}
	return paths
}
