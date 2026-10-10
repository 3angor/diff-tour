package git

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree は「作業ツリー（未コミットの変更を含む）」を表す特別な参照。
// head にこれを指定すると、コミットではなく、いまのファイルを読む。
// 読むのは git が追跡しているファイルと、無視されていない未追跡のファイルだけ（.gitignore にある .env などは読まない）。
const Worktree = "WORKTREE"

// Dirty は未コミットの変更（未追跡のファイルを含む）があるかどうか。
func (r *Repo) Dirty() bool {
	out := r.run("status", "--porcelain", "--untracked-files=normal")
	return out != nil && strings.TrimSpace(*out) != ""
}

// worktreeFiles は作業ツリーで読んでよいファイルの一覧。
func (r *Repo) worktreeFiles() map[string]bool {
	out := r.run("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	files := map[string]bool{}
	if out == nil {
		return files
	}
	for _, f := range strings.Split(*out, "\x00") {
		if f == "" {
			continue
		}
		// 削除したがまだコミットしていないファイルは、一覧に残っていても読めない
		if info, err := os.Lstat(filepath.Join(r.Dir, f)); err == nil && info.Mode().IsRegular() {
			files[f] = true
		}
	}
	return files
}

func (r *Repo) untracked() []string {
	out := r.run("ls-files", "-z", "--others", "--exclude-standard")
	if out == nil {
		return nil
	}
	return nonEmpty(strings.Split(*out, "\x00"))
}

func (r *Repo) showWorktree(file string) []string {
	if !r.worktreeFiles()[file] {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(r.Dir, file))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// untrackedDiff は未追跡のファイルを「新しく足したファイル」として差分にする。
func (r *Repo) untrackedDiff(file string) string {
	cmd := exec.Command("git", "-C", r.Dir, "--no-pager", "diff", "--no-color", "--no-ext-diff", "-U3", "--no-index", "--", os.DevNull, file)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	// --no-index は、違いがあると終了コード 1 を返す
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return ""
	}
	return string(out)
}

func (r *Repo) worktreeDiff(base string, file ...string) string {
	mb := r.MergeBase(base, Worktree)
	args := append([]string{"diff", "--no-color", "--no-ext-diff", "-U3", mb, "--"}, file...)
	var b strings.Builder
	if out := r.run(args...); out != nil {
		b.WriteString(*out)
	}
	want := map[string]bool{}
	for _, f := range file {
		want[f] = true
	}
	for _, f := range r.untracked() {
		if len(file) == 0 || want[f] {
			b.WriteString(r.untrackedDiff(f))
		}
	}
	return b.String()
}

// worktreeID は作業ツリーの中身から決まる識別子（保存したツアーを使い回してよいかの判断に使う）。
func (r *Repo) worktreeID() string {
	h := sha256.New()
	h.Write([]byte(r.RevParse("HEAD")))
	h.Write([]byte(r.worktreeDiff("HEAD")))
	return "worktree-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// worktreeStat は差分の文字列から、変わったファイルの数と行数を数える。
func worktreeStat(diff string) Stat {
	var s Stat
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			s.Files++
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "+"):
			s.Insertions++
		case strings.HasPrefix(l, "-"):
			s.Deletions++
		}
	}
	return s
}
