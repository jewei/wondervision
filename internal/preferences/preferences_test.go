package preferences

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestPrivatePreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wondervision", "preferences.json")
	s := State{Workspace: "/notes", ActiveFile: "/notes/.env", MarkdownMode: 2, SidebarWidth: 260, SidebarHidden: true}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !got.Equal(s) {
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
	if err != nil || !got.Equal(s) {
		t.Fatalf("replace preferences: %+v %v", got, err)
	}
	os.WriteFile(path, []byte(`{"markdown_mode":99,"sidebar_width":-1}`), 0600)
	got, err = Load(path)
	if err != nil || got.MarkdownMode != 1 || got.SidebarWidth != 220 {
		t.Fatal("invalid UI settings were not normalized")
	}
}

func TestRecentFilesAreAbsoluteUniqueAndCapped(t *testing.T) {
	var recent []string
	for i := range MaxRecent + 5 {
		recent = AddRecent(recent, filepath.Join(t.TempDir(), fmt.Sprintf("f%d.md", i)))
	}
	first := recent[3]
	recent = AddRecent(recent, first)
	if len(recent) != MaxRecent || recent[0] != first || recent[4] == first {
		t.Fatalf("AddRecent did not move to front without duplicates: %v", recent[:5])
	}
	path := filepath.Join(t.TempDir(), "preferences.json")
	data := `{"recent": ["relative.md", "/abs/a.md", "/abs/a.md", "/abs/b.md"]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/abs/a.md", "/abs/b.md"}; !reflect.DeepEqual(s.Recent, want) {
		t.Fatalf("Load recent = %v, want %v", s.Recent, want)
	}
}

// Equal must compare every field; a new field needs a line in Equal.
func TestEqualComparesEveryField(t *testing.T) {
	base := State{Workspace: "/w", ActiveFile: "/w/a", MarkdownMode: 1, SidebarHidden: true, SidebarWidth: 200, Recent: []string{"/w/a"}}
	v := reflect.ValueOf(&base).Elem()
	for i := range v.NumField() {
		changed := base
		changed.Recent = slices.Clone(base.Recent)
		f := reflect.ValueOf(&changed).Elem().Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(f.String() + "x")
		case reflect.Int:
			f.SetInt(f.Int() + 1)
		case reflect.Bool:
			f.SetBool(!f.Bool())
		case reflect.Float32:
			f.SetFloat(f.Float() + 1)
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"/other"}))
		default:
			t.Fatalf("field %s has an unhandled kind; extend this test and Equal", v.Type().Field(i).Name)
		}
		if base.Equal(changed) {
			t.Errorf("Equal ignores field %s", v.Type().Field(i).Name)
		}
	}
}
