package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCopilotArgs(t *testing.T) {
	args := copilotArgs("gpt-5")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--output-format json", "--available-tools=view,grep,rg,glob", "--deny-tool=shell", "--deny-tool=write",
		"--disallow-temp-dir", "--disable-builtin-mcps", "--no-custom-instructions", "--no-ask-user",
		"--no-remote", "--no-remote-export", "--no-auto-update", "--model gpt-5",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %s", want, joined)
		}
	}
	// Anything that approves or widens access must not appear, and the prompt comes from stdin, not -p.
	for _, bad := range []string{"--allow-all-tools", "--allow-all", "--yolo", "--allow-all-paths", "--allow-all-urls", "--add-dir", "--plugin-dir", "--enable-memory", "--share", "--share-gist", "-p", "--prompt", "--autopilot"} {
		if slices.Contains(args, bad) || strings.HasPrefix(joined, bad+"=") || strings.Contains(joined, " "+bad+"=") {
			t.Errorf("args contain %s", bad)
		}
	}
	if slices.Contains(copilotArgs(""), "--model") {
		t.Error("--model without a model")
	}
}

func TestPrepareCopilotHome(t *testing.T) {
	user, home := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(user, "config.json"), []byte(`{
  "lastLoggedInUser": {"host": "https://github.com", "login": "me"},
  "loggedInUsers": [{"host": "https://github.com", "login": "me"}],
  "trustedFolders": ["/w"],
  "installedPlugins": [{"name": "p"}],
  "enabledPlugins": {"p": true},
  "extraKnownMarketplaces": {},
  "allowedUrls": ["evil.example"],
  "banner": "never",
}`), 0o600)
	os.WriteFile(filepath.Join(user, "settings.json"), []byte(`{"model": "claude-sonnet-5.5", "hooks": {}}`), 0o600)
	for _, f := range []string{"mcp-config.json", "permissions-config.json", "copilot-instructions.md"} {
		os.WriteFile(filepath.Join(user, f), []byte("{}"), 0o600)
	}
	os.MkdirAll(filepath.Join(user, "hooks"), 0o755)

	model, err := prepareCopilotHome(user, home)
	if err != nil {
		t.Fatal(err)
	}
	if model != "claude-sonnet-5.5" {
		t.Errorf("model = %q", model)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("home has %v, want only config.json", entries)
	}
	var c map[string]any
	b, _ := os.ReadFile(filepath.Join(home, "config.json"))
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range c {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"banner", "lastLoggedInUser", "loggedInUsers"}; !slices.Equal(keys, want) {
		t.Errorf("config keys = %v, want %v", keys, want)
	}
	if info, _ := os.Stat(filepath.Join(home, "config.json")); info.Mode().Perm() != 0o600 {
		t.Errorf("config.json mode = %v", info.Mode().Perm())
	}

	// No Copilot directory at all (signed in with a token or gh): nothing is written.
	empty := t.TempDir()
	if model, err := prepareCopilotHome(filepath.Join(user, "missing"), empty); err != nil || model != "" {
		t.Errorf("missing: %q, %v", model, err)
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("missing: wrote %v", entries)
	}
}

func TestCopilotEnv(t *testing.T) {
	t.Setenv("COPILOT_ALLOW_ALL", "true")
	t.Setenv("GITHUB_COPILOT_PROMPT_MODE_REPO_HOOKS", "true")
	t.Setenv("GITHUB_COPILOT_PROMPT_MODE_WORKSPACE_MCP", "true")
	t.Setenv("COPILOT_CUSTOM_INSTRUCTIONS_DIRS", "/x")
	t.Setenv("COPILOT_SKILLS_DIRS", "/x")
	t.Setenv("COPILOT_PROMPT_TURN_DELIMITER", "---")
	t.Setenv("COPILOT_HOME", "/home/me/.copilot")
	last := map[string]string{}
	for _, kv := range copilotEnv("/tmp/h") {
		k, v, _ := strings.Cut(kv, "=")
		last[k] = v // exec uses the last value of a duplicate key
	}
	want := map[string]string{"COPILOT_HOME": "/tmp/h", "COPILOT_ALLOW_ALL": "false", "COPILOT_AUTO_UPDATE": "false"}
	for k, v := range want {
		if last[k] != v {
			t.Errorf("%s = %q, want %q", k, last[k], v)
		}
	}
	for _, k := range []string{"GITHUB_COPILOT_PROMPT_MODE_REPO_HOOKS", "GITHUB_COPILOT_PROMPT_MODE_WORKSPACE_MCP", "COPILOT_CUSTOM_INSTRUCTIONS_DIRS", "COPILOT_SKILLS_DIRS", "COPILOT_PROMPT_TURN_DELIMITER"} {
		if _, ok := last[k]; ok {
			t.Errorf("%s must be removed", k)
		}
	}
}

// copilotSample is trimmed from a run of Copilot CLI 1.0.98 (--output-format json --stream off).
const copilotSample = `{"type":"session.info","data":{"infoType":"configuration","message":"Disabled tools: bash, create, edit"},"ephemeral":true}
{"type":"user.message","data":{"content":"go"}}
{"type":"assistant.message","data":{"content":"Let me read the diff.","toolRequests":[{"toolCallId":"c0","name":"view","arguments":{"path":"/w/change.diff"},"type":"function"}]}}
{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"c1","name":"grep","arguments":{"pattern":"used_count"},"type":"function"},{"toolCallId":"c2","name":"glob","arguments":{"pattern":"head/**/*.go"},"type":"function"}]}}
{"type":"assistant.message","data":{"content":"sub-agent text","parentToolCallId":"c9","toolRequests":[{"toolCallId":"c3","name":"view","arguments":{"path":"/w/x"},"type":"function"}]}}
{"type":"assistant.message","data":{"content":"=== ツアー JSON ===\n{\"version\":1}","toolRequests":[]}}
{"type":"result","sessionId":"s","exitCode":0,"usage":{"premiumRequests":1}}`

func readCopilotSample(sample string) (*copilotStream, []Event) {
	var events []Event
	s := &copilotStream{dir: "/w", events: func(e Event) { events = append(events, e) }}
	for _, l := range strings.Split(sample, "\n") {
		s.line([]byte(l))
	}
	return s, events
}

func TestCopilotStream(t *testing.T) {
	s, events := readCopilotSample(copilotSample)
	want := []Event{{"read", "change.diff"}, {"search", "used_count"}, {"search", "head/**/*.go"}}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
	if !s.gotResult || s.err != nil {
		t.Errorf("gotResult = %v, err = %v", s.gotResult, s.err)
	}
	if s.text != "=== ツアー JSON ===\n{\"version\":1}" {
		t.Errorf("text = %q", s.text)
	}
}

func TestCopilotStreamErrors(t *testing.T) {
	s, _ := readCopilotSample(`{"type":"session.error","data":{"errorType":"quota","message":"You have run out of AI credits."}}
{"type":"result","exitCode":1}`)
	if s.err == nil || s.err.Error() != "You have run out of AI credits." {
		t.Errorf("err = %v", s.err)
	}
	// A failed model call is retried, so it alone does not fail the run.
	s, _ = readCopilotSample(`{"type":"session.error","data":{"errorType":"model_call","message":"retrying"}}
{"type":"assistant.message","data":{"content":"done"}}
{"type":"result","exitCode":0}`)
	if s.err != nil || s.text != "done" {
		t.Errorf("err = %v, text = %q", s.err, s.text)
	}
	s, _ = readCopilotSample(`{"type":"result","exitCode":1}`)
	if s.err == nil {
		t.Error("a non-zero exit code must fail")
	}
}
