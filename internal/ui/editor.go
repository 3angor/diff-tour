package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/init-co-jp/diff-tour/internal/git"
)

// EditorCommand builds an "open this file at this line" command for commonly used editors.
// If TOUR_EDITOR is set to a command containing "{file}" and "{line}", that is used instead.
func EditorCommand(file string, line int) []string {
	if tmpl := os.Getenv("TOUR_EDITOR"); tmpl != "" {
		var out []string
		for _, a := range strings.Fields(tmpl) {
			a = strings.ReplaceAll(a, "{file}", file)
			out = append(out, strings.ReplaceAll(a, "{line}", strconv.Itoa(line)))
		}
		return out
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := strings.Fields(editor)
	switch filepath.Base(cmd[0]) {
	case "code", "code-insiders", "cursor", "codium", "windsurf":
		return append(cmd, "-g", file+":"+strconv.Itoa(line))
	case "hx", "helix", "micro", "zed", "subl":
		return append(cmd, file+":"+strconv.Itoa(line))
	default:
		return append(cmd, "+"+strconv.Itoa(line), file)
	}
}

var unsafeRef = regexp.MustCompile(`[^\w.-]`)

// PrepareFile decides which file to open. If the working-tree file has the same content as at the tour's point in time, it returns that file.
// Otherwise (when viewing another branch or a PR), it returns a read-only copy of the content at that point.
// In either case, nothing is written to the repository.
func PrepareFile(repo *git.Repo, ref, file string) (path string, copied bool, err error) {
	lines := repo.Show(ref, file)
	if lines == nil {
		return "", false, os.ErrNotExist
	}
	content := strings.Join(lines, "\n")
	real := filepath.Join(repo.Dir, file)
	if b, err := os.ReadFile(real); err == nil && strings.TrimSuffix(string(b), "\n") == content {
		return real, false, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", false, err
	}
	path = filepath.Join(cache, "tour", unsafeRef.ReplaceAllString(ref, "_"), file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	_ = os.Remove(path)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o444); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func editorCmd(path string, line int) *exec.Cmd {
	argv := EditorCommand(path, line)
	return exec.Command(argv[0], argv[1:]...)
}
