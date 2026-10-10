package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DaiAoki/tour/internal/git"
	"github.com/DaiAoki/tour/internal/tour"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

// 分岐のあとで main が進んでも、"base" は分岐点のコードを指す
func TestBaseRefIsMergeBase(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	commitFile(t, dir, "at fork point\n", "base")
	gitRun(t, dir, "switch", "-q", "-c", "feature")
	commitFile(t, dir, "on feature\n", "feature")
	gitRun(t, dir, "switch", "-q", "main")
	commitFile(t, dir, "main moved on\n", "main")

	ctx := Ctx{Tour: &tour.Tour{Base: "main", Head: "feature"}, Repo: git.Open(dir)}
	if got := ctx.Repo.Show(ctx.Ref("base"), "a.txt"); len(got) != 1 || got[0] != "at fork point" {
		t.Errorf("base = %q", got)
	}
	if got := ctx.Repo.Show(ctx.Ref("head"), "a.txt"); len(got) != 1 || got[0] != "on feature" {
		t.Errorf("head = %q", got)
	}
}
