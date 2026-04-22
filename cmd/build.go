package cmd

import (
	"errors"
	"fmt"
	"io/fs"

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

			configLoaded := true
			cfg, err := config.LoadWithDefaults(configPath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					// No config file — build with defaults
					cfg = &config.Config{}
					config.ApplyDefaults(cfg)
					configLoaded = false
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
			if cmd.Flags().Changed("root") {
				v, _ := cmd.Flags().GetString("root")
				flags["root"] = v
			}
			if len(flags) > 0 {
				config.MergeFlags(cfg, flags)
			}

			// Validate config semantics when a config file was loaded
			if configLoaded {
				if err := config.Validate(cfg); err != nil {
					return err
				}
			}

			// Set up progress reporter based on flags
			if !cfg.Quiet {
				if cfg.Verbose {
					pipeline.SetReporter(pipeline.NewVerboseProgress(cmd.OutOrStdout()))
				} else {
					pipeline.SetReporter(pipeline.NewTTYProgress(cmd.OutOrStdout(), 80))
				}
			}
			defer pipeline.SetReporter(nil)

			_, err = pipeline.Build(cfg)
			if err != nil {
				return err
			}

			return nil
		},
	}
}
