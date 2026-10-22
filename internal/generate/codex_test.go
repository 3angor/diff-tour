package generate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestCodexArgs(t *testing.T) {
	t.Setenv(codexWindowsReadOnlyEnv, "1")
	args := codexArgs("/w", []string{"/opt/codex/bin/codex"})
	joined := strings.Join(args, " ")
	filesystem := `permissions.diff_tour_read.filesystem={":minimal"="read", ":workspace_roots"={"."="read"}, "/opt/codex/bin/codex"="read"}`
	if runtime.GOOS == "windows" {
		filesystem = `permissions.diff_tour_read.filesystem={":root"="read", "/opt/codex/bin/codex"="read"}`
		if !strings.Contains(joined, `windows.sandbox="elevated"`) {
			t.Fatal("Windows must use the elevated read-only sandbox")
		}
	}
	for _, want := range []string{
		"exec --json",
		"--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check",
		"-C /w",
		`default_permissions="diff_tour_read"`,
		filesystem,
		`approval_policy="never"`, `web_search="disabled"`, `history.persistence="none"`,
		"--disable apps", "--disable plugins", "--disable multi_agent", "--disable memories",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %s", want, joined)
		}
	}
	// --sandbox would switch off the permissions profile, and anything writable or networked must not appear.
	for _, bad := range []string{"--sandbox", "-s", "--add-dir", "--search", "--dangerously-bypass-approvals-and-sandbox", "write", "network"} {
		if slices.Contains(args, bad) || strings.Contains(joined, `"`+bad+`"`) {
			t.Errorf("args contain %q", bad)
		}
	}
	if args[len(args)-1] != "-" {
		t.Errorf("prompt should come from stdin, last arg = %q", args[len(args)-1])
	}
}

func TestCodexWindowsReadOnlyOptIn(t *testing.T) {
	for _, value := range []string{"", "0", "true", "1"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(codexWindowsReadOnlyEnv, value)
			args := strings.Join(codexArgs("/w", nil), " ")
			want := runtime.GOOS == "windows" && value == "1"
			if got := strings.Contains(args, `":root"="read"`); got != want {
				t.Fatalf("whole-disk read = %v, want %v", got, want)
			}
			if got := strings.Contains(args, `windows.sandbox="elevated"`); got != want {
				t.Fatalf("Windows compatibility sandbox = %v, want %v", got, want)
			}
		})
	}
}

func TestCodexWindowsRequiresExplicitOptIn(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific startup guard")
	}
	t.Setenv(codexWindowsReadOnlyEnv, "")
	_, err := (Codex{Bin: "not-an-installed-command"}).Run(context.Background(), t.TempDir(), "test", func(Event) {})
	if err == nil || !strings.Contains(err.Error(), codexWindowsReadOnlyEnv) || !strings.Contains(err.Error(), "whole-disk read-only") {
		t.Fatalf("expected explicit whole-disk read opt-in error, got %v", err)
	}
}

func TestCodexReadablePaths(t *testing.T) {
	dir := t.TempDir()
	native := filepath.Join(dir, "codex")
	os.WriteFile(native, []byte("\x7fELF..."), 0o755)
	if got := codexReadablePaths(native); !reflect.DeepEqual(got, []string{native}) {
		t.Errorf("native: %v", got)
	}
	pkg := filepath.Join(dir, "lib", "node_modules", "@openai", "codex", "bin")
	os.MkdirAll(pkg, 0o755)
	script := filepath.Join(pkg, "codex.js")
	os.WriteFile(script, []byte("#!/usr/bin/env node\n"), 0o755)
	if got, want := codexReadablePaths(script), []string{filepath.Join(dir, "lib", "node_modules", "@openai")}; !reflect.DeepEqual(got, want) {
		t.Errorf("npm: %v, want %v", got, want)
	}
	standalone := filepath.Join(dir, "installs", "codex", "0.160.1")
	os.MkdirAll(filepath.Join(standalone, "bin"), 0o755)
	os.WriteFile(filepath.Join(standalone, "codex-package.json"), []byte(`{"layoutVersion":1,"entrypoint":"bin/codex"}`), 0o644)
	entry := filepath.Join(standalone, "bin", "codex")
	os.WriteFile(entry, []byte("\x7fELF..."), 0o755)
	if got := codexReadablePaths(entry); !reflect.DeepEqual(got, []string{standalone}) {
		t.Errorf("standalone: %v", got)
	}
	// A wrapper script whose target is unknown gets nothing, so the sandbox fails closed.
	wrapper := filepath.Join(dir, "wrapper")
	os.WriteFile(wrapper, []byte("#!/bin/bash\nexec mise x codex -- codex \"$@\"\n"), 0o755)
	if got := codexReadablePaths(wrapper); got != nil {
		t.Errorf("wrapper: %v", got)
	}
}

