// Package preferences stores paths and UI choices, never document contents.
package preferences

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
)

type State struct {
	Workspace     string  `json:"workspace,omitempty"`
	ActiveFile    string  `json:"active_file,omitempty"`
	MarkdownMode  int     `json:"markdown_mode"`
	SidebarHidden bool    `json:"sidebar_hidden,omitempty"`
	SidebarWidth  float32 `json:"sidebar_width,omitempty"`
	// Recent holds absolute paths of recently opened files, most recent
	// first, for Quick Open. Paths only: never document contents.
	Recent []string `json:"recent,omitempty"`
}

// Equal reports whether s and o hold the same preferences.
func (s State) Equal(o State) bool {
	return s.Workspace == o.Workspace && s.ActiveFile == o.ActiveFile &&
		s.MarkdownMode == o.MarkdownMode && s.SidebarHidden == o.SidebarHidden &&
		s.SidebarWidth == o.SidebarWidth && slices.Equal(s.Recent, o.Recent)
}

// MaxRecent is how many recent files are kept.
const MaxRecent = 20

// AddRecent returns recent with path moved or added to the front, without
// duplicates and at most MaxRecent long. It never modifies recent, so a saved
// copy of a State can share the slice safely.
func AddRecent(recent []string, path string) []string {
	out := make([]string, 0, min(len(recent)+1, MaxRecent))
	out = append(out, path)
	for _, p := range recent {
		if p != path && len(out) < MaxRecent {
			out = append(out, p)
		}
	}
	return out
}

// cleanRecent drops relative paths and duplicates and caps the list.
func cleanRecent(recent []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range recent {
		if filepath.IsAbs(p) && !seen[p] && len(out) < MaxRecent {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wondervision", "preferences.json"), nil
}

func Load(path string) (State, error) {
	s := State{MarkdownMode: 1, SidebarWidth: 220}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&s)
	if s.MarkdownMode < 0 || s.MarkdownMode > 2 {
		s.MarkdownMode = 1
	}
	if s.SidebarWidth < 120 || s.SidebarWidth > 800 {
		s.SidebarWidth = 220
	}
	s.Recent = cleanRecent(s.Recent)
	return s, err
}

func Save(path string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
