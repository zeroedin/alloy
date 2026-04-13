package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Create default alloy.config.yaml",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			err := RunInit(dir)
			if err != nil && strings.Contains(err.Error(), "already exists") {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return nil
			}
			return err
		},
	}
}

// RunInit creates an alloy.config.yaml in the given directory.
// Returns an error if the config file already exists.
func RunInit(dir string) error {
	configPath := filepath.Join(dir, "alloy.config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	defaultConfig := `title: My Alloy Site
`
	return os.WriteFile(configPath, []byte(defaultConfig), 0644)
}
