package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

func newBuildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Run the build pipeline and write _site/",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadWithDefaults(configPath)
			if err != nil {
				if os.IsNotExist(err) {
					// No config file — build with defaults
					cfg = &config.Config{}
					config.ApplyDefaults(cfg)
				} else {
					return fmt.Errorf("loading config: %w", err)
				}
			}

			// Apply CLI flag overrides
			flags := make(map[string]interface{})
			if cmd.Flags().Changed("output") {
				v, _ := cmd.Flags().GetString("output")
				flags["output"] = v
			}
			if cmd.Flags().Changed("verbose") {
				v, _ := cmd.Flags().GetBool("verbose")
				flags["verbose"] = v
			}
			if cmd.Flags().Changed("quiet") {
				v, _ := cmd.Flags().GetBool("quiet")
				flags["quiet"] = v
			}
			if len(flags) > 0 {
				config.MergeFlags(cfg, flags)
			}

			result, err := pipeline.Build(cfg)
			if err != nil {
				return err
			}

			if !cfg.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Build complete: %d pages in %s\n",
					result.PageCount, result.Duration.Round(time.Millisecond))
			}

			return nil
		},
	}
}
