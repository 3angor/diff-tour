package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/init-co-jp/diff-tour/internal/git"
	"github.com/init-co-jp/diff-tour/internal/i18n"
)

func gitAt(t *testing.T, dir, date string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, date, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAt(t, dir, date, "add", "-A")
	gitAt(t, dir, date, "commit", "-q", "-m", name)
}

func heads(targets []Target) []string {
	var out []string
	for _, t := range targets {
		out = append(out, t.Head)
	}
	return out
}

func TestFindTargetsPutsCurrentBranchFirst(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	gitAt(t, dir, "2026-01-01T00:00:00Z", "init", "-q", "-b", "main")
	commitFile(t, dir, "2026-01-01T00:00:00Z", "base.txt")
	gitAt(t, dir, "2026-01-02T00:00:00Z", "checkout", "-q", "-b", "older")
	commitFile(t, dir, "2026-01-02T00:00:00Z", "older.txt")
	gitAt(t, dir, "2026-01-03T00:00:00Z", "checkout", "-q", "-b", "newer", "main")
	commitFile(t, dir, "2026-01-03T00:00:00Z", "newer.txt")
	// The current branch is not the most recently committed one
	gitAt(t, dir, "2026-01-03T00:00:00Z", "checkout", "-q", "older")
	repo := git.Open(dir)

	targets := FindTargets(repo, "main", i18n.EN, "test")
	if got := heads(targets); len(got) != 2 || got[0] != "older" || !targets[0].Current {
		t.Fatalf("clean tree: got %v, want the current branch first", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "older.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo = git.Open(dir)
	targets = FindTargets(repo, "main", i18n.EN, "test")
	if got := heads(targets); len(got) != 3 || got[0] != git.Worktree || got[1] != "older" {
		t.Fatalf("dirty tree: got %v, want uncommitted changes, then the current branch", got)
	}
}
