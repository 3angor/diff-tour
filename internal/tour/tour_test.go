package tour

import (
	"encoding/json"
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
