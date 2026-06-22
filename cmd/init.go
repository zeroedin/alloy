package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
)

func newInitCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Scaffold a new Alloy project",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInitE,
	}
	initCmd.Flags().String("content", "content", "Content directory name")
	initCmd.Flags().String("layouts", "layouts", "Layouts directory name")
	initCmd.Flags().String("assets", "assets", "Assets directory name")
	initCmd.Flags().String("static", "static", "Static files directory name")
	initCmd.Flags().String("data", "data", "Data directory name")
	return initCmd
}

func runInitE(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if _, err := config.DetectConfigFile(dir); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "alloy project already exists in %s\n", dir)
		return nil
	}

	contentDir, _ := cmd.Flags().GetString("content")
	layoutsDir, _ := cmd.Flags().GetString("layouts")
	assetsDir, _ := cmd.Flags().GetString("assets")
	staticDir, _ := cmd.Flags().GetString("static")
	dataDir, _ := cmd.Flags().GetString("data")

	for _, d := range []string{contentDir, layoutsDir, assetsDir, staticDir, dataDir, "plugins"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	configYAML := "title: \"My Alloy Site\"\nbaseURL: \"http://localhost:3000\"\n"

	type entry struct{ key, val string }
	var custom []entry
	for _, f := range []struct{ flag, key string }{
		{"content", "content"},
		{"layouts", "layouts"},
		{"assets", "assets"},
		{"static", "static"},
		{"data", "data"},
	} {
		if cmd.Flags().Changed(f.flag) {
			v, _ := cmd.Flags().GetString(f.flag)
			custom = append(custom, entry{f.key, v})
		}
	}
	if len(custom) > 0 {
		configYAML += "structure:\n"
		for _, e := range custom {
			configYAML += fmt.Sprintf("  %s: %q\n", e.key, e.val)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "alloy.config.yaml"), []byte(configYAML), 0644); err != nil {
		return err
	}

	defaultLayout := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ page.title }}</title>
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  {{ content }}
</body>
</html>
`
	if err := os.WriteFile(filepath.Join(dir, layoutsDir, "default.liquid"), []byte(defaultLayout), 0644); err != nil {
		return err
	}

	indexMd := `---
title: Welcome to Alloy
layout: default
---

# Welcome to Alloy

Your new site is ready.
`
	if err := os.WriteFile(filepath.Join(dir, contentDir, "index.md"), []byte(indexMd), 0644); err != nil {
		return err
	}

	styleCss := `*, *::before, *::after {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: system-ui, -apple-system, sans-serif;
  line-height: 1.6;
  max-width: 48rem;
  margin: 0 auto;
  padding: 2rem 1rem;
  color: #1a1a1a;
}
`
	if err := os.WriteFile(filepath.Join(dir, staticDir, "style.css"), []byte(styleCss), 0644); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created new Alloy project in %s\n", dir)
	return nil
}
