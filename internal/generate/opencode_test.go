package generate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestOpenCodeArgs(t *testing.T) {
	args := openCodeArgs()
	if strings.Join(args, " ") != "run --standalone --format json --agent diff-tour" {
		t.Errorf("args = %q", args)
	}
	// --auto would approve whatever asks, and a message argument would bypass stdin.
	for _, bad := range []string{"--auto", "--yolo", "--dangerously-skip-permissions", "--server", "--attach"} {
		if slices.Contains(args, bad) {
			t.Errorf("args contain %s", bad)
		}
	}
}

func TestOpenCodeConfig(t *testing.T) {
	user := map[string]any{"model": "anthropic/claude", "permissions": "ignored", "agents": "ignored"}
	b, err := json.Marshal(openCodeConfig(user))
	if err != nil {
		t.Fatal(err)
	}
	type rule struct{ Action, Resource, Effect string }
	var c struct {
		Model       string
		Update      string
		Snapshots   *bool
		Permissions []rule
		Policies    []rule
		Agents      map[string]struct {
			Mode        string
			Permissions []rule
		}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Model != "anthropic/claude" || c.Update != "disable" || c.Snapshots == nil || *c.Snapshots {
		t.Errorf("model = %q, update = %q, snapshots = %v", c.Model, c.Update, c.Snapshots)
	}
	want := []rule{{"*", "*", "deny"}, {"read", "*", "allow"}, {"glob", "*", "allow"}, {"grep", "*", "allow"}}
	if !reflect.DeepEqual(c.Permissions, want) {
		t.Errorf("permissions = %v", c.Permissions)
	}
	if a := c.Agents["diff-tour"]; a.Mode != "primary" || !reflect.DeepEqual(a.Permissions, want) || len(c.Agents) != 1 {
		t.Errorf("agents = %+v", c.Agents)
	}
	wantPolicies := []rule{
		{"tool.use", "*", "deny"}, {"tool.use", "read:*", "allow"}, {"tool.use", "glob:*", "allow"}, {"tool.use", "grep:*", "allow"},
		{"integration.use", "mcp:*", "deny"}, {"integration.use", "skill:*", "deny"}, {"integration.use", "plugin:*", "deny"},
	}
	if !reflect.DeepEqual(c.Policies, wantPolicies) {
		t.Errorf("policies = %v", c.Policies)
	}
}

func TestOpenCodeUserConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"model": "openai/gpt", "theme": "dark"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte(`{
  // the model I use
  "model": "anthropic/claude", /* overrides opencode.json */
  "providers": {"local": {"options": {"baseURL": "http://localhost:11434/v1", "apiKey": "{file:key.txt}"}}},
  "plugins": ["evil"],
  "mcp": {"servers": {}},
  "permissions": [{"action": "*", "resource": "*", "effect": "allow"},],
}`), 0o644)
	extra := filepath.Join(t.TempDir(), "extra.json")
	os.WriteFile(extra, []byte(`{"disabled_providers": ["openai"], "agents": {}}`), 0o644)
	t.Setenv("OPENCODE_CONFIG", extra)

	got := openCodeUserConfig()
	want := map[string]any{
		"model":              "anthropic/claude",
		"providers":          map[string]any{"local": map[string]any{"options": map[string]any{"baseURL": "http://localhost:11434/v1", "apiKey": "{file:" + filepath.Join(dir, "key.txt") + "}"}}},
		"disabled_providers": []any{"openai"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestStripJSONC(t *testing.T) {
	in := `{"a": "x // not a comment, /* nor this */", // comment
"b": [1, 2,], /* block
comment */ "c": "\"quoted,\"",}`
	var m map[string]any
	if err := json.Unmarshal(stripJSONC([]byte(in)), &m); err != nil {
		t.Fatalf("%v: %s", err, stripJSONC([]byte(in)))
	}
	if m["a"] != "x // not a comment, /* nor this */" || len(m["b"].([]any)) != 2 || m["c"] != `"quoted,"` {
		t.Errorf("got %v", m)
	}
}

func TestOpenCodeEnv(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG", "/home/me/extra.json")
	t.Setenv("OPENCODE_CONFIG_PROJECT_DISABLE", "0")
	env := openCodeEnv("/tmp/empty", []byte(`{"a":1}`))
	last := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		last[k] = v // exec uses the last value of a duplicate key
	}
	for k, v := range map[string]string{
		"OPENCODE_CONFIG_DIR":             "/tmp/empty",
		"OPENCODE_CONFIG_PROJECT_DISABLE": "1",
		"OPENCODE_DISABLE_PROJECT_CONFIG": "1",
		"OPENCODE_CONFIG_CONTENT":         `{"a":1}`,
		"OPENCODE_DISABLE_AUTOUPDATE":     "1",
		"OPENCODE_DISABLE_FILEWATCHER":    "1",
	} {
		if last[k] != v {
			t.Errorf("%s = %q, want %q", k, last[k], v)
		}
	}
	if _, ok := last["OPENCODE_CONFIG"]; ok {
		t.Error("OPENCODE_CONFIG must be removed")
	}
}

func TestOpenCodeMajor(t *testing.T) {
	for in, want := range map[string]int{"opencode v2.0.29\n": 2, "1.19.0\n": 1, "opencode v10.1.0": 10, "": 0} {
		if got := openCodeMajor(in); got != want {
			t.Errorf("openCodeMajor(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestFindOpenCode(t *testing.T) {
	fake := func(dir, name, version string) {
		os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho '"+version+"'\n"), 0o755)
	}
	v1, v2 := t.TempDir(), t.TempDir()
	fake(v1, "opencode", "1.19.0")
	fake(v2, "opencode2", "opencode v2.0.29")

	t.Setenv("PATH", v1)
	if _, err := FindOpenCode(); !errors.Is(err, ErrOpenCodeV1) {
		t.Errorf("v1 only: err = %v", err)
	}
	// opencode2 is found even when OpenCode 1 comes first on PATH as opencode.
	t.Setenv("PATH", v1+string(os.PathListSeparator)+v2)
	if got, err := FindOpenCode(); err != nil || got != filepath.Join(v2, "opencode2") {
		t.Errorf("got %q, %v", got, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := FindOpenCode(); err == nil || errors.Is(err, ErrOpenCodeV1) {
		t.Errorf("none: err = %v", err)
	}
}

// openCodeSample is trimmed from a real run of OpenCode 2.0.29.
const openCodeSample = `{"type":"step_start","timestamp":1,"sessionID":"s","part":{"type":"step-start"}}
{"type":"text","timestamp":2,"sessionID":"s","part":{"type":"text","text":"Let me look at the change first."}}
{"type":"tool_use","timestamp":3,"sessionID":"s","part":{"type":"tool","tool":"read","state":{"status":"completed","input":{"path":"/w/change.diff"},"output":"..."}}}
{"type":"tool_use","timestamp":4,"sessionID":"s","part":{"type":"tool","tool":"grep","state":{"status":"completed","input":{"pattern":"used_count"},"output":"..."}}}
{"type":"tool_use","timestamp":5,"sessionID":"s","part":{"type":"tool","tool":"glob","state":{"status":"completed","input":{"pattern":"head/**/*.go"},"output":"..."}}}
{"type":"tool_use","timestamp":6,"sessionID":"s","part":{"type":"tool","tool":"read","state":{"status":"error","input":{"path":"/etc/hostname"},"error":"Permission denied: external_directory"}}}
{"type":"step_finish","timestamp":7,"sessionID":"s","part":{"type":"step-finish","reason":"tool-calls","cost":0.25,"tokens":{"input":6892,"output":65}}}
{"type":"step_start","timestamp":8,"sessionID":"s","part":{"type":"step-start"}}
{"type":"text","timestamp":9,"sessionID":"s","part":{"type":"text","text":"=== ツアー JSON ===\n"}}
{"type":"text","timestamp":10,"sessionID":"s","part":{"type":"text","text":"{\"version\":1}"}}
{"type":"step_finish","timestamp":11,"sessionID":"s","part":{"type":"step-finish","reason":"stop","cost":0.5}}`

func readOpenCodeSample(sample string) (*openCodeStream, []Event) {
	var events []Event
	s := &openCodeStream{dir: "/w", events: func(e Event) { events = append(events, e) }}
	for _, l := range strings.Split(sample, "\n") {
		s.line([]byte(l))
	}
	return s, events
}

func TestOpenCodeStream(t *testing.T) {
	s, events := readOpenCodeSample(openCodeSample)
	want := []Event{{"read", "change.diff"}, {"search", "used_count"}, {"search", "head/**/*.go"}, {"read", "/etc/hostname"}}
	if !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
	if s.err != nil || s.cost != 0.75 {
		t.Errorf("err = %v, cost = %v", s.err, s.cost)
	}
	// The narration before the tool calls is dropped; the text blocks after them are joined.
	if got := s.result(); got != "=== ツアー JSON ===\n{\"version\":1}" {
		t.Errorf("text = %q", got)
	}
}

func TestOpenCodeStreamErrors(t *testing.T) {
	// An unknown model is reported before the session starts.
	s, _ := readOpenCodeSample(`{"type":"error","timestamp":1,"sessionID":"s","error":{"type":"provider.no-route","message":"Model unavailable: nope/nothing"}}`)
	if s.err == nil || s.err.Error() != "Model unavailable: nope/nothing" {
		t.Errorf("err = %v", s.err)
	}
	// If the model answers and then still calls a tool, the answer is kept.
	s, _ = readOpenCodeSample(`{"type":"text","part":{"text":"{\"version\":1}"}}
{"type":"tool_use","part":{"tool":"read","state":{"input":{"path":"/w/head/a.go"}}}}`)
	if got := s.result(); got != `{"version":1}` {
		t.Errorf("text = %q", got)
	}
}