// Lines recorded from codex-cli 0.160.1 `codex exec --json` (paths shortened).
const codexSample = `{"type":"thread.started","thread_id":"01a12672-35b4-7341-9e8f-bedf520d2ccd"}
{"type":"item.completed","item":{"id":"item_0","type":"error","message":"Codex is ignoring 1 unrecognized configuration setting."}}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"I’ll start by reading the diff."}}
{"type":"item.started","item":{"id":"item_2","type":"command_execution","command":"/usr/bin/bash -lc 'cat change.diff'","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_2","type":"command_execution","command":"/usr/bin/bash -lc 'cat change.diff'","aggregated_output":"diff --git ...\n","exit_code":0,"status":"completed"}}
{"type":"item.started","item":{"id":"item_3","type":"command_execution","command":"/usr/bin/bash -lc \"rg -n \\\"used_count\\\" head/src\"","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.started","item":{"id":"item_4","type":"command_execution","command":"/usr/bin/bash -lc \"sed -n '1,120p' /w/head/src/coupon.ts\"","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.started","item":{"id":"item_5","type":"command_execution","command":"/usr/bin/bash -lc 'ls -la'","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_5","type":"command_execution","command":"/usr/bin/bash -lc 'ls -la'","aggregated_output":"bwrap: ...","exit_code":1,"status":"failed"}}
not json
{"type":"item.completed","item":{"id":"item_6","type":"agent_message","text":"=== ツアー JSON ===\n{\"version\":1}"}}
{"type":"turn.completed","usage":{"input_tokens":44843,"cached_input_tokens":34816,"cache_write_input_tokens":0,"output_tokens":1543,"reasoning_output_tokens":0}}
`

func TestReadCodexStream(t *testing.T) {
	var got []Event
	s, err := readCodexStream(strings.NewReader(codexSample), "/w", func(e Event) { got = append(got, e) })
	if err != nil {
		t.Fatal(err)
	}
	if !s.gotText || s.text != "=== ツアー JSON ===\n{\"version\":1}" {
		t.Errorf("text = %q (got %v)", s.text, s.gotText)
	}
	if s.failure != "" || len(s.warnings) != 1 {
		t.Errorf("failure = %q, warnings = %v", s.failure, s.warnings)
	}
	want := []Event{
		{"read", "change.diff"},
		{"search", "used_count"},
		{"read", "head/src/coupon.ts"},
		{"search", "."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestReadCodexStreamFailure(t *testing.T) {
	in := `{"type":"thread.started","thread_id":"x"}
{"type":"turn.started"}
{"type":"error","message":"stream disconnected before completion"}
{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}
`
	s, _ := readCodexStream(strings.NewReader(in), "/w", func(Event) {})
	if s.gotText || s.failure != "You've hit your usage limit." {
		t.Errorf("stream = %+v", s)
	}
}

func TestReadCodexStreamSandboxBroken(t *testing.T) {
	in := `{"type":"item.completed","item":{"id":"item_3","type":"command_execution","command":"/usr/bin/bash -lc 'cat x'","aggregated_output":"bwrap: execvp /home/u/.local/share/mise/installs/codex/0.160.1/bin/codex: No such file or directory\n","exit_code":1,"status":"failed"}}
{"type":"item.completed","item":{"id":"item_4","type":"agent_message","text":"All shell attempts failed."}}
`
	s, _ := readCodexStream(strings.NewReader(in), "/w", func(Event) {})
	if !strings.HasPrefix(s.broken, "bwrap: execvp ") {
		t.Errorf("broken = %q", s.broken)
	}
}

func TestCommandEvent(t *testing.T) {
	cases := map[string]Event{
		`/usr/bin/bash -lc 'head -n 40 base/a.go'`:               {"read", "base/a.go"},
		`/usr/bin/bash -lc "nl -ba head/a.go | sed -n '10,20p'"`: {"read", "head/a.go"},
		`/usr/bin/bash -lc 'grep -rn "func Apply" head'`:         {"search", "func Apply"},
		`/usr/bin/bash -lc 'git grep -n coupon'`:                 {"search", "coupon"},
		`/usr/bin/bash -lc 'find head -name "*.go"'`:             {"search", "head -name *.go"},
		`/usr/bin/bash -lc 'cat commits.txt && cat change.diff'`: {"read", "commits.txt"},
		`/usr/bin/bash -lc 'python3 -c "print(1)"'`:              {"read", "python3 -c print(1)"},
		`/usr/bin/bash -lc 'cat /etc/hostname'`:                  {"read", "/etc/hostname"},
	}
	for in, want := range cases {
		if got, ok := commandEvent("/w", in); !ok || got != want {
			t.Errorf("commandEvent(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := commandEvent("/w", ""); ok {
		t.Error("empty command should give no event")
	}
}
