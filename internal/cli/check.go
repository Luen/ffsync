package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/remote"
	"github.com/spf13/cobra"
)

// CheckCmd returns the check command.
func CheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check <local> remote:path",
		Short: "Compare local and remote (size/hash)",
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
			_, cl, baseFolderID, err := AuthClient(ctx, remoteSpec, noCookieStore, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			localFiles, err := local.Scan(localRoot, nil, nil)
			if err != nil {
				return err
			}
			remoteFiles, _, err := BuildRemoteMaps(ctx, cl, baseFolderID, nil)
			if err != nil {
				return err
			}
			var missingLocal, missingRemote, sizeMismatch []string
			for path := range localFiles {
				if _, ok := remoteFiles[path]; !ok {
					missingRemote = append(missingRemote, path)
				}
			}
			for path, r := range remoteFiles {
				l, ok := localFiles[path]
				if !ok {
					missingLocal = append(missingLocal, path)
					continue
				}
				if r.Size != l.Size {
					sizeMismatch = append(sizeMismatch, path)
				}
			}
			if len(missingLocal) > 0 {
				fmt.Fprintln(os.Stderr, "Only on remote:", missingLocal)
			}
			if len(missingRemote) > 0 {
				fmt.Fprintln(os.Stderr, "Only on local:", missingRemote)
			}
			if len(sizeMismatch) > 0 {
				fmt.Fprintln(os.Stderr, "Size mismatch:", sizeMismatch)
			}
			if len(missingLocal) == 0 && len(missingRemote) == 0 && len(sizeMismatch) == 0 {
				fmt.Println("OK: local and remote match (by size).")
				return nil
			}
			os.Exit(ExitError)
			return nil
		},
	}
}
