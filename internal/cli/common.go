package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ffsync/ffsync/internal/client"
	"github.com/ffsync/ffsync/internal/config"
	"github.com/ffsync/ffsync/internal/remote"
	syncpkg "github.com/ffsync/ffsync/internal/sync"
)

// Exit codes.
const (
	ExitSuccess = 0
	ExitUsage   = 1
	ExitAuth    = 2
	ExitError   = 3
)

// AuthClient loads config, creates client, ensures session (reusing stored cookies or logging in), and returns base folder ID for the remote path.
// If noCookieStore is true, no cookie file is used (in-memory only for this run).
// If showProgress is true, session-check and login messages are printed to stderr (e.g. when sync uses --progress or --stats).
func AuthClient(ctx context.Context, remoteSpec string, noCookieStore bool, showProgress bool) (cfg *config.Config, cl *client.Client, baseFolderID string, err error) {
	cfg, err = config.Load()
	if err != nil {
		return nil, nil, "", err
	}
	if cfg.Email == "" || cfg.Password == "" {
		return nil, nil, "", fmt.Errorf("not configured: set email and password (ffsync config set email ... password ...)")
	}
	cookiePath := ""
	if !noCookieStore {
		cookiePath = filepath.Join(cfg.StateDir, "cookies.json")
	}
	cl, err = client.New(cfg.BaseURL, cookiePath)
	if err != nil {
		return nil, nil, "", err
	}
	// With persistent store: try reusing session (List root). Otherwise or on error, login.
	if cookiePath != "" {
		if showProgress {
			fmt.Fprintln(os.Stderr, "Checking session...")
		}
		if _, tryErr := cl.List(ctx, ""); tryErr == nil {
			// session valid, skip login
		} else {
			if showProgress {
				fmt.Fprintln(os.Stderr, "Session not found, invalid, or expired. Logging in.")
			}
			if err := cl.Login(ctx, cfg.Email, cfg.Password); err != nil {
				slog.Error("login failed", "err", err)
				return nil, nil, "", err
			}
		}
	} else {
		if showProgress {
			fmt.Fprintln(os.Stderr, "Logging in.")
		}
		if err := cl.Login(ctx, cfg.Email, cfg.Password); err != nil {
			slog.Error("login failed", "err", err)
			return nil, nil, "", err
		}
	}
	_, remotePath, err := remote.ParseRemote(remoteSpec)
	if err != nil {
		return nil, nil, "", err
	}
	baseFolderID, err = cl.EnsureFolderPath(ctx, "", remotePath)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, cl, baseFolderID, nil
}

// LocalAbs returns the absolute local path.
func LocalAbs(localPath string) (string, error) {
	return filepath.Abs(localPath)
}

// StatePath returns the state file path for a sync root (default .ffsync-state in local root).
func StatePath(localRoot string) string {
	return filepath.Join(localRoot, ".ffsync-state")
}

// LockPath returns the lock file path.
func LockPath(localRoot string) string {
	return filepath.Join(localRoot, ".ffsync.lock")
}

func exitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	return ExitError
}

func exit(err error) {
	if err != nil {
		slog.Error("error", "err", err)
	}
	os.Exit(exitCode(err))
}

// BuildRemoteMaps lists remote recursively and returns files and folders maps.
// If progressWriter is non-nil, a live-updating progress line is written to it (e.g. os.Stderr).
func BuildRemoteMaps(ctx context.Context, cl *client.Client, baseFolderID string, progressWriter io.Writer) (files map[string]remote.Object, folders map[string]remote.Folder, err error) {
	var filesCount, foldersCount atomic.Int64
	var progress client.ListRecursiveProgress
	var done chan struct{}
	var wg sync.WaitGroup
	if progressWriter != nil {
		progress = func(deltaFiles, deltaFolders int) {
			filesCount.Add(int64(deltaFiles))
			foldersCount.Add(int64(deltaFolders))
		}
		done = make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					fmt.Fprintf(progressWriter, "\rListing remote... %d files, %d folders   \n", filesCount.Load(), foldersCount.Load())
					return
				case <-ticker.C:
					fmt.Fprintf(progressWriter, "\rListing remote... %d files, %d folders   ", filesCount.Load(), foldersCount.Load())
				}
			}
		}()
	}
	apiFiles, apiFolders, err := cl.ListRecursive(ctx, baseFolderID, "", progress)
	if done != nil {
		close(done)
		wg.Wait()
	}
	if err != nil {
		return nil, nil, err
	}
	files = make(map[string]remote.Object)
	for path, e := range apiFiles {
		files[path] = syncpkg.RemoteObjectFromEntry(path, e)
	}
	folders = make(map[string]remote.Folder)
	for path, id := range apiFolders {
		folders[path] = syncpkg.RemoteFolderFromPath(path, id)
	}
	return files, folders, nil
}
