package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SpaceCmd returns the space command.
func SpaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "space",
		Short: "Show FolderFort storage usage",
		Long:  "Shows used and available storage for your FolderFort account. Requires login.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			noCookieStore, _ := cmd.Root().Flags().GetBool("no-cookie-store")
			_, cl, _, err := AuthClient(ctx, "remote:", noCookieStore, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(ExitAuth)
			}
			usage, err := cl.SpaceUsage(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("Used:      %s\n", formatBytes(usage.Used))
			fmt.Printf("Available: %s\n", formatBytes(usage.Available))
			fmt.Printf("Total:     %s\n", formatBytes(usage.Used+usage.Available))
			return nil
		},
	}
}
