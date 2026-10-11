package render

import (
	"strings"
	"testing"

	"github.com/init-co-jp/diff-tour/internal/text"
	"github.com/init-co-jp/diff-tour/internal/tour"
)

func plain(lines []text.Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(strings.TrimRight(text.ANSI(l, true), " "))
		b.WriteString("\n")
	}
	return b.String()
}

// A node marked start begins a separate flow, so no arrow leads into it.
func TestFlowStartHasNoArrow(t *testing.T) {
	f := &tour.Flow{Nodes: []tour.FlowNode{
		{Label: "POST /orders", Next: "order id"},
		{Label: "save"},
		{Label: "nightly job", Start: true},
	}}
	out := plain(renderFlow(f, 60))
	if got := strings.Count(out, "▼"); got != 1 {
		t.Errorf("want 1 arrow, got %d:\n%s", got, out)
	}
	if !strings.Contains(out, "order id") || !strings.Contains(out, "┄") {
		t.Errorf("want the arrow label and a separator:\n%s", out)
	}
}

// A box with its kind written into the top border is as wide as the others, so the right edges line up.
func TestFlowBoxesLineUp(t *testing.T) {
	f := &tour.Flow{Nodes: []tour.FlowNode{{Label: "handler", Kind: "changed"}, {Label: "db", Kind: "external"}, {Label: "save"}}}
	edges := map[int]bool{}
	for _, l := range strings.Split(plain(renderFlow(f, 60)), "\n") {
		if i := strings.IndexAny(l, "┐│┘"); i >= 0 && strings.ContainsAny(l, "┐┘") {
			edges[text.Width(l[:strings.LastIndexAny(l, "┐┘")])] = true
		}
	}
	if len(edges) != 1 {
		t.Errorf("right edges at %v, want one column", edges)
	}
}

// A long line of code continues under a ↪ mark instead of being cut off.
func TestWrapCodeKeepsTheWholeLine(t *testing.T) {
	gutter := text.Line{{Text: " 7 "}, {Text: "  "}}
	code := text.Line{{Text: "const total = items.reduce((sum, item) => sum + item.price, 0) // in yen"}}
	lines := wrapCode(gutter, code, 30, "")
	if len(lines) < 2 {
		t.Fatalf("want the line wrapped, got %d line(s)", len(lines))
	}
	out := plain(lines)
	if !strings.Contains(out, "↪") {
		t.Errorf("want a continuation mark:\n%s", out)
	}
	joined := strings.ReplaceAll(out, "\n", "")
	for _, w := range []string{"const", "reduce", "in yen"} {
		if !strings.Contains(joined, w) {
			t.Errorf("lost %q:\n%s", w, out)
		}
	}
	for _, l := range lines {
		if l.Width() > 30 {
			t.Errorf("line wider than 30: %q", text.ANSI(l, true))
		}
	}
}
