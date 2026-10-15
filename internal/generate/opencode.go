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
	"strconv"
	"strings"
	"time"
)

// OpenCode runs OpenCode 2 (opencode run) restricted to read-only tools.
type OpenCode struct {
	Bin string
}

func (o OpenCode) Name() string { return "OpenCode" }

// openCodeBins are the names OpenCode 2 is installed under. The npm package installs both, and
// opencode2 comes first so that an OpenCode 1 earlier on PATH as opencode is skipped.
var openCodeBins = []string{"opencode2", "opencode"}

// ErrOpenCodeV1 means only OpenCode 1 was found. Its command line and config differ, and it is not supported.
var ErrOpenCodeV1 = errors.New("opencode: OpenCode 2 or later is needed")

// FindOpenCode returns the path of an OpenCode 2 executable on PATH.
func FindOpenCode() (string, error) {
	found := false
	for _, bin := range openCodeBins {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		found = true
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, path, "--version").Output()
		cancel()
		if err == nil && openCodeMajor(string(out)) >= 2 {
			return path, nil
		}
	}
	if found {
		return "", ErrOpenCodeV1
	}
	return "", exec.ErrNotFound
}

var openCodeVersion = regexp.MustCompile(`(\d+)\.\d+`)

// openCodeMajor reads the major version from `opencode --version` ("opencode v2.0.29", or "1.19.0" for OpenCode 1).
func openCodeMajor(version string) int {
	m := openCodeVersion.FindStringSubmatch(version)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// openCodeAgent is the agent tour runs as. Every built-in agent can run commands (even explore), so tour defines its own.
const openCodeAgent = "diff-tour"

// openCodeTools are the only tools the model is given. external_directory, checked before read, denies any path
// outside the working directory (the check is on the path alone, but the workspace holds no symlinks).
var openCodeTools = []string{"read", "glob", "grep"}

// openCodeArgs are the flags for running read-only (checked against OpenCode 2.0.29). The prompt is read from stdin.
//
//	--standalone      starts a private server for this run, so the background service (which may have been
//	                  started with other settings) is not used
//	--agent diff-tour the agent defined in openCodeConfig
//
// Not --auto: it would approve anything that asks. Without it, a request that asks is rejected.
func openCodeArgs() []string {
	return []string{"run", "--standalone", "--format", "json", "--agent", openCodeAgent}
}

// openCodeConfig is passed as OPENCODE_CONFIG_CONTENT, which outranks the global and project config
// (only the system config, /etc/opencode, outranks it). user holds the model and provider settings
// copied from the user's own config, so the same model and sign-ins are used.
//
//	permissions        deny everything, then allow openCodeTools. Agent rules come after these, and tour's agent
//	                   has the same rules, so nothing set elsewhere comes after them. A tool whose last rule is a
//	                   deny for every resource is not shown to the model at all.
//	policies           a second layer that can only take away: every tool but openCodeTools, and every MCP server,
//	                   skill and plugin (the built-in tools are not plugins this blocks)
//	update, snapshots  no update check, and no git snapshots of the workspace
func openCodeConfig(user map[string]any) map[string]any {
	var rules []map[string]string
	rules = append(rules, map[string]string{"action": "*", "resource": "*", "effect": "deny"})
	policies := []map[string]string{{"action": "tool.use", "resource": "*", "effect": "deny"}}
	for _, t := range openCodeTools {
		rules = append(rules, map[string]string{"action": t, "resource": "*", "effect": "allow"})
		policies = append(policies, map[string]string{"action": "tool.use", "resource": t + ":*", "effect": "allow"})
	}
	for _, kind := range []string{"mcp", "skill", "plugin"} {
		policies = append(policies, map[string]string{"action": "integration.use", "resource": kind + ":*", "effect": "deny"})
	}
	c := map[string]any{}
	for k, v := range user {
		c[k] = v
	}
	c["update"] = "disable"
	c["snapshots"] = false
	c["permissions"] = rules
	c["policies"] = policies
	c["agents"] = map[string]any{
		openCodeAgent: map[string]any{
			"mode":        "primary",
			"description": "Reads the working directory to write a tour. It cannot change anything.",
			"permissions": rules,
		},
	}
	return c
}

// openCodeUserKeys are the settings copied from the user's config: which model to use, and the providers
// (with their keys and endpoints) behind it. provider is OpenCode 1's name for providers, still accepted.
var openCodeUserKeys = []string{"model", "provider", "providers", "enabled_providers", "disabled_providers"}

// openCodeUserConfig reads openCodeUserKeys from the user's global config (opencode.json, then opencode.jsonc,
// as OpenCode reads them) and from OPENCODE_CONFIG. Nothing else is taken: the rest of it (plugins, MCP servers,
// permissions, agents, AGENTS.md) is not loaded, because the run gets an empty config directory.
func openCodeUserConfig() map[string]any {
	dir := os.Getenv("OPENCODE_CONFIG_DIR")
	if dir == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".config")
		}
		dir = filepath.Join(base, "opencode")
	}
	files := []string{filepath.Join(dir, "opencode.json"), filepath.Join(dir, "opencode.jsonc")}
	if f := os.Getenv("OPENCODE_CONFIG"); f != "" {
		files = append(files, f)
	}
	c := map[string]any{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(stripJSONC(b), &m) != nil {
			continue
		}
		for _, k := range openCodeUserKeys {
			if v, ok := m[k]; ok {
				c[k] = absoluteFileRefs(v, filepath.Dir(f))
			}
		}
	}
	return c
}

// fileRef is OpenCode's {file:path} substitution. A relative path is relative to the config file.
var fileRef = regexp.MustCompile(`\{file:([^}]+)\}`)

