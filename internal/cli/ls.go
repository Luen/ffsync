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

// LsCmd returns the ls command.
func LsCmd() *cobra.Command {
	var recurse bool
	c := &cobra.Command{
		Use:   "ls [remote:path]",
		Short: "List remote path (default remote:)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := "remote:"
			if len(args) > 0 {
				spec = args[0]
			}
			name, path, err := remote.ParseRemote(spec)
			if err != nil {
				return err
			}
			if name != "remote" {
				return fmt.Errorf("only remote is supported, got %q", name)
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Email == "" || cfg.Password == "" {
				fmt.Fprintln(os.Stderr, "Not configured. Run: ffsync config init && ffsync config set email <e> password <p>")
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

			var folderID string
			if path == "" {
				folderID = ""
			} else {
				folderID, err = cl.EnsureFolderPath(context.Background(), "", path)
				if err != nil {
					return err
				}
			}

			entries, err := cl.List(context.Background(), folderID)
			if err != nil {
				return err
			}
			for _, e := range entries {
				prefix := " "
				if e.Type == "folder" {
					prefix = "d"
				}
				fmt.Printf("%s %s\n", prefix, e.Name)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&recurse, "recurse", "R", false, "List recursively (future)")
	return c
}
