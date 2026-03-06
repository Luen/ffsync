package cli

import (
	"context"
	"fmt"
	"os"

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

			ctx := context.Background()
			noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
			_, _, baseFolderID, err := AuthClient(ctx, "remote:"+path, noCookieStore)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			fmt.Println("Created path, folder ID:", baseFolderID)
			return nil
		},
	}
}
