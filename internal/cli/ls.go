package cli

import (
	"context"
	"fmt"
	"os"

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
			name, _, err := remote.ParseRemote(spec)
			if err != nil {
				return err
			}
			if name != "remote" {
				return fmt.Errorf("only remote is supported, got %q", name)
			}

			ctx := context.Background()
			noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
			_, cl, baseFolderID, err := AuthClient(ctx, spec, noCookieStore, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}

			entries, err := cl.List(ctx, baseFolderID)
			if err != nil {
				return err
			}
			for _, e := range entries {
				prefix := " "
				if e.Type == "folder" {
					prefix = "d"
				}
				fmt.Printf("%s %s\n", prefix, e.Name.String())
			}
			return nil
		},
	}
	_ = recurse
	c.Flags().BoolVarP(&recurse, "recurse", "R", false, "List recursively (future)")
	return c
}
