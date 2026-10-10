// Package generate は LLM にツアーを書かせる。
package generate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/DaiAoki/tour/internal/git"
	"github.com/DaiAoki/tour/internal/i18n"
	"github.com/DaiAoki/tour/internal/render"
	"github.com/DaiAoki/tour/internal/tour"
)

//go:embed prompts/*.md
var prompts embed.FS

// PromptVersion はプロンプトを変えたら上げる。保存済みのツアーを作り直させるため。
const PromptVersion = "v1"

// MaxRepairs は、検証で誤りが見つかったときに LLM に直させる回数の上限。
const MaxRepairs = 2

type Request struct {
	Repo *git.Repo
	Base string
	Head string
	Lang i18n.Lang
}

// Progress は生成の進み具合。
type Progress struct {
	Event  *Event
	Phase  string // "reading"、"repairing"
	Repair int
}

type Stats struct {
	CostUSD float64
	Repairs int
}

var languageRules = map[i18n.Lang]string{
	i18n.EN: "Write all text in English. Use short, plain sentences.",
	i18n.JA: "文章は日本語。です・ます調ではなく、短い常体（「〜している」「〜になる」）で書く。",
}

func buildPrompt(req Request) (string, error) {
	var b strings.Builder
	for _, name := range []string{"common.md", "planC.md", "format.md", "output-C.md"} {
		src, err := prompts.ReadFile("prompts/" + name)
		if err != nil {
			return "", err
		}
		t, err := template.New(name).Parse(string(src))
		if err != nil {
			return "", err
		}
		if err := t.Execute(&b, map[string]string{"LanguageRule": languageRules[req.Lang]}); err != nil {
			return "", err
		}
		b.WriteString("\n\n")
		if name == "common.md" {
			fmt.Fprintf(&b, "対象の変更: base は `%s`、head は `%s`。JSON の base と head にはこの名前をそのまま書く。\n", req.Base, req.Head)
			if req.Head == git.Worktree {
				b.WriteString("head はコミットではなく、まだコミットしていない変更を含む作業ツリー。commits.txt にあるコミットのあとに、未コミットの変更が続いている。未コミットの変更だけの差分は uncommitted.diff にある。\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// separator は、下調べとツアーの JSON の区切り（prompts/output-C.md で指示している）。
const separator = "=== ツアー JSON ==="

// extractJSON は返答から JSON の部分を取り出す（下調べ、```json の囲み、前置きがあっても外す）。
func extractJSON(text string) []byte {
	if _, after, ok := strings.Cut(text, separator); ok {
		text = after
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil
	}
	return []byte(text[start : end+1])
}

// parse は返答をツアーにし、形式と、指しているファイル・行の実在を確かめる。問題があれば一覧で返す。
func parse(req Request, text string) (*tour.Tour, []string) {
	raw := extractJSON(text)
	if raw == nil {
		return nil, []string{"the reply does not contain a JSON object"}
	}
	var t tour.Tour
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, []string{"invalid JSON: " + err.Error()}
	}
	// 名前の書き間違いで別のコミットを読まないよう、base と head は依頼したものに揃える
	t.Base, t.Head = req.Base, req.Head
	if err := t.Validate(); err != nil {
		return &t, []string{err.Error()}
	}
	return &t, render.Check(render.Ctx{Tour: &t, Repo: req.Repo})
}

func readPrompt(name string) string {
	b, err := prompts.ReadFile("prompts/" + name)
	if err != nil {
		panic(err) // 埋め込んだファイルなので、ないことはない
	}
	return string(b)
}

// repairPrompt は検証で見つかった誤りだけを直させる。直す前の JSON は作業ディレクトリの draft.txt に置く。
func repairPrompt(problems []string) string {
	return strings.Join([]string{
		readPrompt("fix.md"),
		"## 検証の結果\n- " + strings.Join(problems, "\n- "),
		readPrompt("format.md"),
		readPrompt("fix-output.md"),
	}, "\n\n")
}

// Generate はツアーを書かせる。検証で問題があれば、MaxRepairs 回まで直させる。
func Generate(ctx context.Context, backend Backend, req Request, workDir string, progress func(Progress)) (*tour.Tour, Stats, error) {
	var stats Stats
	ws, err := PrepareWorkspace(req.Repo, req.Base, req.Head, workDir)
	if err != nil {
		return nil, stats, err
	}
	defer ws.Remove()

	prompt, err := buildPrompt(req)
	if err != nil {
		return nil, stats, err
	}
	onEvent := func(e Event) { progress(Progress{Event: &e, Phase: "reading", Repair: stats.Repairs}) }
	progress(Progress{Phase: "reading"})
	res, err := backend.Run(ctx, ws.Dir, prompt, onEvent)
	stats.CostUSD += res.CostUSD
	if err != nil {
		return nil, stats, err
	}
	// 返答に JSON が全くない（途中で止まった、など）なら、直させるのではなく 1 回だけ作り直す
	if extractJSON(res.Text) == nil {
		res, err = backend.Run(ctx, ws.Dir, prompt, onEvent)
		stats.CostUSD += res.CostUSD
		if err != nil {
			return nil, stats, err
		}
	}
	for {
		t, problems := parse(req, res.Text)
		if len(problems) == 0 {
			return t, stats, nil
		}
		if stats.Repairs >= MaxRepairs {
			return nil, stats, &InvalidTourError{Problems: problems}
		}
		stats.Repairs++
		progress(Progress{Phase: "repairing", Repair: stats.Repairs})
		draft := extractJSON(res.Text)
		if draft == nil {
			draft = []byte(res.Text)
		}
		if err := os.WriteFile(filepath.Join(ws.Dir, "draft.txt"), draft, 0o644); err != nil {
			return nil, stats, err
		}
		res, err = backend.Run(ctx, ws.Dir, repairPrompt(problems), onEvent)
		stats.CostUSD += res.CostUSD
		if err != nil {
			return nil, stats, err
		}
	}
}

// InvalidTourError は、直させても形式や指定の誤りが残ったことを表す。
type InvalidTourError struct{ Problems []string }

func (e *InvalidTourError) Error() string {
	return "the generated tour is still invalid: " + strings.Join(e.Problems, "; ")
}

// CacheKey は、同じ変更・同じ言語・同じプロンプト・同じ LLM なら同じになる。
func CacheKey(repo *git.Repo, base, head string, lang i18n.Lang, backend string) string {
	h := sha256.New()
	for _, s := range []string{repo.MergeBase(base, head), repo.RevParse(head), string(lang), PromptVersion, backend} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tour"), nil
}

// CachePath は保存したツアーの場所。
func CachePath(key string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tours", key+".json"), nil
}

// WorkDir は作業ディレクトリの場所（生成が終われば消す）。
func WorkDir(key string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "work", key), nil
}

// LoadCached は保存したツアーを読む。なければ nil。
func LoadCached(key string) (*tour.Tour, error) {
	path, err := CachePath(key)
	if err != nil {
		return nil, err
	}
	t, err := tour.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return t, err
}

func SaveCached(key string, t *tour.Tour) error {
	path, err := CachePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
