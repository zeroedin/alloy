package cmd

import "github.com/spf13/cobra"

func newBuildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Run the build pipeline and write _site/",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ErrNotImplemented
		},
	}
}
