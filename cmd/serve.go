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
	"time"

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
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Printf("warning: file watcher unavailable: %v", err)
			} else {
				defer watcher.Close()

				watchDirs := server.WatchDirs(cfg)
				for _, dir := range watchDirs {
					absDir := dir
					if cfg.ProjectRoot != "" {
						absDir = filepath.Join(cfg.ProjectRoot, dir)
					}
					if info, err := os.Stat(absDir); err == nil && info.IsDir() {
						if err := addRecursiveWatch(watcher, absDir); err != nil {
							log.Printf("warning: watching %s: %v", dir, err)
						}
					}
				}

				debouncer := server.NewDebouncer(
					time.Duration(srv.DebounceInterval())*time.Millisecond,
					10,
				)

				go func() {
					var pending []server.ChangeEvent
					timer := time.NewTimer(time.Duration(srv.DebounceInterval()) * time.Millisecond)
					timer.Stop()

					for {
						select {
						case event, ok := <-watcher.Events:
							if !ok {
								return
							}
							if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
								continue
							}

							relPath := event.Name
							if cfg.ProjectRoot != "" {
								if r, err := filepath.Rel(cfg.ProjectRoot, event.Name); err == nil {
									relPath = r
								}
							}

							changeType := server.ClassifyChange(relPath, cfg)
							pending = append(pending, server.ChangeEvent{
								Path:       relPath,
								ChangeType: changeType,
							})

							if event.Op&fsnotify.Create != 0 {
								if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
									addRecursiveWatch(watcher, event.Name)
								}
							}

							timer.Reset(time.Duration(srv.DebounceInterval()) * time.Millisecond)

						case <-timer.C:
							if len(pending) == 0 {
								continue
							}

							events := pending
							pending = nil

							events, _ = debouncer.Debounce(events)

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

						case err, ok := <-watcher.Errors:
							if !ok {
								return
							}
							log.Printf("watcher error: %v", err)
						}
					}
				}()
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
