// tour は、git の変更をシニアエンジニアの頭の中を追体験するツアーとして見せる。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DaiAoki/tour/internal/config"
	"github.com/DaiAoki/tour/internal/generate"
	"github.com/DaiAoki/tour/internal/git"
	"github.com/DaiAoki/tour/internal/i18n"
	"github.com/DaiAoki/tour/internal/render"
	"github.com/DaiAoki/tour/internal/tour"
	"github.com/DaiAoki/tour/internal/ui"
)

const usage = `usage:
  tour                      choose a branch and tour it
  tour <branch>             tour a branch (compared with the default branch)
  tour <commit>             tour a single commit
  tour <base>..<head>       tour a range
  tour <tour.json>          play a tour file
  tour setup                choose the language and the LLM again

options:
  --repo <path>   repository to read (default: current directory)
  --lang en|ja    language for this run only
  --dump          print every step to stdout instead of opening the TUI
  --cols, --rows  screen size for --dump (default 100x30)
  --plain         --dump without colors
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tour:", err)
		os.Exit(1)
	}
}

func isTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

// parse はフラグと位置引数が混ざっていても読めるようにする（Go の flag は最初の位置引数で止まるため）。
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tour", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	repoDir := fs.String("repo", ".", "")
	langFlag := fs.String("lang", "", "")
	dump := fs.Bool("dump", false, "")
	plain := fs.Bool("plain", false, "")
	cols := fs.Int("cols", 100, "")
	rows := fs.Int("rows", 30, "")
	positional, err := parse(fs, args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(positional) == 1 && positional[0] == "setup" {
		return runSetup(cfg)
	}
	if len(positional) > 1 {
		fs.Usage()
		os.Exit(2)
	}

	lang, _ := i18n.Detect()
	if cfg != nil {
		lang, _ = i18n.Parse(cfg.Lang)
	}
	if *langFlag != "" {
		l, ok := i18n.Parse(*langFlag)
		if !ok {
			return fmt.Errorf("unsupported language: %s", *langFlag)
		}
		lang = l
	}
	i18n.Set(lang)

	dir, err := filepath.Abs(*repoDir)
	if err != nil {
		return err
	}
	repo := git.Open(dir)

	// ツアーのファイルを再生する
	if len(positional) == 1 && strings.HasSuffix(positional[0], ".json") {
		t, err := tour.Load(positional[0])
		if err != nil {
			return err
		}
		ctx := render.Ctx{Tour: t, Repo: repo}
		if *dump {
			fmt.Print(ui.Dump(ctx, *cols, *rows, *plain))
			return nil
		}
		if !isTerminal() {
			return fmt.Errorf("%s", i18n.T("needTTY"))
		}
		_, err = tea.NewProgram(ui.NewViewer(ctx)).Run()
		return err
	}

	if repo.RevParse("HEAD") == "" {
		return fmt.Errorf("%s", i18n.T("err.notRepo", dir))
	}
	if !isTerminal() {
		return fmt.Errorf("%s", i18n.T("needTTY"))
	}
	if cfg == nil {
		if err := runSetup(nil); err != nil {
			return err
		}
		if cfg, _ = config.Load(); cfg == nil {
			return nil // 初期設定の途中で終了した
		}
		if *langFlag == "" {
			lang, _ = i18n.Parse(cfg.Lang)
		}
		i18n.Set(lang)
	}
	backend, err := backendFor(cfg.LLM)
	if err != nil {
		return err
	}

	var app *ui.App
	if len(positional) == 0 {
		base := repo.DefaultBranch()
		if base == "" {
			return fmt.Errorf("%s", i18n.T("err.noBase"))
		}
		app = ui.NewPicker(repo, base, lang, backend)
	} else {
		base, head, err := resolve(repo, positional[0])
		if err != nil {
			return err
		}
		app = ui.NewGenerating(repo, base, head, lang, backend)
	}
	_, err = tea.NewProgram(app).Run()
	return err
}

// resolve は引数を、比べる 2 つのコミットにする。
//
//	a..b / a...b   a の分岐点から b まで
//	ブランチ名     既定のブランチから、そのブランチまで
//	コミット       そのコミット 1 つ（親から、そのコミットまで）
func resolve(repo *git.Repo, arg string) (string, string, error) {
	if base, head, ok := strings.Cut(strings.Replace(arg, "...", "..", 1), ".."); ok {
		for _, r := range []string{base, head} {
			if repo.RevParse(r) == "" {
				return "", "", fmt.Errorf("%s", i18n.T("err.badRef", r))
			}
		}
		return base, head, nil
	}
	if repo.RevParse("refs/heads/"+arg) != "" {
		base := repo.DefaultBranch()
		if base == "" {
			return "", "", fmt.Errorf("%s", i18n.T("err.noBase"))
		}
		return base, arg, nil
	}
	if repo.RevParse(arg) == "" {
		return "", "", fmt.Errorf("%s", i18n.T("err.badRef", arg))
	}
	return arg + "^", arg, nil
}

func backendFor(llm string) (generate.Backend, error) {
	switch llm {
	case "claude-code":
		return generate.ClaudeCode{}, nil
	case "":
		return nil, fmt.Errorf("%s", i18n.T("err.noLLM"))
	}
	name := llm
	for _, o := range ui.DetectLLMs() {
		if o.ID == llm {
			name = o.Name
		}
	}
	return nil, fmt.Errorf("%s", i18n.T("err.unsupported", name))
}

func runSetup(current *config.Config) error {
	if !isTerminal() {
		return fmt.Errorf("%s", i18n.T("needTTY"))
	}
	lang, from := i18n.Detect()
	llm := ""
	if current != nil {
		lang, _ = i18n.Parse(current.Lang)
		from, llm = "", current.LLM
	}
	s := ui.NewSetup(lang, from, llm, ui.DetectLLMs())
	if _, err := tea.NewProgram(s).Run(); err != nil {
		return err
	}
	if !s.Done {
		return nil
	}
	path, err := config.Save(&config.Config{Lang: string(s.Lang()), LLM: s.LLM()})
	if err != nil {
		return err
	}
	i18n.Set(s.Lang())
	fmt.Println(i18n.T("setup.saved", path))
	return nil
}
