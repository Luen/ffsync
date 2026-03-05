package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionStr = "dev"

// SetVersion sets the version string (from ldflags in build).
func SetVersion(v string) {
	if v != "" {
		versionStr = v
	}
}

// VersionCmd returns the version command.
func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("ffsync", versionStr)
			return nil
		},
	}
}
