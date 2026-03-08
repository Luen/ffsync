package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/remote"
	"github.com/spf13/cobra"
)

// StarCmd returns the star command.
func StarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "star remote:path [remote:path ...]",
		Short: "Star remote files or folders",
		Long:  "Resolves each remote path to a file or folder entry and adds it to starred. Paths are relative to the drive root.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runStarUnstar(true),
	}
}

// UnstarCmd returns the unstar command.
func UnstarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unstar remote:path [remote:path ...]",
		Short: "Unstar remote files or folders",
		Long:  "Resolves each remote path to a file or folder entry and removes it from starred. Paths are relative to the drive root.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runStarUnstar(false),
	}
}

func runStarUnstar(star bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
		_, cl, baseFolderID, err := AuthClient(ctx, "remote:", noCookieStore, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(ExitAuth)
		}
		// baseFolderID is "" for drive root; GetEntryID uses it to resolve paths.
		var entryIDs []string
		for _, spec := range args {
			name, path, err := remote.ParseRemote(spec)
			if err != nil {
				return err
			}
			if name != "remote" {
				return fmt.Errorf("only remote is supported, got %q", name)
			}
			if path == "" {
				return fmt.Errorf("path is required for %s", spec)
			}
			id, err := cl.GetEntryID(ctx, baseFolderID, path)
			if err != nil {
				return fmt.Errorf("%s: %w", spec, err)
			}
			entryIDs = append(entryIDs, id)
		}
		if star {
			if err := cl.Star(ctx, entryIDs); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Starred", len(entryIDs), "item(s).")
		} else {
			if err := cl.Unstar(ctx, entryIDs); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Unstarred", len(entryIDs), "item(s).")
		}
		return nil
	}
}
