package cmd

import (
	"github.com/spf13/cobra"
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

var rootCmd = NewRootCommand()

// RootCommand returns the global root command instance.
func RootCommand() *cobra.Command {
	return rootCmd
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
