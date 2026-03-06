package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/plan"
	"github.com/ffsync/ffsync/internal/remote"
	"github.com/ffsync/ffsync/internal/sync"
	"github.com/spf13/cobra"
)

// SyncCmd returns the sync command.
func SyncCmd() *cobra.Command {
	var deleteFlag, dryRun, force, progressFiles, trash bool
	var checkers, transfers, maxDelete int
	var include, exclude []string
	var statsInterval string
	c := &cobra.Command{
		Use:   "sync <local> remote:path",
		Short: "Sync local directory to remote (one-way mirror)",
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
			showProgress := progressFiles || (cmd.Flags().Changed("stats") && statsInterval != "" && statsInterval != "0")
			var progressWriter io.Writer
			if showProgress {
				progressWriter = os.Stderr
			}
			cfg, cl, baseFolderID, err := AuthClient(ctx, remoteSpec, noCookieStore, showProgress)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			_ = cfg
			localFiles, err := local.Scan(localRoot, include, exclude)
			if err != nil {
				return err
			}
			remoteFiles, remoteFolders, err := BuildRemoteMaps(ctx, cl, baseFolderID, progressWriter)
			if err != nil {
				return err
			}
			copyOnly := !deleteFlag
			p := plan.Compute(localFiles, remoteFiles, remoteFolders, copyOnly)
			totalDeletes := len(p.DeleteFiles) + len(p.DeleteFolders)
			if totalDeletes > maxDelete && !force {
				return fmt.Errorf("planned deletes (%d) exceed --max-delete (%d); use --force to override", totalDeletes, maxDelete)
			}
			lockPath := LockPath(localRoot)
			unlock, err := sync.Lock(lockPath)
			if err != nil {
				return err
			}
			defer sync.Unlock(unlock)
			// Clean up lock on Ctrl+C / interrupt so the next run doesn't see a stale lock.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				sync.Unlock(unlock)
				os.Exit(130)
			}()
			defer signal.Stop(sigCh)
			statePath := StatePath(localRoot)
			statsDur := time.Duration(0)
			if statsInterval != "" && statsInterval != "0" {
				d, err := time.ParseDuration(statsInterval)
				if err != nil {
					return fmt.Errorf("invalid --stats duration %q: %w", statsInterval, err)
				}
				statsDur = d
			}
			var syncProgressWriter io.Writer
			if showProgress {
				syncProgressWriter = os.Stderr
			}
			opts := sync.ExecutorOptions{
				DryRun:         dryRun,
				Transfers:      transfers,
				StatePath:      statePath,
				ProgressWriter: syncProgressWriter,
				StatsInterval:  statsDur,
				ProgressFiles:  progressFiles,
				DeleteToTrash:  trash,
			}
			return sync.Execute(ctx, p, cl, localRoot, baseFolderID, statePath, opts)
		},
	}
	c.Flags().BoolVar(&deleteFlag, "delete", false, "Delete remote files not in local")
	c.Flags().BoolVar(&trash, "trash", false, "With --delete: move removed items to trash instead of permanent delete")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Only log planned actions")
	c.Flags().BoolVar(&force, "force", false, "Allow deletes beyond --max-delete")
	c.Flags().BoolVar(&progressFiles, "progress", false, "Show per-file progress (rsync-style; can be noisy with many transfers)")
	c.Flags().StringVar(&statsInterval, "stats", "1s", "Progress stats update interval (e.g. 1s, 500ms). Use 0 to disable the one-line stats")
	if f := c.Flags().Lookup("stats"); f != nil {
		f.NoOptDefVal = "1s" // --stats with no value means use default 1s
	}
	c.Flags().IntVar(&checkers, "checkers", 4, "Number of hash checkers")
	c.Flags().IntVar(&transfers, "transfers", 4, "Number of transfer workers")
	c.Flags().IntVar(&maxDelete, "max-delete", 10, "Abort if planned deletes exceed this (0 = no deletes unless --force)")
	c.Flags().StringSliceVar(&exclude, "exclude", nil, "Exclude patterns (glob)")
	c.Flags().StringSliceVar(&include, "include", nil, "Include patterns (glob)")
	return c
}
