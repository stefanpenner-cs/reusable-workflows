package core

import (
	"io/fs"
	"os"
	"path/filepath"
)

// MirrorTree recursively copies src into dest, excluding any `.git` entry — so we graft the
// consumer's working *files* (not its history) onto the runner branch.
func MirrorTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Recreate symlinks as links (never dereference — a dir symlink would break os.ReadFile and
		// abort the whole mirror; a file symlink would silently become a copy).
		if d.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Preserve the exec bit — consumer CI often runs mirrored scripts directly.
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
