package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/plan"
	"github.com/ffsync/ffsync/internal/remote"
	"github.com/ffsync/ffsync/internal/sync"
	"github.com/spf13/cobra"
)

// CopyCmd returns the copy command.
func CopyCmd() *cobra.Command {
	var include, exclude []string
	c := &cobra.Command{
		Use:   "copy <local> remote:path",
		Short: "Copy local path to remote (no deletes)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localPath := args[0]
			remoteSpec := args[1]
			name, _, err := remote.ParseRemote(remoteSpec)
			if err != nil {
				return err
			}
			if name != "remote" {
				return fmt.Errorf("only remote is supported, got %q", name)
			}
			localRoot, err := LocalAbs(localPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
			cfg, cl, baseFolderID, err := AuthClient(ctx, remoteSpec, noCookieStore)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			_ = cfg
			localFiles, err := local.Scan(localRoot, include, exclude)
			if err != nil {
				return err
			}
			remoteFiles, _, err := BuildRemoteMaps(ctx, cl, baseFolderID)
			if err != nil {
				return err
			}
			p := plan.Compute(localFiles, remoteFiles, nil, true)
			statePath := StatePath(localRoot)
			opts := sync.ExecutorOptions{Transfers: 4, StatePath: statePath}
			return sync.Execute(ctx, p, cl, localRoot, baseFolderID, statePath, opts)
		},
	}
	c.Flags().StringSliceVar(&exclude, "exclude", nil, "Exclude patterns (glob)")
	c.Flags().StringSliceVar(&include, "include", nil, "Include patterns (glob)")
	return c
}
