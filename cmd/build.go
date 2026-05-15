package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"golang.org/x/term"
)

// isTTY returns true if stdout is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// termWidth returns the terminal width, defaulting to 80.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func newBuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Run the build pipeline and write _site/",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := resolveConfigPath(cmd)

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

			profile, _ := cmd.Flags().GetBool("profile")

			// Set up progress reporter based on flags + TTY detection
			var reporter pipeline.ProgressReporter
			if !cfg.Quiet {
				if cfg.Verbose {
					reporter = pipeline.NewVerboseProgress(cmd.OutOrStdout())
				} else if isTTY() {
					reporter = pipeline.NewTTYProgress(cmd.OutOrStdout(), termWidth())
				}
			}

			var profiler *pipeline.Profiler
			if profile {
				profileDir, _ := cmd.Flags().GetString("profile-dir")
				if !filepath.IsAbs(profileDir) && cfg.ProjectRoot != "" {
					profileDir = filepath.Join(cfg.ProjectRoot, profileDir)
				}
				var err error
				profiler, err = pipeline.StartProfiling(profileDir)
				if err != nil {
					return err
				}
			}

			buildOpts := pipeline.BuildOptions{Profile: profile, Reporter: reporter}
			result, err := pipeline.Build(cfg, buildOpts)
			if err != nil {
				if profiler != nil {
					profiler.StopProfiling()
				}
				return err
			}

			if profiler != nil {
				if err := profiler.StopProfiling(); err != nil {
					return err
				}
			}

			// Non-TTY without --verbose: print summary line for CI/piped output
			if !cfg.Quiet && !cfg.Verbose && !isTTY() {
				fmt.Fprintf(cmd.OutOrStdout(), "[alloy] Built %d pages in %s\n",
					result.PageCount, result.Duration.Round(time.Millisecond))
			}

			// Print stage timing table when profiling
			if profiler != nil && len(result.StageTimings) > 0 {
				pipeline.PrintStageTimings(cmd.OutOrStdout(), result.StageTimings)
				fmt.Fprintf(cmd.OutOrStdout(), "[alloy] Wrote profiles to %s\n", profiler.Dir())
			}

			return nil
		},
	}
	cmd.Flags().Bool("profile", false, "Enable pprof profiling and print per-stage timing breakdown")
	cmd.Flags().String("profile-dir", ".alloy/profiles", "Directory for profile output files")
	return cmd
}
