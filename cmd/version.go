package cmd

import "github.com/spf13/cobra"

// Version is set at build time via ldflags.
var Version = ""

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Alloy version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ErrNotImplemented
		},
	}
}
