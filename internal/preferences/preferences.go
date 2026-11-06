// Package preferences stores paths and UI choices, never document contents.
package preferences

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type State struct {
	Workspace     string  `json:"workspace,omitempty"`
	ActiveFile    string  `json:"active_file,omitempty"`
	MarkdownMode  int     `json:"markdown_mode"`
	SidebarHidden bool    `json:"sidebar_hidden,omitempty"`
	SidebarWidth  float32 `json:"sidebar_width,omitempty"`
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
