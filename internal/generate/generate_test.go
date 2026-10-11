package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/init-co-jp/diff-tour/internal/git"
	"github.com/init-co-jp/diff-tour/internal/i18n"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                          `{"a":1}`,
		"```json\n{\"a\":1}\n```":          `{"a":1}`,
		"notes first\n\n{\"a\":{\"b\":2}}": `{"a":{"b":2}}`,
		"=== 下調べ ===\n- {x} を見る\n=== ツアー JSON ===\n{\"a\":1}": `{"a":1}`,
	}
	for in, want := range cases {
		if got := string(extractJSON(in)); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
	if extractJSON("no json here") != nil {
		t.Error("expected nil")
	}
}

func TestParseReportsProblems(t *testing.T) {
	req := Request{Base: "main", Head: "feature"}
	if _, problems := parse(req, "not json"); len(problems) != 1 {
		t.Errorf("problems = %v", problems)
	}
	// Fields not in the format are treated as typos and must be fixed.
	if _, problems := parse(req, `{"version":1,"title":"x","base":"main","head":"feature","chapters":[],"extra":1}`); len(problems) != 1 || !strings.Contains(problems[0], "extra") {
		t.Errorf("problems = %v", problems)
	}
}

func TestBuildPrompt(t *testing.T) {
	for _, lang := range i18n.Langs {
		p, err := buildPrompt(Request{Base: "main", Head: "feature/x", Lang: lang})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p, languageRules[lang]) {
			t.Errorf("%s: language rule missing", lang)
		}
		if strings.Contains(p, "{{") {
			t.Errorf("%s: template left unexpanded", lang)
		}
		if !strings.Contains(p, "`feature/x`") {
			t.Errorf("%s: target missing", lang)
		}
	}
}

func TestWriteLessons(t *testing.T) {
	dir := t.TempDir()
	if err := writeLessons(dir); err != nil {
		t.Fatal(err)
	}
	// Every lesson file the prompts name must be in the workspace
	prompt, err := buildPrompt(Request{Base: "main", Head: "feature", Lang: i18n.EN})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("`(lessons/[^`]+\\.md)`").FindAllStringSubmatch(prompt, -1) {
		if _, err := os.Stat(filepath.Join(dir, m[1])); err != nil {
			t.Errorf("prompt names %s, which is not in the workspace", m[1])
		}
	}
}

// scripted is a Backend that returns canned replies in order and records the prompts it got.
type scripted struct {
	replies []string
	prompts []string
}

func (s *scripted) Name() string { return "scripted" }

func (s *scripted) Run(_ context.Context, _, prompt string, _ func(Event)) (Result, error) {
	s.prompts = append(s.prompts, prompt)
	if len(s.replies) == 0 {
		return Result{}, errors.New("no more replies")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return Result{Text: r}, nil
}

func tourReply(title string) string {
	return "=== 下調べ ===\n- notes\n" + separator + "\n" +
		`{"version":1,"title":"` + title + `","base":"main","head":"feature","chapters":[{"title":"c","steps":[{"title":"s","blocks":[{"p":"hi","basis":"fact"}]}]}]}`
}

func TestReviewStage(t *testing.T) {
	repoDir := t.TempDir()
	run(t, repoDir, "init", "-q", "-b", "main")
	write(t, filepath.Join(repoDir, "a.txt"), "a\n")
	run(t, repoDir, "add", "-A")
	run(t, repoDir, "commit", "-q", "-m", "base")
	run(t, repoDir, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(repoDir, "a.txt"), "b\n")
	run(t, repoDir, "commit", "-q", "-am", "change")
	req := Request{Repo: git.Open(repoDir), Base: "main", Head: "feature", Lang: i18n.EN}

	cases := map[string]struct {
		replies   []string
		title     string
		reviewed  bool
		callCount int
	}{
		"reviewed tour is used":     {[]string{tourReply("first"), tourReply("reviewed")}, "reviewed", true, 2},
		"broken review keeps first": {[]string{tourReply("first"), "no json", "still none", "nope"}, "first", false, 4},
		"failed review keeps first": {[]string{tourReply("first")}, "first", false, 2},
	}
	for name, c := range cases {
		b := &scripted{replies: c.replies}
		got, st, err := Generate(context.Background(), b, req, filepath.Join(t.TempDir(), "ws"), func(Progress) {})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Title != c.title || st.Reviewed != c.reviewed || len(b.prompts) != c.callCount {
			t.Errorf("%s: title %q reviewed %v calls %d", name, got.Title, st.Reviewed, len(b.prompts))
		}
		if !strings.Contains(b.prompts[1], "審査") {
			t.Errorf("%s: second call is not the review", name)
		}
	}
}
