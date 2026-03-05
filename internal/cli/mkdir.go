package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/client"
	"github.com/ffsync/ffsync/internal/config"
	"github.com/ffsync/ffsync/internal/remote"
	"github.com/spf13/cobra"
)

// MkdirCmd returns the mkdir command.
func MkdirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mkdir remote:path",
		Short: "Create remote directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, path, err := remote.ParseRemote(args[0])
			if err != nil {
				return err
			}
			if name != "remote" {
				return fmt.Errorf("only remote is supported, got %q", name)
			}
			if path == "" {
				return fmt.Errorf("path is required for mkdir")
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Email == "" || cfg.Password == "" {
				fmt.Fprintln(os.Stderr, "Not configured.")
				os.Exit(2)
			}

			cl, err := client.New(cfg.BaseURL)
			if err != nil {
				return err
			}
			if err := cl.Login(context.Background(), cfg.Email, cfg.Password); err != nil {
				fmt.Fprintln(os.Stderr, "Login failed:", err)
				os.Exit(2)
			}

			folderID, err := cl.EnsureFolderPath(context.Background(), "", path)
			if err != nil {
				return err
			}
			fmt.Println("Created path, folder ID:", folderID)
			return nil
		},
	}
}
