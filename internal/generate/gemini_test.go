package generate

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestGeminiArgs(t *testing.T) {
	args := geminiArgs("PROMPT", "/tmp/p/policy.toml")
	want := [][]string{
		{"-p", "PROMPT"},
		{"--admin-policy", "/tmp/p/policy.toml"},
		{"--approval-mode", "default"},
		{"--allowed-mcp-server-names", geminiMCPSentinel},
		{"--extensions", "none"},
		{"--output-format", "stream-json"},
	}
	for _, w := range want {
		i := slices.Index(args, w[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != w[1] {
			t.Errorf("want %s %s in %q", w[0], w[1], args)
		}
	}
	if !slices.Contains(args, "--skip-trust") {
		t.Errorf("want --skip-trust in %q", args)
	}
	for _, a := range args {
		if a == "-y" || a == "--yolo" || a == "--sandbox" || strings.HasPrefix(a, "--allowed-tools") || strings.HasPrefix(a, "--include-directories") {
			t.Errorf("unexpected %s", a)
		}
	}
}

func TestGeminiSettings(t *testing.T) {
	b, err := json.Marshal(geminiSettings)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Tools struct {
			Core             []string `json:"core"`
			DiscoveryCommand *string  `json:"discoveryCommand"`
			CallCommand      *string  `json:"callCommand"`
		} `json:"tools"`
		MCP struct {
			ServerCommand *string `json:"serverCommand"`
		} `json:"mcp"`
		Experimental struct {
			EnableAgents *bool `json:"enableAgents"`
		} `json:"experimental"`
		Skills      struct{ Enabled *bool } `json:"skills"`
		HooksConfig struct{ Enabled *bool } `json:"hooksConfig"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Tools.Core, []string{"read_file", "glob", "grep_search", "list_directory"}) {
		t.Errorf("tools.core = %v", s.Tools.Core)
	}
	// An empty string (not a missing key) is needed to override a user's command.
	for name, v := range map[string]*string{"tools.discoveryCommand": s.Tools.DiscoveryCommand, "tools.callCommand": s.Tools.CallCommand, "mcp.serverCommand": s.MCP.ServerCommand} {
		if v == nil || *v != "" {
			t.Errorf("%s must be set to \"\"", name)
		}
	}
	for name, v := range map[string]*bool{"experimental.enableAgents": s.Experimental.EnableAgents, "skills.enabled": s.Skills.Enabled, "hooksConfig.enabled": s.HooksConfig.Enabled} {
		if v == nil || *v {
			t.Errorf("%s must be set to false", name)
		}
	}
}

func TestGeminiPolicy(t *testing.T) {
	dir := "/home/u/.cache/tour/work/abc"
	p, err := geminiPolicy(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`toolName = ["read_file", "glob", "grep_search", "list_directory"]` + "\ndecision = \"allow\"\npriority = 200",
		`toolName = ["*"]` + "\ndecision = \"deny\"\npriority = 100",
	} {
		if !strings.Contains(p, s) {
			t.Errorf("policy lacks %q:\n%s", s, p)
		}
	}
	if strings.Count(p, "[[rule]]") != len(geminiOutsideRules(dir))+2 {
		t.Errorf("unexpected number of rules:\n%s", p)
	}
	if _, err := geminiPolicy("/" + strings.Repeat("x", 3000)); err == nil {
		t.Error("a pattern Gemini CLI would drop must be an error")
	}
}

// The arguments as Gemini CLI's policy engine sees them (stableStringify: `{ "key":"value" , ... }`).
func geminiArgsJSON(kv ...string) string {
	var parts []string
	for i := 0; i < len(kv); i += 2 {
		k, _ := json.Marshal(kv[i])
		v, _ := json.Marshal(kv[i+1])
		parts = append(parts, string(k)+":"+string(v))
	}
	return "{ " + strings.Join(parts, " , ") + " }"
}

func TestGeminiOutsideRules(t *testing.T) {
	dir := "/home/u/.cache/tour/work/abc"
	rules := geminiOutsideRules(dir)
	for _, r := range rules {
		if len(r.pattern) > 2048 || geminiUnsafePattern.MatchString(r.pattern) {
			t.Errorf("Gemini CLI would drop %s", r.pattern)
		}
	}
	// Go's regexp has no lookahead, so the first rule (absolute paths outside dir) is checked by its shape
	// only; it is exercised in Gemini CLI itself only by a manual safety probe.
	if !strings.Contains(rules[0].pattern, "(?!(?:"+regexp.QuoteMeta(dir)+")") {
		t.Errorf("rule 0 = %s", rules[0].pattern)
	}
	denied := func(tool, args string) bool {
		for _, r := range rules[1:] {
			if slices.Contains(r.tools, tool) && regexp.MustCompile(r.pattern).MatchString(args) {
				return true
			}
		}
		return false
	}
	cases := []struct {
		tool, args string
		deny       bool
	}{
		{"read_file", geminiArgsJSON("file_path", "../canary.txt"), true},
		{"read_file", geminiArgsJSON("file_path", ".."), true},
		{"read_file", geminiArgsJSON("file_path", "head/../../canary.txt"), true},
		{"read_file", geminiArgsJSON("file_path", dir+"/../canary.txt"), true},
		{"read_file", geminiArgsJSON("file_path", `head\..\..\canary.txt`), true},
		{"read_file", geminiArgsJSON("file_path", `a"b/../../x`), true},
		{"list_directory", geminiArgsJSON("dir_path", ".."), true},
		{"glob", geminiArgsJSON("pattern", "../*.txt"), true},
		{"glob", geminiArgsJSON("pattern", "/home/**"), true},
		{"glob", geminiArgsJSON("pattern", "~/x"), true},
		{"read_file", geminiArgsJSON("file_path", "head/a.go"), false},
		{"read_file", geminiArgsJSON("file_path", "head/foo..bar.go"), false},
		{"read_file", geminiArgsJSON("file_path", dir+"/head/a.go"), false},
		{"glob", geminiArgsJSON("pattern", "**/*.go"), false},
		{"grep_search", geminiArgsJSON("dir_path", "head", "pattern", "/api/v1"), false},
		{"grep_search", geminiArgsJSON("pattern", `\.\./x`), false},
	}
	for _, c := range cases {
		if got := denied(c.tool, c.args); got != c.deny {
			t.Errorf("%s %s: denied = %v, want %v", c.tool, c.args, got, c.deny)
		}
	}
}

// Lines as Gemini CLI 0.63.0 writes them with --output-format stream-json (timestamps and stats trimmed).
const geminiSample = `{"type":"init","timestamp":"2026-10-10T15:43:03.507Z","session_id":"26ba88a1","model":"auto"}
{"type":"message","timestamp":"2026-10-10T15:43:03.508Z","role":"user","content":"Write the tour."}
{"type":"message","timestamp":"2026-10-10T15:43:03.520Z","role":"assistant","content":"I'll start by reading ","delta":true}
{"type":"message","timestamp":"2026-10-10T15:43:03.521Z","role":"assistant","content":"the diff.","delta":true}
{"type":"tool_use","timestamp":"2026-10-10T15:43:03.575Z","tool_name":"read_file","tool_id":"read_file__read_file_1_0","parameters":{"file_path":"/w/change.diff"}}
{"type":"tool_use","timestamp":"2026-10-10T15:43:03.576Z","tool_name":"grep_search","tool_id":"grep_search__grep_search_1_1","parameters":{"dir_path":"head","pattern":"used_count"}}
{"type":"tool_use","timestamp":"2026-10-10T15:43:03.577Z","tool_name":"glob","tool_id":"glob__glob_1_2","parameters":{"pattern":"head/**/*.go"}}
{"type":"tool_use","timestamp":"2026-10-10T15:43:03.578Z","tool_name":"list_directory","tool_id":"list_directory__list_directory_1_3","parameters":{"dir_path":"head"}}
{"type":"tool_use","timestamp":"2026-10-10T15:43:03.579Z","tool_name":"run_shell_command","tool_id":"run_shell_command__run_shell_command_1_4","parameters":{"command":"rm -rf head"}}
{"type":"tool_result","timestamp":"2026-10-10T15:43:03.637Z","tool_id":"run_shell_command__run_shell_command_1_4","status":"error","output":"Tool \"run_shell_command\" not found.","error":{"type":"tool_not_registered","message":"Tool \"run_shell_command\" not found."}}
{"type":"tool_result","timestamp":"2026-10-10T15:43:03.638Z","tool_id":"read_file__read_file_1_0","status":"success","output":""}
{"type":"message","timestamp":"2026-10-10T15:43:03.647Z","role":"assistant","content":"=== ツアー JSON ===\n{\"version\":","delta":true}
{"type":"message","timestamp":"2026-10-10T15:43:03.648Z","role":"assistant","content":"1}","delta":true}
{"type":"result","timestamp":"2026-10-10T15:43:03.648Z","status":"success","stats":{"total_tokens":45,"input_tokens":30,"output_tokens":15,"duration_ms":141,"tool_calls":5}}
`

func readGeminiSample(t *testing.T, sample string) (*geminiStream, []Event) {
	t.Helper()
	var events []Event
	s := &geminiStream{dir: "/w", events: func(e Event) { events = append(events, e) }}
	for _, l := range strings.Split(sample, "\n") {
		s.line([]byte(l))
	}
	return s, events
}

func TestGeminiStream(t *testing.T) {
	s, events := readGeminiSample(t, geminiSample)
	want := []Event{{"read", "change.diff"}, {"search", "used_count"}, {"search", "head/**/*.go"}}
	if !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
	if !s.gotResult || s.err != nil {
		t.Errorf("gotResult = %v, err = %v", s.gotResult, s.err)
	}
	// The narration before the tool calls is dropped; the streamed deltas after them are joined.
	if got := s.result(); got != "=== ツアー JSON ===\n{\"version\":1}" {
		t.Errorf("text = %q", got)
	}
}

func TestGeminiStreamTextBeforeToolCall(t *testing.T) {
	// If the model answers and then still calls a tool, the answer is kept.
	s, _ := readGeminiSample(t, `{"type":"message","role":"assistant","content":"{\"version\":1}","delta":true}
{"type":"tool_use","tool_name":"read_file","parameters":{"file_path":"head/a.go"}}
{"type":"result","status":"success"}`)
	if got := s.result(); got != `{"version":1}` {
		t.Errorf("text = %q", got)
	}
}

func TestGeminiStreamErrors(t *testing.T) {
	// A fatal error (handleError) is reported in the result itself.
	s, _ := readGeminiSample(t, `{"type":"init","session_id":"x","model":"auto"}
{"type":"result","status":"error","error":{"type":"Error","message":"[API Error: quota exceeded]"},"stats":{}}`)
	if s.err == nil || s.err.Error() != "[API Error: quota exceeded]" {
		t.Errorf("err = %v", s.err)
	}
	// Otherwise the message comes in an error event just before the result.
	s, _ = readGeminiSample(t, `{"type":"message","role":"assistant","content":"partial","delta":true}
{"type":"error","severity":"error","message":"Model stream ended with empty response text."}
{"type":"result","status":"error","stats":{}}`)
	if s.err == nil || !strings.Contains(s.err.Error(), "empty response") {
		t.Errorf("err = %v", s.err)
	}
	// Warnings alone do not fail the run.
	s, _ = readGeminiSample(t, `{"type":"error","severity":"warning","message":"Loop detected, stopping execution"}
{"type":"message","role":"assistant","content":"done","delta":true}
{"type":"result","status":"success","stats":{}}`)
	if s.err != nil || s.result() != "done" {
		t.Errorf("err = %v, text = %q", s.err, s.result())
	}
	// No result at all (e.g. not signed in: Gemini CLI prints to stderr only).
	s, _ = readGeminiSample(t, "")
	if s.gotResult {
		t.Error("gotResult with no output")
	}
}
