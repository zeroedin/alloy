package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// NewRootCommand creates a fresh root command tree. Tests use this
// to avoid shared state between test cases.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "alloy",
		Short: "Alloy static site generator",
	}
	root.AddCommand(newBuildCommand())
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
