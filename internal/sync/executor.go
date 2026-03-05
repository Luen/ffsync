package sync

import (
	"context"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ffsync/ffsync/internal/client"
	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/plan"
	"github.com/ffsync/ffsync/internal/remote"
)

// ExecutorOptions configures the sync executor.
type ExecutorOptions struct {
	DryRun    bool
	Transfers int
	StatePath string
}

// Execute runs the plan: uploads, updates (upload then delete old), deletes files, then deletes folders.
func Execute(ctx context.Context, p *plan.Plan, cl *client.Client, localRoot, baseFolderID, statePath string, opts ExecutorOptions) error {
	if opts.Transfers <= 0 {
		opts.Transfers = 4
	}
	if opts.DryRun {
		logPlan(p)
		return nil
	}
	state, _ := local.LoadState(statePath)
	if state == nil {
		state = &local.State{Files: make(map[string]local.FileState), Folders: make(map[string]string)}
	}
	var stateMu sync.Mutex
	sem := make(chan struct{}, opts.Transfers)
	var wg sync.WaitGroup
	for _, f := range p.Upload {
		f := f
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}
			runUpload(ctx, cl, localRoot, baseFolderID, f, state, &stateMu, nil)
		}()
	}
	for _, u := range p.Update {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}
			runUpload(ctx, cl, localRoot, baseFolderID, u.Local, state, &stateMu, &u.RemoteID)
		}()
	}
	wg.Wait()
	for _, obj := range p.DeleteFiles {
		if err := cl.DeleteFileEntry(ctx, obj.ID); err != nil {
			slog.Error("delete file", "path", obj.Path, "err", err)
		}
		delete(state.Files, obj.Path)
	}
	for _, folder := range p.DeleteFolders {
		if err := cl.DeleteFileEntry(ctx, folder.ID); err != nil {
			slog.Error("delete folder", "path", folder.Path, "err", err)
		}
		delete(state.Folders, folder.Path)
	}
	if opts.StatePath != "" {
		_ = local.SaveState(opts.StatePath, state)
	}
	return nil
}

func runUpload(ctx context.Context, cl *client.Client, localRoot, baseFolderID string, f local.File, state *local.State, stateMu *sync.Mutex, deleteAfterID *string) {
	parentPath := filepath.Dir(f.Path)
	if parentPath == "." {
		parentPath = ""
	}
	parentPath = filepath.ToSlash(parentPath)
	parentID, err := cl.EnsureFolderPath(ctx, baseFolderID, parentPath)
	if err != nil {
		slog.Error("ensure folder", "path", parentPath, "err", err)
		return
	}
	fullPath := filepath.Join(localRoot, filepath.FromSlash(f.Path))
	file, err := os.Open(fullPath)
	if err != nil {
		slog.Error("open file", "path", fullPath, "err", err)
		return
	}
	defer file.Close()
	name := filepath.Base(f.Path)
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	if ext == "" && strings.HasPrefix(name, ".") {
		ext = "txt"
	}
	mimeType := mime.TypeByExtension(filepath.Ext(name))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	_, err = cl.UploadFile(ctx, parentID, name, mimeType, ext, f.Size, file)
	if err != nil {
		slog.Error("upload", "path", f.Path, "err", err)
		return
	}
	if state != nil && stateMu != nil {
		stateMu.Lock()
		state.Files[f.Path] = local.FileState{Size: f.Size, Mtime: f.Mtime, Hash: f.Hash}
		stateMu.Unlock()
	}
	if deleteAfterID != nil && *deleteAfterID != "" {
		_ = cl.DeleteFileEntry(ctx, *deleteAfterID)
	}
}

func logPlan(p *plan.Plan) {
	for _, f := range p.Upload {
		slog.Info("would upload", "path", f.Path)
	}
	for _, u := range p.Update {
		slog.Info("would update", "path", u.Local.Path)
	}
	for _, obj := range p.DeleteFiles {
		slog.Info("would delete file", "path", obj.Path)
	}
	for _, folder := range p.DeleteFolders {
		slog.Info("would delete folder", "path", folder.Path)
	}
}

// RemoteObjectFromEntry converts client file entry to remote.Object.
func RemoteObjectFromEntry(path string, e client.FileEntryResponse) remote.Object {
	return remote.Object{
		ID:        e.ID.String(),
		Path:      path,
		Name:      e.Name,
		Size:      e.Size,
		Mime:      e.Mime,
		Extension: e.Extension,
		UpdatedAt: e.UpdatedAt,
	}
}

// RemoteFolderFromPath creates a remote.Folder from path and ID.
func RemoteFolderFromPath(path, id string) remote.Folder {
	name := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		name = path[i+1:]
	}
	return remote.Folder{ID: id, Path: path, Name: name}
}
