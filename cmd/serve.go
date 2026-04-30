package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/server"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Build and serve the production site",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			if rootPath, _ := cmd.Flags().GetString("root"); rootPath != "" && !cmd.Flags().Changed("config") {
				configPath = filepath.Join(rootPath, configPath)
			}

			configLoaded := true
			cfg, err := config.LoadWithDefaults(configPath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					cfg = &config.Config{}
					config.ApplyDefaults(cfg)
					configLoaded = false
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
			if cmd.Flags().Changed("refetch") {
				v, _ := cmd.Flags().GetBool("refetch")
				flags["refetch"] = v
			}
			if cmd.Flags().Changed("root") {
				v, _ := cmd.Flags().GetString("root")
				flags["root"] = v
			}
			if len(flags) > 0 {
				config.MergeFlags(cfg, flags)
			}

			if configLoaded {
				if err := config.Validate(cfg); err != nil {
					return err
				}
			}

			// Production server always excludes drafts
			cfg.IncludeDrafts = false

			// Set up progress reporter for build
			if !cfg.Quiet {
				if cfg.Verbose {
					pipeline.SetReporter(pipeline.NewVerboseProgress(cmd.OutOrStdout()))
				} else if isTTY() {
					pipeline.SetReporter(pipeline.NewTTYProgress(cmd.OutOrStdout(), termWidth()))
				}
			}
			defer pipeline.SetReporter(nil)

			// Run the full build pipeline (same as alloy build)
			if _, err := pipeline.Build(cfg); err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			srv := server.NewWithMode(cfg, server.ModePreview)

			portStr, _ := cmd.Flags().GetString("port")
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid port %q: %w", portStr, err)
			}

			actualPort, err := srv.StartWithPortFallback(port, 10)
			if err != nil {
				return err
			}

			if !cfg.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Serving at http://localhost:%d\n", actualPort)
			}

			// Set up file watcher for live rebuild
			var watcher *fsnotify.Watcher
			watcher = startWatcher(cfg, srv, func(events []server.ChangeEvent, _ server.RebuildScope) {
				if !cfg.Quiet {
					log.Printf("rebuilding (%d files changed)...", len(events))
				}

				needsRebuild := false
				for _, ev := range events {
					switch ev.ChangeType {
					case server.ContentChange, server.LayoutChange, server.DataChange, server.ComponentChange:
						needsRebuild = true
					case server.AssetChange, server.StaticChange:
						copyChangedFileToOutput(ev.Path, cfg)
					case server.PassthroughChange:
						if dest, err := server.RecopyPassthroughFile(ev.Path, cfg); err == nil {
							srcPath := ev.Path
							if cfg.ProjectRoot != "" {
								srcPath = filepath.Join(cfg.ProjectRoot, ev.Path)
								dest = filepath.Join(cfg.ProjectRoot, dest)
							}
							copyFileToPath(srcPath, dest, cfg)
						}
					}
				}

				if needsRebuild {
					if _, err := pipeline.Build(cfg); err != nil {
						log.Printf("rebuild failed: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "rebuild"},
						})
					} else {
						srv.Overlay().ClearErrors()
						if !cfg.Quiet {
							log.Printf("rebuild complete")
						}
					}
				}

				srv.BroadcastReload()
			})
			if watcher != nil {
				defer watcher.Close()
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			if !cfg.Quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "\nShutting down...")
			}
			return srv.Stop()
		},
	}

	cmd.Flags().StringP("port", "p", "3000", "Port to serve on")
	cmd.Flags().Bool("refetch", false, "Bypass fetch cache")

	return cmd
}

func copyChangedFileToOutput(relPath string, cfg *config.Config) {
	outputDir := cfg.Build.Output
	if outputDir == "" {
		outputDir = "_site"
	}

	changeType := server.ClassifyChange(relPath, cfg)
	var sourceDir string
	switch changeType {
	case server.StaticChange:
		sourceDir = cfg.Structure.Static
		if sourceDir == "" {
			sourceDir = "static"
		}
	case server.AssetChange:
		sourceDir = cfg.Structure.Assets
		if sourceDir == "" {
			sourceDir = "assets"
		}
	default:
		return
	}

	destRel, err := filepath.Rel(sourceDir, relPath)
	if err != nil {
		log.Printf("warning: computing relative path for %s: %v", relPath, err)
		return
	}

	srcPath := relPath
	if cfg.ProjectRoot != "" {
		srcPath = filepath.Join(cfg.ProjectRoot, relPath)
	}
	destPath := filepath.Join(outputDir, destRel)
	if cfg.ProjectRoot != "" {
		destPath = filepath.Join(cfg.ProjectRoot, destPath)
	}
	copyFileToPath(srcPath, destPath, cfg)
}

func copyFileToPath(src, dest string, cfg *config.Config) {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		log.Printf("warning: creating directory for %s: %v", dest, err)
		return
	}
	srcFile, err := os.Open(src)
	if err != nil {
		log.Printf("warning: opening %s: %v", src, err)
		return
	}
	defer srcFile.Close()
	destFile, err := os.Create(dest)
	if err != nil {
		log.Printf("warning: creating %s: %v", dest, err)
		return
	}
	defer destFile.Close()
	if _, err := io.Copy(destFile, srcFile); err != nil {
		log.Printf("warning: copying %s to %s: %v", src, dest, err)
	}
}
