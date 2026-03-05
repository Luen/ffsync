package plan

import (
	"testing"

	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/remote"
)

func TestCompute(t *testing.T) {
	localFiles := map[string]local.File{
		"a.txt":  {Path: "a.txt", Size: 10, Mtime: 100},
		"b/c.txt": {Path: "b/c.txt", Size: 20, Mtime: 200},
	}
	remoteFiles := map[string]remote.Object{
		"a.txt":  {ID: "id1", Path: "a.txt", Size: 10},
		"old.txt": {ID: "id2", Path: "old.txt", Size: 1},
	}
	remoteFolders := map[string]remote.Folder{
		"b":     {ID: "fb", Path: "b", Name: "b"},
		"orphan": {ID: "fo", Path: "orphan", Name: "orphan"},
	}

	p := Compute(localFiles, remoteFiles, remoteFolders, false)
	if len(p.Upload) != 1 || p.Upload[0].Path != "b/c.txt" {
		t.Errorf("Upload: %+v", p.Upload)
	}
	if len(p.Update) != 0 {
		t.Errorf("Update: %+v", p.Update)
	}
	if len(p.DeleteFiles) != 1 || p.DeleteFiles[0].Path != "old.txt" {
		t.Errorf("DeleteFiles: %+v", p.DeleteFiles)
	}
	if len(p.DeleteFolders) != 1 || p.DeleteFolders[0].Path != "orphan" {
		t.Errorf("DeleteFolders (should be deepest first, orphan only): %+v", p.DeleteFolders)
	}
}

func TestComputeCopyOnly(t *testing.T) {
	localFiles := map[string]local.File{"a.txt": {}}
	remoteFiles := map[string]remote.Object{"old.txt": {}}
	remoteFolders := map[string]remote.Folder{"x": {}}
	p := Compute(localFiles, remoteFiles, remoteFolders, true)
	if len(p.DeleteFiles) != 0 || len(p.DeleteFolders) != 0 {
		t.Errorf("copyOnly should have no deletes: %+v", p)
	}
}
