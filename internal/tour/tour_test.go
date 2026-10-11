package tour

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func parse(t *testing.T, s string) *Tour {
	t.Helper()
	var tr Tour
	if err := json.Unmarshal([]byte(s), &tr); err != nil {
		t.Fatal(err)
	}
	return &tr
}

func TestValidate(t *testing.T) {
	ok := `{"version":1,"title":"x","base":"main","head":"f","chapters":[{"title":"c","steps":[{"title":"s","blocks":[{"p":"hi","basis":"fact"}]}]}]}`
	if err := parse(t, ok).Validate(); err != nil {
		t.Errorf("valid tour: %v", err)
	}
	bad := map[string]string{
		"version":   strings.Replace(ok, `"version":1`, `"version":2`, 1),
		"two kinds": strings.Replace(ok, `{"p":"hi","basis":"fact"}`, `{"p":"hi","pre":"x"}`, 1),
		"basis":     strings.Replace(ok, `"basis":"fact"`, `"basis":"maybe"`, 1),
		"no steps":  `{"version":1,"title":"x","base":"main","head":"f","chapters":[]}`,
	}
	for name, s := range bad {
		if err := parse(t, s).Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestInsertAfter(t *testing.T) {
	tr := &Tour{Chapters: []Chapter{{Steps: []Step{{Title: "a"}, {Title: "b"}}}, {Steps: []Step{{Title: "c"}}}}}
	tr.InsertAfter(1, []Step{{Title: "x"}})
	tr.InsertAfter(3, []Step{{Title: "y"}})
	var got []string
	for _, s := range tr.Steps() {
		got = append(got, s.Title)
	}
	if strings.Join(got, "") != "abxcy" {
		t.Errorf("got %v", got)
	}
	if len(tr.Chapters[0].Steps) != 3 {
		t.Errorf("x should join the first chapter")
	}
}

// Asks on a question step and on the step before it are dropped, even across a chapter boundary.
func TestDropRevealingAsks(t *testing.T) {
	tr := parse(t, `{"chapters": [
		{"title": "a", "steps": [{"title": "1", "asks": ["keep"], "blocks": []}, {"title": "2", "asks": ["drop"], "blocks": []}]},
		{"title": "b", "steps": [{"title": "3", "question": true, "asks": ["drop"], "blocks": []}, {"title": "4", "asks": ["keep"], "blocks": []}]}
	]}`)
	tr.DropRevealingAsks()
	var got []int
	for _, s := range tr.Steps() {
		got = append(got, len(s.Asks))
	}
	if want := []int{1, 0, 0, 1}; !slices.Equal(got, want) {
		t.Errorf("asks per step = %v, want %v", got, want)
	}
}
