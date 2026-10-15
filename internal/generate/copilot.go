package generate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Copilot runs the GitHub Copilot CLI (copilot, with the prompt on stdin) restricted to read-only tools.
type Copilot struct {
	Bin string
}

func (c Copilot) Name() string { return "Copilot CLI" }

// copilotTools are the only tools the model is given (rg is another name for grep). view also lists directories.
var copilotTools = []string{"view", "grep", "rg", "glob"}

// copilotArgs are the flags for running read-only (checked against Copilot CLI 1.0.98). Piped stdin runs one
// prompt and exits, like -p, without the argument size limit.
//
//	--available-tools          the model sees only copilotTools: no shell, no writes, no web, no sub-agents (task),
//	                           no skills, no MCP tools. Asking for any other tool fails with "does not exist"
//	--deny-tool shell, write   a second layer; deny rules win over every allow rule
//	(no --allow-all-tools)     reads inside the working directory are approved; anything else that needs
//	                           approval, such as reading outside it, is denied at once since no one can answer
//	--disallow-temp-dir        the system temporary directory is not readable either
//	--disable-builtin-mcps     no GitHub MCP server (user MCP servers are not loaded at all; see copilotEnv)
//	--no-custom-instructions   no AGENTS.md, CLAUDE.md, .github/copilot-instructions.md and the like
//	--no-ask-user              no questions back to the user
//	--no-remote, --no-remote-export, --no-auto-update
//	                           the session is not exported to GitHub, and no update is downloaded
//
// Memory is off in this mode unless --enable-memory is passed.
func copilotArgs(model string) []string {
	args := []string{
		"--output-format", "json", "--stream", "off",
		"--available-tools=" + strings.Join(copilotTools, ","),
		"--deny-tool=shell", "--deny-tool=write",
		"--disallow-temp-dir",
		"--disable-builtin-mcps",
		"--no-custom-instructions",
		"--no-ask-user",
		"--no-remote", "--no-remote-export", "--no-auto-update",
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

// copilotHome is the user's Copilot directory.
func copilotHome() string {
	if h := os.Getenv("COPILOT_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".copilot")
}

// copilotUnsafeKey matches config.json keys that could load code or widen access: installed and enabled plugins,
// marketplaces, trusted folders (whose MCP servers and hooks would load), and anything about hooks, MCP,
// extensions, URLs, permissions, skills, agents or instructions.
var copilotUnsafeKey = regexp.MustCompile(`(?i)plugin|marketplace|trust|hook|mcp|extension|url|permission|skill|agent|instruction`)

// prepareCopilotHome fills the empty directory home with what a run needs from the user's Copilot directory:
// config.json, which holds the sign-in, without copilotUnsafeKey keys. Nothing else is copied: not settings.json
// (hooks, allowed URLs, extensions), mcp-config.json, hooks/, extensions/, installed-plugins/, skills/, agents/,
// saved permissions or copilot-instructions.md. It returns the model chosen in settings.json, if any.
func prepareCopilotHome(user, home string) (model string, err error) {
	if b, err := os.ReadFile(filepath.Join(user, "config.json")); err == nil {
		var c map[string]any
		if json.Unmarshal(stripJSONC(b), &c) == nil {
			for k := range c {
				if copilotUnsafeKey.MatchString(k) {
					delete(c, k)
				}
			}
			out, err := json.MarshalIndent(c, "", "  ")
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(home, "config.json"), out, 0o600); err != nil {
				return "", err
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(user, "settings.json")); err == nil {
		var s struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(stripJSONC(b), &s) == nil {
			model = s.Model
		}
	}
	return model, nil
}

// copilotEnv points Copilot at the prepared home and turns off what the user's environment could turn on.
//
//	COPILOT_HOME=<prepared dir>   the user's hooks, plugins, extensions, MCP servers, skills, agents, trusted
//	                              folders and saved approvals are not loaded. Sessions and logs are written there
//	                              and removed with it
//	COPILOT_ALLOW_ALL=false       would approve every tool, and "true" would also trust the directory
//	COPILOT_AUTO_UPDATE=false     no update download
//
// Removed: GITHUB_COPILOT_PROMPT_MODE_* (they load the workspace's MCP servers, hooks and extensions),
// COPILOT_CUSTOM_INSTRUCTIONS_DIRS, COPILOT_SKILLS_DIRS, and COPILOT_PROMPT_TURN_DELIMITER (it would split the
// prompt into several turns).
func copilotEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(k, "GITHUB_COPILOT_PROMPT_MODE_"),
			k == "COPILOT_CUSTOM_INSTRUCTIONS_DIRS", k == "COPILOT_SKILLS_DIRS", k == "COPILOT_PROMPT_TURN_DELIMITER":
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"COPILOT_HOME="+home,
		"COPILOT_ALLOW_ALL=false",
		"COPILOT_AUTO_UPDATE=false",
	)
}

type copilotEvent struct {
	Type string `json:"type"`
	Data struct {
		Content          string `json:"content"`
		ParentToolCallID string `json:"parentToolCallId"`
		ToolRequests     []struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"toolRequests"`
		Message   string `json:"message"`
		ErrorType string `json:"errorType"`
	} `json:"data"`
	ExitCode *int `json:"exitCode"`
}

// copilotStream reads the --output-format json events. Tool calls come inside assistant messages (there is no
// event for a tool running), and the answer is the last top-level assistant message that has text (a message
// that only calls tools has none).
type copilotStream struct {
	dir       string
	events    func(Event)
	text      string
	gotResult bool
	err       error
}

func (s *copilotStream) line(b []byte) {
	var e copilotEvent
	if json.Unmarshal(b, &e) != nil {
		return
	}
	switch e.Type {
	case "assistant.message":
		if e.Data.ParentToolCallID != "" {
			return
		}
		if strings.TrimSpace(e.Data.Content) != "" {
			s.text = e.Data.Content
		}
		for _, r := range e.Data.ToolRequests {
			var in struct {
				Path    string `json:"path"`
				Pattern string `json:"pattern"`
			}
			_ = json.Unmarshal(r.Arguments, &in)
			switch r.Name {
			case "view":
				s.events(Event{"read", relative(s.dir, in.Path)})
			case "grep", "rg", "glob":
				s.events(Event{"search", in.Pattern})
			}
		}
	case "session.error":
		// A failed model call is retried or reported again; anything else ends the run.
		if e.Data.ErrorType != "model_call" {
			msg := strings.TrimSpace(e.Data.Message)
			if msg == "" {
				msg = e.Data.ErrorType
			}
			s.err = errors.New(msg)
		}
	case "result":
		s.gotResult = true
		if e.ExitCode != nil && *e.ExitCode != 0 && s.err == nil {
			s.err = fmt.Errorf("exit code %d", *e.ExitCode)
		}
	}
}

func (c Copilot) Run(ctx context.Context, dir, prompt string, events func(Event)) (Result, error) {
	bin := c.Bin
	if bin == "" {
		bin = "copilot"
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Result{}, err
	}
	home, err := os.MkdirTemp("", "diff-tour-copilot-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(home)
	model, err := prepareCopilotHome(copilotHome(), home)
	if err != nil {
		return Result{}, err
	}

	cmd := exec.CommandContext(ctx, bin, copilotArgs(model)...)
	cmd.Dir = dir
	cmd.Env = copilotEnv(home)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	s := &copilotStream{dir: dir, events: events}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		s.line(sc.Bytes())
	}
	waitErr := cmd.Wait()
	res := Result{Text: s.text}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if s.err != nil {
		return res, fmt.Errorf("copilot: %w", s.err)
	}
	// Sign-in and model errors come before the session starts, as plain text on stderr.
	if waitErr != nil || !s.gotResult || strings.TrimSpace(res.Text) == "" {
		msg := strings.TrimSpace(stderr.String())
		if waitErr != nil && msg == "" {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = "no reply"
		}
		return res, fmt.Errorf("copilot: %s", msg)
	}
	return res, nil
}
