package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
)

// NewRootCommand creates a fresh root command tree. Tests use this
// to avoid shared state between test cases.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "alloy",
		Short: "Alloy static site generator",
	}

	// Global persistent flags
	root.PersistentFlags().StringP("config", "c", "alloy.config.yaml", "Path to config file")
	root.PersistentFlags().StringP("output", "o", "_site", "Output directory")
	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
	root.PersistentFlags().StringP("root", "r", "", "Project root directory (overrides config file location)")

	root.AddCommand(newBuildCommand())
	root.AddCommand(newDevCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newInitCommand())
	root.AddCommand(newVersionCommand())
	return root
}

// loadRequiredConfig locates and loads the config file for build/dev/serve.
// When --config is explicitly set, loads that exact path. Otherwise uses
// config.DetectConfigFile to search the project directory for any recognized
// extension (.yaml, .yml, .toml, .json). Returns an actionable error when
// no config file is found.
func loadRequiredConfig(cmd *cobra.Command) (*config.Config, error) {
	if cmd.Flags().Changed("config") {
		configPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadWithDefaults(configPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("config file not found: %s", configPath)
			}
			return nil, fmt.Errorf("loading config: %w", err)
		}
		return cfg, nil
	}

	dir := "."
	if rootPath, _ := cmd.Flags().GetString("root"); rootPath != "" {
		dir = rootPath
	}
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}

	configPath, err := config.DetectConfigFile(dir)
	if err != nil {
		return nil, fmt.Errorf(
			"no alloy.config file found in %s; use --config to specify a config file path", dir)
	}

	cfg, loadErr := config.LoadWithDefaults(configPath)
	if loadErr != nil {
		return nil, fmt.Errorf("loading config: %w", loadErr)
	}
	return cfg, nil
}

var rootCmd = NewRootCommand()

// RootCommand returns the global root command instance.
func RootCommand() *cobra.Command {
	return rootCmd
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
