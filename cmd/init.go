package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default alloy.config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
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
