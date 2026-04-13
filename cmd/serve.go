package cmd

import "github.com/spf13/cobra"

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the dev server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.Flags().StringP("port", "p", "3000", "Port to serve on")
	cmd.Flags().Bool("preview", false, "Serve production build preview")
	cmd.Flags().Bool("no-drafts", false, "Exclude draft content")
	cmd.Flags().Bool("refetch", false, "Bypass fetch cache")

	return cmd
}
