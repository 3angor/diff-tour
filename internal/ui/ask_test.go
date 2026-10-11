package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/init-co-jp/diff-tour/internal/generate"
	"github.com/init-co-jp/diff-tour/internal/git"
	"github.com/init-co-jp/diff-tour/internal/i18n"
	"github.com/init-co-jp/diff-tour/internal/render"
	"github.com/init-co-jp/diff-tour/internal/tour"
)

// oneAnswer is a Backend that answers every question with the same step and records the prompt.
type oneAnswer struct{ prompt string }

func (b *oneAnswer) Name() string { return "test" }

func (b *oneAnswer) Run(_ context.Context, _, prompt string, _ func(generate.Event)) (generate.Result, error) {
	b.prompt = prompt
	return generate.Result{Text: "=== ツアー JSON ===\n" +
		`{"version":1,"title":"a","base":"main","head":"feature","chapters":[{"title":"q","steps":[{"title":"Because","blocks":[{"code":{"file":"a.txt","lines":[1,1],"marks":[{"line":1,"text":"here"}]}},{"p":"It is so.","basis":"fact"}]}]}]}`}, nil
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func TestAskInsertsAnswerAfterCurrentStep(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	gitAt(t, dir, "2026-01-01T00:00:00Z", "init", "-q", "-b", "main")
	commitFile(t, dir, "2026-01-01T00:00:00Z", "base.txt")
	gitAt(t, dir, "2026-01-02T00:00:00Z", "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAt(t, dir, "2026-01-02T00:00:00Z", "add", "-A")
	gitAt(t, dir, "2026-01-02T00:00:00Z", "commit", "-q", "-m", "a")
	repo := git.Open(dir)

	tr := &tour.Tour{Version: 1, Title: "t", Base: "main", Head: "feature", Chapters: []tour.Chapter{{Title: "c", Steps: []tour.Step{
		{Title: "one", Asks: []string{"Why?"}, Blocks: []tour.Block{{Target: true}}},
		{Title: "two", Blocks: []tour.Block{{Target: true}}},
	}}}}
	var saved *tour.Tour
	backend := &oneAnswer{}
	v := NewViewer(render.Ctx{Tour: tr, Repo: repo}, &Asker{
		Backend: backend,
		Req:     generate.Request{Repo: repo, Base: "main", Head: "feature", Lang: i18n.EN},
		WorkDir: filepath.Join(t.TempDir(), "ask"),
		Save:    func(t *tour.Tour) error { saved = t; return nil },
	})
	v.width, v.height = 120, 40

	v.Update(key("?"))
	if v.ask.mode != askChoosing || !strings.Contains(v.View().Content, "Why?") {
		t.Fatalf("the panel should open with the suggested question")
	}
	// Typing moves to the free-text line; going back up picks the suggestion
	v.Update(key("x"))
	if string(v.ask.input) != "x" || v.ask.sel != 1 {
		t.Fatalf("typing should fill the free-text line, got %q sel %d", string(v.ask.input), v.ask.sel)
	}
	v.ask.sel = 0
	_, cmd := v.Update(key("enter"))
	if cmd == nil || v.ask.mode != askAnswering {
		t.Fatalf("enter should start answering")
	}
	for msg := range v.ask.ch {
		if done, ok := msg.(answerDoneMsg); ok {
			v.Update(done)
			break
		}
	}
	if !strings.Contains(backend.prompt, "Why?") {
		t.Errorf("the prompt should carry the question")
	}
	steps := tr.Steps()
	if len(steps) != 3 || steps[1].Title != "Because" || steps[1].Asked != "Why?" || v.index != 1 {
		t.Fatalf("the answer should be the next step and be shown, got %d steps, index %d", len(steps), v.index)
	}
	if saved == nil {
		t.Errorf("the tour with the answer should be saved")
	}
	if c := v.View().Content; !strings.Contains(c, "Your question") || !strings.Contains(c, "here") {
		t.Errorf("the answer step should show the question and the line note")
	}
}
