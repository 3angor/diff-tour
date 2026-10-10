package generate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Event は生成中に起きたこと（画面に流す用）。
type Event struct {
	Kind   string // "read"、"search"
	Detail string
}

type Result struct {
	Text    string
	CostUSD float64
}

// Backend はツアーを書く LLM。
type Backend interface {
	Name() string
	// Run は dir を作業ディレクトリにして prompt を渡し、最後の返答を返す。
	Run(ctx context.Context, dir, prompt string, events func(Event)) (Result, error)
}

// ClaudeCode は Claude Code の CLI（claude -p）を、読み取りの道具だけに絞って動かす。
type ClaudeCode struct {
	Bin string
}

func (c ClaudeCode) Name() string { return "Claude Code" }

// claudeArgs は読み取り専用で動かすためのフラグ。
//
//	--restricted                コマンドを実行する道具と WebFetch を外し、ファイルの道具を作業ディレクトリに閉じ込める
//	--strict-mcp-config         MCP サーバーを読み込まない
//	--tools Read,Grep,Glob      使える道具を読み取りの 3 つだけにする
//	--permission-mode dontAsk   確認が必要な操作は、聞かずに拒否する
func claudeArgs(prompt string) []string {
	return []string{
		"-p",
		"--restricted", "--strict-mcp-config",
		"--tools", "Read,Grep,Glob",
		"--permission-mode", "dontAsk", "--permission-prompts", "none",
		"--no-session-persistence",
		"--output-format", "stream-json", "--verbose",
		prompt,
	}
}

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Result  string  `json:"result"`
	IsError bool    `json:"is_error"`
	Cost    float64 `json:"total_cost_usd"`
}

func (c ClaudeCode) Run(ctx context.Context, dir, prompt string, events func(Event)) (Result, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	cmd := exec.CommandContext(ctx, bin, claudeArgs(prompt)...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	var res Result
	gotResult := false
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var e streamEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		switch e.Type {
		case "assistant":
			for _, part := range e.Message.Content {
				if part.Type != "tool_use" {
					continue
				}
				var in struct {
					FilePath string `json:"file_path"`
					Pattern  string `json:"pattern"`
					Path     string `json:"path"`
				}
				_ = json.Unmarshal(part.Input, &in)
				switch part.Name {
				case "Read":
					events(Event{"read", relative(dir, in.FilePath)})
				case "Grep", "Glob":
					events(Event{"search", in.Pattern})
				}
			}
		case "result":
			res = Result{Text: e.Result, CostUSD: e.Cost}
			gotResult = true
			if e.IsError {
				err = errors.New(strings.TrimSpace(e.Result))
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if err != nil {
		return res, err
	}
	if !gotResult {
		msg := strings.TrimSpace(stderr.String())
		if waitErr != nil && msg == "" {
			msg = waitErr.Error()
		}
		return res, fmt.Errorf("claude: %s", msg)
	}
	return res, nil
}

func relative(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}
