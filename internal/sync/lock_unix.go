//go:build unix || linux || darwin

package sync

import "syscall"

func pidRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err == nil {
		return true
	}
	return false
}
