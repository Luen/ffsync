package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// EmptyTrashCmd returns the empty-trash command.
func EmptyTrashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "empty-trash",
		Short: "Permanently delete all items in FolderFort trash",
		Long:  "Empties the trash on FolderFort (permanently deletes all trashed files and folders). Requires login.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
			_, cl, _, err := AuthClient(ctx, "remote:", noCookieStore, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			if err := cl.EmptyTrash(ctx); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Trash emptied.")
			return nil
		},
	}
}
