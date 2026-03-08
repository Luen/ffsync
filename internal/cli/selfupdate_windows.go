//go:build windows

package cli

import "os/exec"

func runDetachedWindows(batchPath string) error {
	// start /b runs the batch in a background process that is NOT our child, so it survives when we exit.
	// start /b cmd /c batchPath => our child is "cmd /c start /b ..."; start launches another cmd (running batch) which is not our child.
	cmd := exec.Command("cmd", "/c", "start", "/b", "cmd", "/c", batchPath)
	return cmd.Start()
}
