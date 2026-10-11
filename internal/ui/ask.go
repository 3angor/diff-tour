package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/init-co-jp/diff-tour/internal/generate"
	"github.com/init-co-jp/diff-tour/internal/i18n"
	"github.com/init-co-jp/diff-tour/internal/text"
	"github.com/init-co-jp/diff-tour/internal/tour"
)

// Asker lets the reader ask questions while reading a tour.
type Asker struct {
	Backend generate.Backend
	Req     generate.Request
	// WorkDir is where the files for answering are exported (and deleted afterwards).
	WorkDir string
	// Save keeps the tour with its answers. Nil if answers are only kept while the tour is open.
	Save func(*tour.Tour) error
}

type askMode int

const (
	askOff askMode = iota
	askChoosing
	askAnswering
)

type askState struct {
	mode    askMode
	sel     int // index into the step's suggested questions; len(asks) is the free-text line
	input   []rune
	asked   string
	started time.Time
	reads   int
	last    string
	cancel  context.CancelFunc
	ch      chan tea.Msg
}

type answerProgressMsg generate.Progress

type answerDoneMsg struct {
	steps []tour.Step
	stats generate.Stats
	err   error
}

type answerTickMsg struct{}

func answerTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return answerTickMsg{} })
}

func (v *Viewer) suggestions() []string {
	return v.steps[v.index].Asks
}

// openAsk shows the question panel, with the suggested questions for this step and a line for free text.
func (v *Viewer) openAsk() {
	if v.asker == nil {
		v.message = i18n.T("ask.unavailable")
		return
	}
	v.ask = askState{mode: askChoosing}
}

func (v *Viewer) updateAsk(msg tea.Msg) (tea.Model, tea.Cmd) {
	a := &v.ask
	switch msg := msg.(type) {
	case answerProgressMsg:
		if msg.Event != nil && msg.Event.Kind == "read" {
			a.reads++
			a.last = msg.Event.Detail
		}
		return v, waitFor(a.ch)
	case answerTickMsg:
		if a.mode == askAnswering {
			return v, answerTick()
		}
	case answerDoneMsg:
		v.ask = askState{}
		if msg.err != nil {
			v.message = i18n.T("ask.failed", firstLine(msg.err.Error()))
			return v, nil
		}
		v.ctx.Tour.InsertAfter(v.index, msg.steps)
		v.steps = v.ctx.Tour.Steps()
		v.goTo(v.index + 1)
		v.message = i18n.T("ask.added", msg.stats.CostUSD)
		if v.asker.Save != nil {
			if err := v.asker.Save(v.ctx.Tour); err != nil {
				v.message = i18n.T("ask.notSaved", err)
			}
		}
		return v, nil
	case tea.PasteMsg:
		if a.mode == askChoosing {
			a.input = append(a.input, []rune(strings.ReplaceAll(msg.Content, "\n", " "))...)
			a.sel = len(v.suggestions())
		}
	case tea.KeyPressMsg:
		if a.mode == askAnswering {
			if s := msg.String(); s == "esc" || s == "ctrl+c" {
				a.cancel()
			}
			return v, nil
		}
		asks := v.suggestions()
		switch msg.String() {
		case "esc":
			v.ask = askState{}
		case "ctrl+c":
			return v, tea.Quit
		case "up":
			a.sel = max(0, a.sel-1)
		case "down", "tab":
			a.sel = min(len(asks), a.sel+1)
		case "backspace":
			if len(a.input) > 0 {
				a.input = a.input[:len(a.input)-1]
			}
			a.sel = len(asks)
		case "enter":
			q := strings.TrimSpace(string(a.input))
			if a.sel < len(asks) {
				q = asks[a.sel]
			}
			if q != "" {
				return v, v.startAnswer(q)
			}
		default:
			if msg.Text != "" {
				a.input = append(a.input, []rune(msg.Text)...)
				a.sel = len(asks)
			}
		}
	}
	return v, nil
}

func (v *Viewer) startAnswer(q string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan tea.Msg, 64)
	v.ask = askState{mode: askAnswering, asked: q, started: time.Now(), cancel: cancel, ch: ch}
	asker, question := v.asker, generate.Question{Tour: v.ctx.Tour, Index: v.index, Text: q}
	go func() {
		steps, stats, err := generate.Answer(ctx, asker.Backend, asker.Req, question, asker.WorkDir, func(p generate.Progress) {
			select {
			case ch <- answerProgressMsg(p):
			default:
			}
		})
		if ctx.Err() != nil {
			err = fmt.Errorf("%s", i18n.T("ask.canceled"))
		}
		ch <- answerDoneMsg{steps, stats, err}
	}()
	return tea.Batch(waitFor(ch), answerTick())
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// askPanel draws the question panel that replaces the bottom of the screen.
func (v *Viewer) askPanel(width int) []text.Line {
	a := v.ask
	rule := text.Line{{Text: strings.Repeat("─", width), Style: "35"}}
	if a.mode == askAnswering {
		elapsed := time.Since(a.started).Round(time.Second)
		lines := []text.Line{
			rule,
			text.Fit(text.Line{{Text: " " + spinner(elapsed) + " ", Style: "35"}, {Text: i18n.T("ask.answering", v.asker.Backend.Name(), elapsed)}}, width, ""),
			text.Fit(text.Line{{Text: "   " + i18n.T("ask.question") + " ", Style: "90"}, {Text: a.asked}}, width, ""),
		}
		detail := i18n.T("gen.files", a.reads)
		if a.last != "" {
			detail += "  " + a.last
		}
		return append(lines, text.Fit(text.Line{{Text: "   " + detail, Style: "90"}}, width, ""))
	}
	lines := []text.Line{rule, {{Text: " " + i18n.T("ask.title") + " ", Style: "30;45"}, {Text: "  " + i18n.T("ask.help"), Style: "90"}}}
	asks := v.suggestions()
	for i, q := range asks {
		cursor, style := "   ", ""
		if i == a.sel {
			cursor, style = " ▸ ", "1;35"
		}
		lines = append(lines, text.Fit(text.Line{{Text: cursor, Style: "35"}, {Text: q, Style: style}}, width, ""))
	}
	cursor := "   "
	if a.sel == len(asks) {
		cursor = " ▸ "
	}
	input := text.Line{{Text: cursor, Style: "35"}, {Text: "› ", Style: "35"}}
	if len(a.input) == 0 {
		input = append(input, text.Span{Text: i18n.T("ask.placeholder"), Style: "90"})
	} else {
		// Long input scrolls so the end, where the cursor is, stays visible
		shown := a.input
		for len(shown) > 0 && text.Width(string(shown)) > width-8 {
			shown = shown[1:]
		}
		if len(shown) < len(a.input) {
			input = append(input, text.Span{Text: "…", Style: "90"})
		}
		input = append(input, text.Span{Text: string(shown)}, text.Span{Text: "▏", Style: "35"})
	}
	return append(lines, text.Fit(input, width, ""))
}
