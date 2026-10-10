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
	"slices"
	"strings"
)

// Gemini runs the Gemini CLI (gemini -p) restricted to read-only tools.
type Gemini struct {
	Bin string
}

func (g Gemini) Name() string { return "Gemini CLI" }

// geminiTools are the only tools the model is given. All of them only read, and all of them
// refuse paths that resolve (symlinks included) outside the workspace directories.
var geminiTools = []string{"read_file", "glob", "grep_search", "list_directory"}

// geminiMCPSentinel is a server name nobody uses. Allowing only it blocks every MCP server.
const geminiMCPSentinel = "diff-tour-no-mcp"

// geminiArgs are the flags for running read-only.
//
//	--admin-policy <file>       loads geminiPolicy at the admin tier, which outranks user, workspace and
//	                            extension policies: only geminiTools are allowed (and only inside the
//	                            workspace); everything else (shell, write_file, replace, web_fetch,
//	                            google_web_search, MCP tools, subagents, skills, ...) is denied and is not
//	                            even shown to the model
//	--approval-mode default     ignores a user's general.defaultApprovalMode (e.g. auto_edit); headless,
//	                            anything that would need confirmation is denied
//	--allowed-mcp-server-names  allows only geminiMCPSentinel, so no configured MCP server is started
//	--extensions none           loads no extensions (and so none of their MCP servers, policies or context)
//	--skip-trust                trusts this directory for the session (folder trust is on by default), so
//	                            the workspace settings below are applied; see also geminiEnv (diff-tour
//	                            created the directory, so it holds nothing else of Gemini CLI's, such as a .env)
//
// geminiSettings (written to the workspace's .gemini/settings.json) are the second, independent layer.
func geminiArgs(prompt, policyPath string) []string {
	return []string{
		"-p", prompt,
		"--admin-policy", policyPath,
		"--approval-mode", "default",
		"--allowed-mcp-server-names", geminiMCPSentinel,
		"--extensions", "none",
		"--skip-trust",
		"--output-format", "stream-json",
	}
}

// geminiSettings are written to .gemini/settings.json in the workspace. Workspace settings outrank the
// user's ~/.gemini/settings.json for every key below (only a system settings file outranks them, and Gemini
// CLI accepts that only when it and its directories are owned by root, so diff-tour cannot use one).
//
//	tools.core                  registers only geminiTools (and adds a user-tier deny for every other tool)
//	tools.discoveryCommand, tools.callCommand, mcp.serverCommand
//	                            cleared, so no user-configured command is run to discover or call tools
//	experimental.enableAgents   off, so there are no subagents (e.g. the browser agent)
//	skills.enabled, hooksConfig.enabled
//	                            off, so no skill scripts and no user hooks run
//	security.disableYoloMode, security.disableAlwaysAllow
//	                            no auto-approval of anything
//	context.fileName            a name that does not exist, so no GEMINI.md (the user's, or one inside the
//	                            exported repository) is loaded as instructions
//	general.checkpointing       off, so no git snapshots of the workspace are written
//	telemetry, privacy.usageStatisticsEnabled
//	                            off (GEMINI_TELEMETRY_ENABLED=false too)
//
// Not enforceable from here:
//   - The session transcript is always written to ~/.gemini/tmp/<project>/chats (there is no setting to turn
//     it off; Gemini CLI deletes it after general.sessionRetention, 30 days by default), and the workspace
//     directory is added to ~/.gemini/projects.json.
//   - Directories in the user's context.includeDirectories are appended to the workspace (that list is
//     concatenated across settings files, so it cannot be cleared). geminiPolicy denies absolute paths
//     outside the workspace and ".." segments, which keeps the tools out of them.
//   - A root-owned system settings file or system policy directory (e.g. /etc/gemini-cli) set by an
//     administrator outranks these settings, and the system policy directory makes Gemini CLI ignore
//     --admin-policy. --allowed-mcp-server-names and --extensions none still apply.
var geminiSettings = map[string]any{
	"tools": map[string]any{
		"core":             geminiTools,
		"discoveryCommand": "",
		"callCommand":      "",
	},
	"mcp":          map[string]any{"serverCommand": ""},
	"experimental": map[string]any{"enableAgents": false, "autoMemory": false},
	"skills":       map[string]any{"enabled": false},
	"hooksConfig":  map[string]any{"enabled": false},
	"security": map[string]any{
		"disableYoloMode":    true,
		"disableAlwaysAllow": true,
	},
	"context": map[string]any{"fileName": "DIFF_TOUR_NO_CONTEXT_FILE.md"},
	"general": map[string]any{
		"checkpointing":        map[string]any{"enabled": false},
		"topicUpdateNarration": false,
	},
	"telemetry": map[string]any{"enabled": false},
	"privacy":   map[string]any{"usageStatisticsEnabled": false},
}

