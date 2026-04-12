package cmd

import "github.com/spf13/cobra"

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default alloy.config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ErrNotImplemented
		},
	}
}

// RunInit creates an alloy.config.yaml in the given directory.
// Returns an error if the config file already exists.
func RunInit(dir string) error {
	return ErrNotImplemented
}
