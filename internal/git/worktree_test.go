package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorktree(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "one\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), ".env\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")

	writeFile(t, filepath.Join(dir, "a.txt"), "one\ntwo\n")    // 追跡しているファイルの変更
	writeFile(t, filepath.Join(dir, "new.txt"), "brand new\n") // 未追跡のファイル
	writeFile(t, filepath.Join(dir, ".env"), "SECRET=1\n")     // 無視しているファイル
	index := filepath.Join(dir, ".git", "index")
	before, _ := os.Stat(index)
	time.Sleep(10 * time.Millisecond)

	r := Open(dir)
	if !r.Dirty() {
		t.Fatal("expected dirty")
	}
	if got := r.Show(Worktree, "a.txt"); strings.Join(got, "|") != "one|two" {
		t.Errorf("show a.txt = %q", got)
	}
	if got := r.Show(Worktree, ".env"); got != nil {
		t.Errorf("ignored file must not be readable, got %q", got)
	}
	files := strings.Join(r.ListFiles(Worktree, ""), ",")
	if !strings.Contains(files, "new.txt") || strings.Contains(files, ".env") {
		t.Errorf("files = %s", files)
	}
	if hunks, _ := r.Diff("main", Worktree, "new.txt"); len(hunks) != 1 || hunks[0].Lines[0].Text != "brand new" {
		t.Errorf("untracked diff = %+v", hunks)
	}
	if hunks, _ := r.Diff("main", Worktree, "a.txt"); len(hunks) != 1 {
		t.Errorf("tracked diff = %+v", hunks)
	}
	diff := r.RawDiff("main", Worktree)
	if strings.Contains(diff, "SECRET") || !strings.Contains(diff, "+brand new") || !strings.Contains(diff, "+two") {
		t.Errorf("raw diff = %s", diff)
	}
	if s := r.Shortstat("main", Worktree); s.Files != 2 || s.Insertions != 2 {
		t.Errorf("stat = %+v", s)
	}
	after, _ := os.Stat(index)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error(".git/index was rewritten")
	}
}
