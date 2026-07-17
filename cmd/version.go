package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/update"
)

// Version is set at build time via ldflags.
var Version = "0.4.2"

func newVersionCommand() *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Alloy version",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if !check {
				fmt.Fprintf(out, "alloy %s\n", Version)
				return nil
			}

			fmt.Fprintf(out, "alloy %s\n", Version)

			latest, err := update.CheckLatestVersion()
			if err != nil {
				fmt.Fprintf(out, "Update check failed: %s\n", err)
				_ = update.SaveCache(update.CacheResult{})
				return nil
			}

			_ = update.SaveCache(update.CacheResult{
				LatestVersion: latest,
				CheckedAt:     time.Now(),
			})

			if update.IsNewer(Version, latest) {
				fmt.Fprintf(out, "Update available: %s → %s\n", Version, latest)
				fmt.Fprintf(out, "Upgrade: %s\n", update.UpgradeURL)
			} else {
				fmt.Fprintf(out, "alloy %s (up to date)\n", Version)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "Check for newer version")
	return cmd
}
