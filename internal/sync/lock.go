package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Lock acquires the lock file at path; returns unlock function or error.
// Uses a simple PID file: create exclusive, write PID; unlock removes file.
func Lock(path string) (unlock func(), err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	// Try create exclusive (O_CREATE|O_EXCL); if exists, read PID and check if still running.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
		// Lock exists: read PID
		b, _ := os.ReadFile(path)
		pidStr := strings.TrimSpace(string(b))
		if pidStr != "" {
			pid, _ := strconv.Atoi(pidStr)
			if pidRunning(pid) {
				return nil, fmt.Errorf("another ffsync is running (PID %d); lock file: %s", pid, path)
			}
		}
		// Stale lock, remove and retry
		os.Remove(path)
		return Lock(path)
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Sync()
	_ = f.Close()
	return func() {
		os.Remove(path)
	}, nil
}

// Unlock releases the lock by calling the returned unlock function.
func Unlock(unlock func()) {
	if unlock != nil {
		unlock()
	}
}

// pidRunning is implemented in lock_unix.go and lock_windows.go.
