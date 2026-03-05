//go:build windows

package sync

func pidRunning(pid int) bool {
	// On Windows we cannot easily check if PID exists without cgo or windows API.
	// Treat as running so we don't remove a valid lock.
	if pid <= 0 {
		return false
	}
	return true
}
