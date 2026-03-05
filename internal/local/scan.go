package local

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ffsync/ffsync/pkg/pathutil"
)

// DefaultExclude is applied when no exclude list is given (skip VCS and state).
var DefaultExclude = []string{".git", ".ffsync", ".ffsync.lock", ".ffsync.*"}

// Scan walks the local tree and returns a map path -> File (path normalised with /).
// include/exclude are glob patterns (forward slash); default exclude skips .git, .ffsync.
func Scan(root string, include, exclude []string) (map[string]File, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}
	if len(exclude) == 0 {
		exclude = DefaultExclude
	}
	out := make(map[string]File)
	err = filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		key := pathutil.Normalise(filepath.ToSlash(rel))
		if d.IsDir() {
			base := filepath.Base(full)
			for _, ex := range exclude {
				ok, _ := filepath.Match(ex, base)
				if ok {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !pathutil.MatchGlob(include, exclude, key) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[key] = File{
			Path:  key,
			Size:  info.Size(),
			Mtime: info.ModTime().Unix(),
		}
		return nil
	})
	return out, err
}

// SkipDir returns true if the directory name should be skipped (e.g. .git).
func SkipDir(name string) bool {
	return strings.HasPrefix(name, ".") && (name == ".git" || name == ".ffsync" || strings.HasPrefix(name, ".ffsync."))
}
