package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the dev server",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadWithDefaults(configPath)
			if err != nil {
				if os.IsNotExist(err) {
					// No config file — serve with defaults
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

			// Determine server mode
			preview, _ := cmd.Flags().GetBool("preview")
			mode := server.ModeDev
			if preview {
				mode = server.ModePreview
			}

			srv := server.NewWithMode(cfg, mode)

			// Apply --no-drafts flag
			noDrafts, _ := cmd.Flags().GetBool("no-drafts")
			srv.SetNoDrafts(noDrafts)

			// Parse port
			portStr, _ := cmd.Flags().GetString("port")
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid port %q: %w", portStr, err)
			}

			if !cfg.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Serving at http://localhost:%d\n", port)
			}

			return srv.Start(port)
		},
	}

	cmd.Flags().StringP("port", "p", "3000", "Port to serve on")
	cmd.Flags().Bool("preview", false, "Serve production build preview")
	cmd.Flags().Bool("no-drafts", false, "Exclude draft content")
	cmd.Flags().Bool("refetch", false, "Bypass fetch cache")

	return cmd
}
