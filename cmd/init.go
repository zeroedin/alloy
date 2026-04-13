package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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
			return RunInit(dir)
		},
	}
}

// RunInit creates an alloy.config.yaml in the given directory.
// Returns an error if the config file already exists.
func RunInit(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	configPath := filepath.Join(dir, "alloy.config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	defaultConfig := `title: "My Alloy Site"
baseURL: "http://localhost:3000"
`
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", configPath)
	return nil
}
