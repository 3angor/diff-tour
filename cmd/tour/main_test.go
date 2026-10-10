package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/init-co-jp/diff-tour/internal/git"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", name)
}

func TestResolveRemoteBranch(t *testing.T) {
	upstream := t.TempDir()
	gitIn(t, upstream, "init", "-q", "-b", "main")
	commit(t, upstream, "base.txt")
	gitIn(t, upstream, "checkout", "-q", "-b", "feature/x")
	commit(t, upstream, "one.txt")
	commit(t, upstream, "two.txt")
	gitIn(t, upstream, "checkout", "-q", "main")

	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, upstream, "clone", "-q", upstream, clone)
	repo := git.Open(clone)

	base, head, err := resolve(repo, "origin/feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if head != "origin/feature/x" || base == "origin/feature/x^" {
		t.Fatalf("got %s..%s, want the whole branch from the default branch", base, head)
	}
	if n := repo.Ahead(base, head); n != 2 {
		t.Fatalf("%d commits in %s..%s, want 2", n, base, head)
	}
}