// geminiPolicy is the admin-tier policy (TOML) for a workspace at dir. The highest priority that matches wins:
//
//	300  deny geminiTools when a path argument leaves dir (geminiOutsideRules)
//	200  allow geminiTools
//	100  deny every other tool, built-in or MCP
//
// Gemini CLI drops (with only a warning) any rule whose argsPattern looks unsafe to it: longer than 2048
// characters, or a group with a quantifier inside followed by another quantifier. The patterns are written
// to pass that check, and an error is returned rather than writing a rule that would be dropped.
func geminiPolicy(dir string) (string, error) {
	var b strings.Builder
	rule := func(tools []string, decision string, priority int, argsPattern, msg string) {
		b.WriteString("[[rule]]\n")
		fmt.Fprintf(&b, "toolName = %s\n", geminiTOMLStrings(tools))
		if argsPattern != "" {
			fmt.Fprintf(&b, "argsPattern = %s\n", geminiTOMLString(argsPattern))
		}
		fmt.Fprintf(&b, "decision = %q\npriority = %d\n", decision, priority)
		if msg != "" {
			fmt.Fprintf(&b, "denyMessage = %s\n", geminiTOMLString(msg))
		}
		b.WriteString("\n")
	}
	for _, r := range geminiOutsideRules(dir) {
		if len(r.pattern) > 2048 || geminiUnsafePattern.MatchString(r.pattern) {
			return "", fmt.Errorf("gemini: cannot build a read-only policy for %s", dir)
		}
		rule(r.tools, "deny", 300, r.pattern, "diff-tour only allows reading inside the working directory.")
	}
	rule(geminiTools, "allow", 200, "", "")
	rule([]string{"*"}, "deny", 100, "", "diff-tour only allows reading files.")
	return b.String(), nil
}

// geminiUnsafePattern is Gemini CLI's own check for argsPattern (isSafeRegExp in 0.63.0).
var geminiUnsafePattern = regexp.MustCompile(`\([^)]*[*+?{].*\)[*+?{]`)

type geminiRule struct {
	tools   []string
	pattern string
}

// geminiOutsideRules match, in the JSON of a tool's arguments (`"key":"value"`, JSON-escaped), a path that
// leaves dir: an absolute or home-relative path not under dir, one that starts with "..", or one with a
// ".." segment. glob's pattern is joined to the search directory, so it gets the same treatment.
func geminiOutsideRules(dir string) []geminiRule {
	var prefixes []string
	for _, d := range []string{dir, geminiEvalSymlinks(dir)} {
		j, _ := json.Marshal(filepath.Clean(d))
		if q := regexp.QuoteMeta(string(j[1 : len(j)-1])); !slices.Contains(prefixes, q) {
			prefixes = append(prefixes, q)
		}
	}
	const (
		paths  = `"(?:file_path|dir_path)":"`
		abs    = `(?:[/~\\]|[A-Za-z]:)` // /x, \x (starts an escape in JSON), C:x, ~x
		sep    = `(?:/|\\\\)`           // / or an escaped \
		dotdot = `\.\.(?:/|\\\\|")`     // ".." then a separator or the end of the value
	)
	inside := `(?:` + strings.Join(prefixes, "|") + `)(?:/|\\\\|")`
	return []geminiRule{
		{geminiTools, paths + `(?!` + inside + `)` + abs},
		{geminiTools, paths + dotdot},
		{geminiTools, sep + dotdot}, // anywhere in any argument, which may also block a harmless grep pattern
		{[]string{"glob"}, `"pattern":"` + abs},
		{[]string{"glob"}, `"pattern":"` + dotdot},
	}
}

