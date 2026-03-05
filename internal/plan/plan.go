package plan

import (
	"sort"
	"strings"

	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/remote"
)

// Compute builds a sync plan from local and remote inventories.
// copyOnly: if true, no DeleteFiles or DeleteFolders.
func Compute(localFiles map[string]local.File, remoteFiles map[string]remote.Object, remoteFolders map[string]remote.Folder, copyOnly bool) *Plan {
	p := &Plan{}
	localPaths := make(map[string]struct{})
	for path := range localFiles {
		localPaths[path] = struct{}{}
	}
	remotePaths := make(map[string]struct{})
	for path := range remoteFiles {
		remotePaths[path] = struct{}{}
	}
	localFolderSet := localFoldersFromFiles(localFiles)

	// Upload: in local, not in remote
	for path, f := range localFiles {
		if _, ok := remoteFiles[path]; !ok {
			p.Upload = append(p.Upload, f)
		}
	}
	// Update: in both, size or hash differs
	for path, f := range localFiles {
		r, ok := remoteFiles[path]
		if !ok {
			continue
		}
		if r.Size != f.Size || (f.Hash != "" && r.ID != "" && needHashCompare(f, r)) {
			p.Update = append(p.Update, UpdateAction{Local: f, RemoteID: r.ID})
		}
	}
	if !copyOnly {
		// DeleteFiles: in remote, not in local
		for path, obj := range remoteFiles {
			if _, ok := localFiles[path]; !ok {
				p.DeleteFiles = append(p.DeleteFiles, obj)
			}
		}
		// DeleteFolders: remote folders not in local set, deepest first
		for path, folder := range remoteFolders {
			if _, ok := localFolderSet[path]; !ok {
				p.DeleteFolders = append(p.DeleteFolders, folder)
			}
		}
		sort.Slice(p.DeleteFolders, func(i, j int) bool {
			return pathDepth(p.DeleteFolders[j].Path) < pathDepth(p.DeleteFolders[i].Path)
		})
	}
	return p
}

func needHashCompare(f local.File, r remote.Object) bool {
	// If we have hash on both sides we could compare; for now size is the main signal.
	return false
}

func localFoldersFromFiles(files map[string]local.File) map[string]struct{} {
	out := make(map[string]struct{})
	for path := range files {
		dir := path
		for {
			i := strings.LastIndex(dir, "/")
			if i < 0 {
				break
			}
			dir = dir[:i]
			if dir != "" {
				out[dir] = struct{}{}
			}
		}
	}
	return out
}

func pathDepth(p string) int {
	n := 0
	for _, c := range p {
		if c == '/' {
			n++
		}
	}
	return n
}
