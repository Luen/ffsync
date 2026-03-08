//go:build !windows

package cli

// runDetachedWindows is only used on Windows; this stub satisfies the linker.
func runDetachedWindows(_ string) error {
	return nil
}