func geminiEvalSymlinks(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

func geminiTOMLString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func geminiTOMLStrings(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = geminiTOMLString(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// geminiEnv overrides what the user's environment could set.
//
//	GEMINI_CLI_TRUST_WORKSPACE=true  trusts the workspace from the start: settings are read before --skip-trust
//	                                 is parsed, and those of an untrusted workspace (geminiSettings) are ignored
//	GEMINI_RESTRICTED_MODE=false     the same (when true it marks every folder untrusted)
//	GEMINI_SANDBOX=false             no re-launch inside a container, where the policy file would not exist
//	GEMINI_TELEMETRY_ENABLED=false   overrides telemetry.enabled from any source
func geminiEnv() []string {
	return append(os.Environ(),
		"GEMINI_CLI_TRUST_WORKSPACE=true",
		"GEMINI_RESTRICTED_MODE=false",
		"GEMINI_SANDBOX=false",
		"GEMINI_TELEMETRY_ENABLED=false",
	)
}

type geminiEvent struct {
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolName   string          `json:"tool_name"`
	Parameters json.RawMessage `json:"parameters"`
	Message    string          `json:"message"`
	Status     string          `json:"status"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// geminiStream reads the stream-json output. Text the model writes before a tool call is narration,
// so only the text after the last tool call is kept (as Gemini CLI does for --output-format json).
type geminiStream struct {
	dir       string
	events    func(Event)
	text      strings.Builder
	preTool   string
	gotResult bool
	lastError string
	err       error
}

func (s *geminiStream) line(b []byte) {
	var e geminiEvent
	if json.Unmarshal(b, &e) != nil {
		return
	}
	switch e.Type {
	case "message":
		if e.Role == "assistant" {
			s.text.WriteString(e.Content)
		}
	case "tool_use":
		if t := s.text.String(); strings.TrimSpace(t) != "" {
			s.preTool = t
		}
		s.text.Reset()
		var in struct {
			FilePath string `json:"file_path"`
			Pattern  string `json:"pattern"`
		}
		_ = json.Unmarshal(e.Parameters, &in)
		switch e.ToolName {
		case "read_file":
			s.events(Event{"read", relative(s.dir, in.FilePath)})
		case "grep_search", "glob":
			s.events(Event{"search", in.Pattern})
		}
	case "error":
		s.lastError = strings.TrimSpace(e.Message)
	case "result":
		s.gotResult = true
		if e.Status == "error" {
			msg := s.lastError
			if e.Error != nil && strings.TrimSpace(e.Error.Message) != "" {
				msg = strings.TrimSpace(e.Error.Message)
			}
			if msg == "" {
				msg = "the run ended with an error"
			}
			s.err = errors.New(msg)
		}
	}
}

func (s *geminiStream) result() string {
	if t := s.text.String(); strings.TrimSpace(t) != "" {
		return t
	}
	return s.preTool
}

func (g Gemini) Run(ctx context.Context, dir, prompt string, events func(Event)) (Result, error) {
	bin := g.Bin
	if bin == "" {
		bin = "gemini"
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Result{}, err
	}
	// The policy lives outside the workspace; the settings must be inside it.
	tmp, err := os.MkdirTemp("", "diff-tour-gemini-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)
	policy, err := geminiPolicy(dir)
	if err != nil {
		return Result{}, err
	}
	policyPath := filepath.Join(tmp, "policy.toml")
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		return Result{}, err
	}
	settings, err := json.MarshalIndent(geminiSettings, "", "  ")
	if err != nil {
		return Result{}, err
	}
	settingsDir := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(settingsDir)
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), settings, 0o644); err != nil {
		return Result{}, err
	}

	cmd := exec.CommandContext(ctx, bin, geminiArgs(prompt, policyPath)...)
	cmd.Dir = dir
	cmd.Env = geminiEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	s := &geminiStream{dir: dir, events: events}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		s.line(sc.Bytes())
	}
	waitErr := cmd.Wait()
	res := Result{Text: s.result()}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if s.err != nil {
		return res, fmt.Errorf("gemini: %w", s.err)
	}
	if !s.gotResult || strings.TrimSpace(res.Text) == "" {
		msg := strings.TrimSpace(stderr.String())
		if waitErr != nil && msg == "" {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = "no reply"
		}
		return res, fmt.Errorf("gemini: %s", msg)
	}
	return res, nil
}
