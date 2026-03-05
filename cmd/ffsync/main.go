package main

import (
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "ffsync",
		Short: "Sync local directory to FolderFort",
	}
	root.PersistentFlags().String("config", "", "Config file (default: FOLDERFORT_CONFIG or ~/.config/ffsync/ffsync.conf)")
	root.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	_ = root.PersistentFlags().MarkHidden("config")

	root.AddCommand(cli.VersionCmd())
	root.AddCommand(cli.ConfigCmd())
	root.AddCommand(cli.LsCmd())
	root.AddCommand(cli.MkdirCmd())
	root.AddCommand(cli.CopyCmd())
	root.AddCommand(cli.SyncCmd())
	root.AddCommand(cli.CheckCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
