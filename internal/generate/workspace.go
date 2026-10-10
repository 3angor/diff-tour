package generate

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DaiAoki/tour/internal/git"
)

// Workspace は LLM に読ませるための作業ディレクトリ。
//
//	base/        変更前（分岐点）の全ファイル
//	head/        変更後の全ファイル
//	change.diff  base から head への差分
//	commits.txt  変更に含まれるコミットのメッセージ
//
// リポジトリそのものではなく書き出したものを読ませるのは、LLM の道具を
// このディレクトリの中に閉じ込めるため（git を使わせなくて済み、作業ツリーにも触れない）。
type Workspace struct {
	Dir string
}

func (w *Workspace) Remove() error { return os.RemoveAll(w.Dir) }

// PrepareWorkspace は dir に作業ディレクトリを作る。リポジトリは読むだけ。
func PrepareWorkspace(repo *git.Repo, base, head, dir string) (*Workspace, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	mb := repo.MergeBase(base, head)
	for name, ref := range map[string]string{"base": mb, "head": head} {
		if err := extract(repo, ref, filepath.Join(dir, name)); err != nil {
			return nil, fmt.Errorf("export %s: %w", ref, err)
		}
	}
	files := map[string]string{
		"change.diff": repo.RawDiff(base, head),
		"commits.txt": repo.CommitMessages(base, head),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return nil, err
		}
	}
	return &Workspace{Dir: dir}, nil
}

func extract(repo *git.Repo, ref, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	r, wait, err := repo.Archive(ref)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			r.Close()
			wait()
			return err
		}
		// 書き出し先の外を指すパスは受け付けない
		name := filepath.Clean(h.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
			// シンボリックリンクなどは書き出さない（作業ディレクトリの外を読ませないため）
		}
	}
	return wait()
}
