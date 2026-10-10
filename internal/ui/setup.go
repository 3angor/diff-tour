package ui

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/init-co-jp/diff-tour/internal/i18n"
	"github.com/init-co-jp/diff-tour/internal/text"
)

// LLMOption is a candidate LLM for generating tours.
type LLMOption struct {
	ID        string
	Name      string
	Available bool
	// Why it can't be used, or "found"
	Status string
	// Runs on this machine (code is not sent anywhere)
	Local bool
}

// DetectLLMs looks for usable LLMs. Ones that aren't found are returned too, along with what is missing.
func DetectLLMs() []LLMOption {
	command := func(id, name, bin string) LLMOption {
		_, err := exec.LookPath(bin)
		if err != nil {
			return LLMOption{ID: id, Name: name, Status: "setup.notFound"}
		}
		return LLMOption{ID: id, Name: name, Available: true, Status: "setup.found"}
	}
	apiKey := func(id, name, env string) LLMOption {
		if os.Getenv(env) == "" {
			return LLMOption{ID: id, Name: name, Status: "setup.noKey:" + env}
		}
		return LLMOption{ID: id, Name: name, Available: true, Status: "setup.found"}
	}
	ollama := LLMOption{ID: "ollama", Name: "Ollama", Local: true, Status: "setup.notRunning"}
	client := http.Client{Timeout: 300 * time.Millisecond}
	if res, err := client.Get("http://127.0.0.1:11434/api/tags"); err == nil {
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			ollama.Available, ollama.Status = true, "setup.found"
		}
	}
	options := []LLMOption{
		command("claude-code", "Claude Code", "claude"),
		command("codex", "Codex CLI", "codex"),
		apiKey("anthropic-api", "Anthropic API", "ANTHROPIC_API_KEY"),
		apiKey("openai-api", "OpenAI API", "OPENAI_API_KEY"),
		ollama,
	}
	// For now only Claude Code can generate tours. Others are made unselectable even if found, and the user is told so
	for i := range options {
		if options[i].ID != "claude-code" {
			options[i].Status = options[i].Status + "|setup.unsupported"
			options[i].Available = false
		}
	}
	return options
}

// statusText turns a status like "setup.found|setup.unsupported" into a message.
func statusText(s string) string {
	var parts []string
	for _, key := range strings.Split(s, "|") {
		if env, ok := strings.CutPrefix(key, "setup.noKey:"); ok {
			parts = append(parts, i18n.T("setup.noKey", env))
		} else {
			parts = append(parts, i18n.T(key))
		}
	}
	return strings.Join(parts, " · ")
}

// Setup is the first-run screen for choosing the language and LLM. The recommended options are preselected, so Enter alone moves on.
type Setup struct {
	detectedFrom string
	lang         int
	llms         []LLMOption
	llm          int // -1 means nothing is usable
	focus        int // 0: language, 1: LLM
	width        int
	Done         bool
}

func NewSetup(current i18n.Lang, detectedFrom, currentLLM string, llms []LLMOption) *Setup {
	s := &Setup{detectedFrom: detectedFrom, llms: llms, llm: -1}
	for i, l := range i18n.Langs {
		if l == current {
			s.lang = i
		}
	}
	for i, o := range llms {
		if o.Available && (s.llm < 0 || o.ID == currentLLM) {
			s.llm = i
		}
	}
	i18n.Set(current)
	return s
}

func (s *Setup) Lang() i18n.Lang { return i18n.Langs[s.lang] }

func (s *Setup) LLM() string {
	if s.llm < 0 {
		return ""
	}
	return s.llms[s.llm].ID
}

func (s *Setup) Init() tea.Cmd { return nil }

// moveLLM moves the selection, skipping unusable candidates.
func (s *Setup) moveLLM(delta int) {
	for i := s.llm + delta; i >= 0 && i < len(s.llms); i += delta {
		if s.llms[i].Available {
			s.llm = i
			return
		}
	}
}

func (s *Setup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if s.focus == 0 {
				s.lang = max(0, s.lang-1)
			} else if s.llm >= 0 {
				s.moveLLM(-1)
			}
		case "down", "j":
			if s.focus == 0 {
				s.lang = min(len(i18n.Langs)-1, s.lang+1)
			} else if s.llm >= 0 {
				s.moveLLM(1)
			}
		case "enter", "right", "space":
			if s.focus == 0 {
				s.focus = 1
			} else {
				s.Done = true
				return s, tea.Quit
			}
		case "left", "backspace":
			s.focus = 0
		case "q", "ctrl+c":
			return s, tea.Quit
		}
		// when the language is changed, switch this screen's messages on the spot
		i18n.Set(s.Lang())
	}
	return s, nil
}

func (s *Setup) View() tea.View {
	width := s.width
	if width == 0 {
		width = 80
	}
	inner := width - 2
	var lines []text.Line
	add := func(l text.Line) { lines = append(lines, text.Concat(text.Line{{Text: " "}}, text.Fit(l, inner, ""))) }
	option := func(selected, focused bool, label, note, noteStyle string, enabled bool) {
		cursor, style := "  ", ""
		if selected {
			cursor = "▸ "
			if focused {
				style = "1;36"
			} else {
				style = "36"
			}
		}
		if !enabled {
			style = "90"
		}
		l := text.Line{{Text: " " + cursor, Style: "36"}, {Text: label, Style: style}}
		if note != "" {
			l = append(l, text.Span{Text: "  " + note, Style: noteStyle})
		}
		add(l)
	}

	add(nil)
	add(text.Line{{Text: " tour ", Style: "30;46"}, {Text: "  " + i18n.T("setup.welcome"), Style: "1"}})
	add(nil)
	headStyle := func(group int) string {
		if s.focus == group {
			return "1"
		}
		return "90"
	}
	add(text.Line{{Text: " " + i18n.T("setup.lang"), Style: headStyle(0)}})
	for i, l := range i18n.Langs {
		note := ""
		if l == i18n.Langs[s.lang] && s.detectedFrom != "" {
			note = i18n.T("setup.detected", s.detectedFrom)
		}
		option(i == s.lang, s.focus == 0, l.Name(), note, "90", true)
	}
	add(nil)
	add(text.Line{{Text: " " + i18n.T("setup.llm"), Style: headStyle(1)}})
	for i, o := range s.llms {
		noteStyle := "90"
		if o.Available {
			noteStyle = "32"
		}
		option(i == s.llm, s.focus == 1, o.Name, statusText(o.Status), noteStyle, o.Available)
	}
	add(text.Line{{Text: "   ────────────────", Style: "90"}})
	switch {
	case s.llm < 0:
		add(text.Line{{Text: "   " + i18n.T("setup.none"), Style: "33"}})
	case s.llms[s.llm].Local:
		add(text.Line{{Text: "   " + i18n.T("setup.local"), Style: "90"}})
	default:
		add(text.Line{{Text: "   " + i18n.T("setup.sendsCode"), Style: "33"}})
	}
	add(nil)
	add(text.Line{{Text: " " + i18n.T("setup.keys"), Style: "90"}})

	view := tea.NewView(join(lines, false))
	view.AltScreen = true
	return view
}
