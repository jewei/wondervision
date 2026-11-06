package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivatePreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wondervision", "preferences.json")
	s := State{Workspace: "/notes", ActiveFile: "/notes/.env", MarkdownMode: 2, SidebarWidth: 260, SidebarHidden: true}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != s {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("preferences should be private")
	}
	s.ActiveFile = "/notes/todo.md"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got != s {
		t.Fatalf("replace preferences: %+v %v", got, err)
	}
	os.WriteFile(path, []byte(`{"markdown_mode":99,"sidebar_width":-1}`), 0600)
	got, err = Load(path)
	if err != nil || got.MarkdownMode != 1 || got.SidebarWidth != 220 {
		t.Fatal("invalid UI settings were not normalized")
	}
}
