package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ffsync/ffsync/internal/client"
	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/plan"
	"github.com/ffsync/ffsync/internal/remote"
)

// ExecutorOptions configures the sync executor.
type ExecutorOptions struct {
	DryRun         bool
	Transfers      int
	StatePath      string
	ProgressWriter io.Writer       // progress output (e.g. os.Stderr); nil = no progress
	StatsInterval  time.Duration   // how often to print one-line stats (e.g. 1s); 0 = no stats line
	ProgressFiles  bool            // if true, print per-file progress (rsync --progress style)
	DeleteToTrash  bool            // if true, move removed items to trash instead of permanent delete (only with --delete)
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
	uploadTotal := len(p.Upload) + len(p.Update)
	deleteTotal := len(p.DeleteFiles) + len(p.DeleteFolders)
	var totalUploadBytes int64
	for _, f := range p.Upload {
		totalUploadBytes += f.Size
	}
	for _, u := range p.Update {
		totalUploadBytes += u.Local.Size
	}

	var uploadDone atomic.Int32
	var uploadBytes atomic.Int64
	startTime := time.Now()

	// outputMu serializes stats line and per-file messages so they don't collide.
	var outputMu sync.Mutex
	var lastStatsLine string

	clearStats := func() {
		if lastStatsLine != "" {
			fmt.Fprint(opts.ProgressWriter, "\r"+strings.Repeat(" ", len(lastStatsLine)+2)+"\r")
			lastStatsLine = ""
		}
	}
	printStats := func() {
		done := uploadDone.Load()
		bytes := uploadBytes.Load()
		elapsed := time.Since(startTime).Seconds()

		pct := float64(0)
		if totalUploadBytes > 0 {
			pct = 100 * float64(bytes) / float64(totalUploadBytes)
		}
		byteRate := float64(0)
		if elapsed > 0 {
			byteRate = float64(bytes) / elapsed
		}

		// Hybrid ETA: blend file-count rate and byte rate.
		eta := "-"
		fileRate := float64(0)
		if elapsed > 0 {
			fileRate = float64(done) / elapsed
		}
		if fileRate > 0 && int(done) < uploadTotal {
			fileETA := float64(uploadTotal-int(done)) / fileRate
			if byteRate > 0 && totalUploadBytes > bytes {
				byteETA := float64(totalUploadBytes-bytes) / byteRate
				// Weight: 70% file-rate ETA, 30% byte-rate ETA for small-file workloads.
				blended := 0.7*fileETA + 0.3*byteETA
				eta = formatDuration(blended)
			} else {
				eta = formatDuration(fileETA)
			}
		}

		filesPerSec := ""
		if fileRate >= 1 {
			filesPerSec = fmt.Sprintf("%.0f files/s", fileRate)
		} else if fileRate > 0 {
			filesPerSec = fmt.Sprintf("%.1f files/s", fileRate)
		}
		rateStr := formatBytes(int64(byteRate)) + "/s"
		if filesPerSec != "" {
			rateStr = filesPerSec + "  " + rateStr
		}

		lastStatsLine = fmt.Sprintf("%s  %d/%d files  %s  %.1f%%  %s  elapsed %s  ETA %s",
			formatBytes(bytes), done, uploadTotal, formatBytes(totalUploadBytes), pct, rateStr, formatDuration(elapsed), eta)
		fmt.Fprint(opts.ProgressWriter, "\r"+lastStatsLine+"   ")
	}

	onUploadDone := func(path string, bytes int64) {
		uploadDone.Add(1)
		uploadBytes.Add(bytes)
		if opts.ProgressWriter != nil && opts.ProgressFiles {
			n := uploadDone.Load()
			outputMu.Lock()
			clearStats()
			fmt.Fprintf(opts.ProgressWriter, "Uploaded %d/%d: %s\n", n, uploadTotal, path)
			if opts.StatsInterval > 0 {
				printStats()
			}
			outputMu.Unlock()
		}
	}
	var onFolderCreated func(string)
	if opts.ProgressWriter != nil && opts.ProgressFiles {
		onFolderCreated = func(path string) {
			outputMu.Lock()
			clearStats()
			fmt.Fprintf(opts.ProgressWriter, "Created folder: %s\n", path)
			if opts.StatsInterval > 0 {
				printStats()
			}
			outputMu.Unlock()
		}
	}

	// One-line stats (rclone/rsync --info=progress2 style)
	statsQuit := make(chan struct{})
	if opts.ProgressWriter != nil && opts.StatsInterval > 0 && uploadTotal > 0 {
		statsDone := make(chan struct{})
		go func() {
			defer close(statsDone)
			ticker := time.NewTicker(opts.StatsInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-statsQuit:
					return
				case <-ticker.C:
					outputMu.Lock()
					printStats()
					outputMu.Unlock()
				}
			}
		}()
		defer func() { close(statsQuit); <-statsDone }()
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
			runUpload(ctx, cl, localRoot, baseFolderID, f, state, &stateMu, nil, !opts.DeleteToTrash, onFolderCreated, onUploadDone)
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
			runUpload(ctx, cl, localRoot, baseFolderID, u.Local, state, &stateMu, &u.RemoteID, !opts.DeleteToTrash, onFolderCreated, onUploadDone)
		}()
	}
	wg.Wait()

	// Clear the stats line and print a short summary if we showed stats
	if opts.ProgressWriter != nil && opts.StatsInterval > 0 && uploadTotal > 0 {
		fmt.Fprint(opts.ProgressWriter, "\r"+strings.Repeat(" ", 100)+"\r")
		bytes := uploadBytes.Load()
		fmt.Fprintf(opts.ProgressWriter, "Uploaded %d files (%s).\n", uploadTotal, formatBytes(bytes))
	}

	var deleteDone atomic.Int32
	// Collect all IDs and use bulk delete (POST /api/v1/file-entries/delete), same as web UI.
	var allDeleteIDs []string
	for _, obj := range p.DeleteFiles {
		allDeleteIDs = append(allDeleteIDs, obj.ID)
	}
	for _, folder := range p.DeleteFolders {
		allDeleteIDs = append(allDeleteIDs, folder.ID)
	}
	const deleteBatchSize = 50
	for i := 0; i < len(allDeleteIDs); i += deleteBatchSize {
		end := i + deleteBatchSize
		if end > len(allDeleteIDs) {
			end = len(allDeleteIDs)
		}
		batch := allDeleteIDs[i:end]
		deleteForever := !opts.DeleteToTrash
		if err := cl.DeleteFileEntries(ctx, batch, deleteForever); err != nil {
			slog.Error("bulk delete", "batch_size", len(batch), "err", err)
			// Fall back to per-item so we don't lose progress
			for _, id := range batch {
				_ = cl.DeleteFileEntries(ctx, []string{id}, deleteForever)
			}
		}
	}
	for _, obj := range p.DeleteFiles {
		delete(state.Files, obj.Path)
		if opts.ProgressWriter != nil && opts.ProgressFiles {
			n := deleteDone.Add(1)
			fmt.Fprintf(opts.ProgressWriter, "Deleted %d/%d: %s\n", n, deleteTotal, obj.Path)
		} else {
			deleteDone.Add(1)
		}
	}
	for _, folder := range p.DeleteFolders {
		delete(state.Folders, folder.Path)
		if opts.ProgressWriter != nil && opts.ProgressFiles {
			n := deleteDone.Add(1)
			fmt.Fprintf(opts.ProgressWriter, "Deleted %d/%d: %s (folder)\n", n, deleteTotal, folder.Path)
		} else {
			deleteDone.Add(1)
		}
	}
	if deleteTotal > 0 && opts.ProgressWriter != nil && !opts.ProgressFiles {
		fmt.Fprintf(opts.ProgressWriter, "Deleted %d remote items.\n", deleteTotal)
	}
	if opts.StatePath != "" {
		_ = local.SaveState(opts.StatePath, state)
	}
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(sec float64) string {
	if sec < 0 || sec != sec {
		return "-"
	}
	h := int(sec / 3600)
	m := int((sec - float64(h*3600)) / 60)
	s := int(sec - float64(h*3600) - float64(m*60))
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func runUpload(ctx context.Context, cl *client.Client, localRoot, baseFolderID string, f local.File, state *local.State, stateMu *sync.Mutex, deleteAfterID *string, deleteForever bool, onFolderCreated func(string), onDone func(path string, bytes int64)) {
	parentPath := filepath.Dir(f.Path)
	if parentPath == "." {
		parentPath = ""
	}
	parentPath = filepath.ToSlash(parentPath)
	parentID, err := cl.EnsureFolderPath(ctx, baseFolderID, parentPath, onFolderCreated)
	if err != nil {
		slog.Error("ensure folder", "path", parentPath, "err", err)
		return
	}
	fullPath := filepath.Join(localRoot, filepath.FromSlash(f.Path))
	name := filepath.Base(f.Path)
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	if ext == "" && strings.HasPrefix(name, ".") {
		ext = "txt"
	}
	mimeType := mime.TypeByExtension(filepath.Ext(name))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Use in-memory body for small files so we don't hold the file open during PUT and retries are safe.
	const maxBufferSize = 4 << 20 // 4 MiB
	var body io.Reader
	if f.Size >= 0 && f.Size <= maxBufferSize {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Error("read file", "path", fullPath, "err", err)
			return
		}
		body = bytes.NewReader(data)
	} else {
		file, err := os.Open(fullPath)
		if err != nil {
			slog.Error("open file", "path", fullPath, "err", err)
			return
		}
		defer file.Close()
		body = file
	}
	_, err = cl.UploadFile(ctx, parentID, name, mimeType, ext, f.Size, body)
	if err != nil {
		slog.Error("upload", "path", f.Path, "err", err)
		return
	}
	if onDone != nil {
		onDone(f.Path, f.Size)
	}
	if state != nil && stateMu != nil {
		stateMu.Lock()
		state.Files[f.Path] = local.FileState{Size: f.Size, Mtime: f.Mtime, Hash: f.Hash}
		stateMu.Unlock()
	}
	if deleteAfterID != nil && *deleteAfterID != "" {
		_ = cl.DeleteFileEntries(ctx, []string{*deleteAfterID}, deleteForever)
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
		Name:      e.Name.String(),
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