// absoluteFileRefs makes relative {file:...} paths absolute, since the copied settings are read from elsewhere.
func absoluteFileRefs(v any, dir string) any {
	switch v := v.(type) {
	case string:
		return fileRef.ReplaceAllStringFunc(v, func(s string) string {
			p := fileRef.FindStringSubmatch(s)[1]
			if filepath.IsAbs(p) || strings.HasPrefix(p, "~") {
				return s
			}
			return "{file:" + filepath.Join(dir, p) + "}"
		})
	case map[string]any:
		for k, x := range v {
			v[k] = absoluteFileRefs(x, dir)
		}
	case []any:
		for i, x := range v {
			v[i] = absoluteFileRefs(x, dir)
		}
	}
	return v
}

// stripJSONC turns JSONC into JSON: it removes comments and trailing commas outside strings.
func stripJSONC(b []byte) []byte {
	var out []byte
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '"':
			j := i + 1
			for ; j < len(b) && b[j] != '"'; j++ {
				if b[j] == '\\' {
					j++
				}
			}
			out = append(out, b[i:min(j+1, len(b))]...)
			i = j
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			i--
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(string(b[i+2:]), "*/")
			if end < 0 {
				return out
			}
			i += end + 3
		case c == ',':
			j := i + 1
			for j < len(b) && strings.ContainsRune(" \t\r\n", rune(b[j])) {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// openCodeEnv sets up a run that reads only tour's own settings.
//
//	OPENCODE_CONFIG_DIR=<empty dir>   replaces ~/.config/opencode, so the user's config, AGENTS.md, plugins,
//	                                  agents, commands and skills there are not loaded
//	OPENCODE_CONFIG_PROJECT_DISABLE   skips opencode.json, AGENTS.md, .opencode, .claude and .agents in the working
//	OPENCODE_DISABLE_PROJECT_CONFIG   directory and above it (both names, since the first one wins if the user set it)
//	OPENCODE_CONFIG_CONTENT           openCodeConfig
//	OPENCODE_DISABLE_AUTOUPDATE, OPENCODE_DISABLE_FILEWATCHER
//	                                  no update check, and no watching of the workspace
//
// OPENCODE_CONFIG (an extra config file) is removed; the settings tour needs from it are already copied.
func openCodeEnv(configDir string, config []byte) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "OPENCODE_CONFIG=") {
			env = append(env, kv)
		}
	}
	return append(env,
		"OPENCODE_CONFIG_DIR="+configDir,
		"OPENCODE_CONFIG_PROJECT_DISABLE=1",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_CONFIG_CONTENT="+string(config),
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_FILEWATCHER=1",
	)
}

type openCodeEvent struct {
	Type string `json:"type"`
	Part struct {
		Text  string  `json:"text"`
		Tool  string  `json:"tool"`
		Cost  float64 `json:"cost"`
		State struct {
			Input json.RawMessage `json:"input"`
		} `json:"state"`
	} `json:"part"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// openCodeStream reads the --format json output. Each text block comes as one text event, and text the
// model writes before a tool call is narration, so only the text after the last tool call is kept.
type openCodeStream struct {
	dir     string
	events  func(Event)
	text    strings.Builder
	preTool string
	cost    float64
	err     error
}

func (s *openCodeStream) line(b []byte) {
	var e openCodeEvent
	if json.Unmarshal(b, &e) != nil {
		return
	}
	switch e.Type {
	case "text":
		s.text.WriteString(e.Part.Text)
	case "tool_use":
		if t := s.text.String(); strings.TrimSpace(t) != "" {
			s.preTool = t
		}
		s.text.Reset()
		var in struct {
			Path    string `json:"path"`
			Pattern string `json:"pattern"`
		}
		_ = json.Unmarshal(e.Part.State.Input, &in)
		switch e.Part.Tool {
		case "read":
			s.events(Event{"read", relative(s.dir, in.Path)})
		case "glob", "grep":
			s.events(Event{"search", in.Pattern})
		}
	case "step_finish":
		s.cost += e.Part.Cost
	case "error":
		msg := "the run ended with an error"
		if e.Error != nil && strings.TrimSpace(e.Error.Message) != "" {
			msg = strings.TrimSpace(e.Error.Message)
		}
		s.err = errors.New(msg)
	}
}

func (s *openCodeStream) result() string {
	if t := s.text.String(); strings.TrimSpace(t) != "" {
		return t
	}
	return s.preTool
}

func (o OpenCode) Run(ctx context.Context, dir, prompt string, events func(Event)) (Result, error) {
	bin := o.Bin
	if bin == "" {
		var err error
		if bin, err = FindOpenCode(); err != nil {
			return Result{}, err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Result{}, err
	}
	configDir, err := os.MkdirTemp("", "diff-tour-opencode-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(configDir)
	config, err := json.Marshal(openCodeConfig(openCodeUserConfig()))
	if err != nil {
		return Result{}, err
	}

	cmd := exec.CommandContext(ctx, bin, openCodeArgs()...)
	cmd.Dir = dir
	cmd.Env = openCodeEnv(configDir, config)
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

	s := &openCodeStream{dir: dir, events: events}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		s.line(sc.Bytes())
	}
	waitErr := cmd.Wait()
	res := Result{Text: s.result(), CostUSD: s.cost}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if s.err != nil {
		return res, fmt.Errorf("opencode: %w", s.err)
	}
	if waitErr != nil || strings.TrimSpace(res.Text) == "" {
		msg := strings.TrimSpace(stderr.String())
		if waitErr != nil && msg == "" {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = "no reply"
		}
		return res, fmt.Errorf("opencode: %s", msg)
	}
	return res, nil
}
