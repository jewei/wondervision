package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestShellSettlesAndFollowsTheme(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	tt := ui.NewTester(a.view, 1040, 720)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		if !tt.HasText("EXPLORER") || !tt.HasText("Ready") {
			t.Fatal("shell is incomplete")
		}
		tt.Advance(time.Minute)
		if tt.FramePending() {
			t.Fatal("untouched shell keeps requesting frames")
		}
	}
	a.notify(errors.New("Unable to open folder"))
	tt.Frame()
	if !tt.HasText(a.err) {
		t.Fatal("error notification missing")
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	if a.err != "" {
		t.Fatal("error notification did not dismiss")
	}
}
