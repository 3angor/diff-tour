// Package git は tour が使う git の操作をまとめる。
// tour は読み取り専用なので、作業ツリーも .git も書き換えないコマンドだけを使う。
package git

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type DiffLine struct {
	Kind  byte // ' '、'+'、'-'
	OldNo int  // 0 は「その側にない」
	NewNo int
	Text  string
}

type Hunk struct {
	Header string
	Lines  []DiffLine
}

type Commit struct {
	Hash    string
	Subject string
}

type Repo struct {
	Dir   string
	mu    sync.Mutex
	cache map[string]*string
}

func Open(dir string) *Repo {
	return &Repo{Dir: dir, cache: map[string]*string{}}
}

// run は git を実行して標準出力を返す。失敗したら nil。
func (r *Repo) run(args ...string) *string {
	key := strings.Join(args, "\x00")
	r.mu.Lock()
	if out, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return out
	}
	r.mu.Unlock()

	cmd := exec.Command("git", append([]string{"-C", r.Dir, "--no-pager"}, args...)...)
	// ロックファイルなどの付随的な書き込みもさせない
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var res *string
	if b, err := cmd.Output(); err == nil {
		s := string(b)
		res = &s
	}
	r.mu.Lock()
	r.cache[key] = res
	r.mu.Unlock()
	return res
}

// Show はある時点のファイルの中身を行ごとに返す。存在しなければ nil。
func (r *Repo) Show(ref, file string) []string {
	out := r.run("show", ref+":"+file)
	if out == nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(*out, "\n"), "\n")
}

// Diff は base から分岐して以降の head での変更（三点リーダー＝分岐点との比較）。
func (r *Repo) Diff(base, head, file string) ([]Hunk, bool) {
	out := r.run("diff", "--no-color", "--no-ext-diff", "-U3", base+"..."+head, "--", file)
	if out == nil {
		return nil, false
	}
	return ParseDiff(*out), true
}

// MergeBase は a と b の分岐点のコミット。見つからなければ a をそのまま返す。
func (r *Repo) MergeBase(a, b string) string {
	if out := r.run("merge-base", a, b); out != nil {
		return strings.TrimSpace(*out)
	}
	return a
}

// ListFiles はある時点で dir の下にあるファイルのパスを返す。
func (r *Repo) ListFiles(ref, dir string) []string {
	out := r.run("ls-tree", "-r", "--name-only", ref, "--", dir)
	if out == nil {
		return nil
	}
	return nonEmpty(strings.Split(*out, "\n"))
}

// Commits は base になくて head にあるコミットを、古い順に返す。
func (r *Repo) Commits(base, head string) []Commit {
	out := r.run("log", "--reverse", "--format=%h%x09%s", base+".."+head)
	if out == nil {
		return nil
	}
	var commits []Commit
	for _, l := range nonEmpty(strings.Split(*out, "\n")) {
		hash, subject, _ := strings.Cut(l, "\t")
		commits = append(commits, Commit{hash, subject})
	}
	return commits
}

type Stat struct{ Files, Insertions, Deletions int }

var statRe = regexp.MustCompile(`(\d+) (file|insertion|deletion)`)

func (r *Repo) Shortstat(base, head string) Stat {
	var s Stat
	out := r.run("diff", "--shortstat", base+"..."+head)
	if out == nil {
		return s
	}
	for _, m := range statRe.FindAllStringSubmatch(*out, -1) {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "file":
			s.Files = n
		case "insertion":
			s.Insertions = n
		case "deletion":
			s.Deletions = n
		}
	}
	return s
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)`)

func ParseDiff(s string) []Hunk {
	var hunks []Hunk
	var oldNo, newNo int
	for _, line := range strings.Split(s, "\n") {
		if m := hunkRe.FindStringSubmatch(line); m != nil {
			oldNo, _ = strconv.Atoi(m[1])
			newNo, _ = strconv.Atoi(m[2])
			hunks = append(hunks, Hunk{Header: strings.TrimSpace(m[3])})
			continue
		}
		if len(hunks) == 0 || line == "" {
			continue
		}
		h := &hunks[len(hunks)-1]
		switch line[0] {
		case '+':
			h.Lines = append(h.Lines, DiffLine{Kind: '+', NewNo: newNo, Text: line[1:]})
			newNo++
		case '-':
			h.Lines = append(h.Lines, DiffLine{Kind: '-', OldNo: oldNo, Text: line[1:]})
			oldNo++
		case ' ':
			h.Lines = append(h.Lines, DiffLine{Kind: ' ', OldNo: oldNo, NewNo: newNo, Text: line[1:]})
			oldNo++
			newNo++
		}
	}
	return hunks
}

func nonEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
