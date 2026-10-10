package ui

import (
	"strings"
	"testing"
)

func TestEditorCommand(t *testing.T) {
	cases := map[string]string{
		"nvim":                           "nvim +12 a.go",
		"omarchy-launch-editor --inline": "omarchy-launch-editor --inline +12 a.go",
		"code --wait":                    "code --wait -g a.go:12",
		"hx":                             "hx a.go:12",
	}
	for editor, want := range cases {
		t.Setenv("TOUR_EDITOR", "")
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", editor)
		if got := strings.Join(EditorCommand("a.go", 12), " "); got != want {
			t.Errorf("EDITOR=%q: got %q, want %q", editor, got, want)
		}
	}
	t.Setenv("TOUR_EDITOR", "myedit --line={line} {file}")
	if got := strings.Join(EditorCommand("a.go", 12), " "); got != "myedit --line=12 a.go" {
		t.Errorf("TOUR_EDITOR: got %q", got)
	}
}
