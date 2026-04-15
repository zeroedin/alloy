package cmd

import (
	"errors"
	"fmt"
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
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/server"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the dev server",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			configLoaded := true
			cfg, err := config.LoadWithDefaults(configPath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					// No config file — serve with defaults
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
			if len(flags) > 0 {
				config.MergeFlags(cfg, flags)
			}

			// Validate config semantics when a config file was loaded
			if configLoaded {
				if err := config.Validate(cfg); err != nil {
					return err
				}
			}

			// Initial build per spec §9 step 2
			if _, err := pipeline.Build(cfg); err != nil {
				log.Printf("warning: initial build failed: %v", err)
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

			if err := srv.Start(port); err != nil {
				return err
			}

			if !cfg.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Serving at http://localhost:%d\n", port)
			}

			// Fire onDevServerStart plugin hook
			hooks := plugin.NewHookRegistry()
			hooks.SetTimeout(cfg.Plugins.Timeout)
			hooks.RunWithTimeout(plugin.OnDevServerStart, cfg)

			// Set up file watcher for live rebuild
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Printf("warning: file watcher unavailable: %v", err)
			} else {
				defer watcher.Close()

				// Watch configured directories
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

				// File watcher goroutine
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

							// Make path relative to project root for classification
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

							// If a new directory was created, add it to the watcher
							if event.Op&fsnotify.Create != 0 {
								if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
									addRecursiveWatch(watcher, event.Name)
								}
							}

							// Reset debounce timer
							timer.Reset(time.Duration(srv.DebounceInterval()) * time.Millisecond)

						case <-timer.C:
							if len(pending) == 0 {
								continue
							}

							events := pending
							pending = nil

							_, scope := debouncer.Debounce(events)

							// Fire onFileChanged plugin hook
							hooks.RunWithTimeout(plugin.OnFileChanged, events)

							if !cfg.Quiet {
								action := "incremental"
								if scope == server.RebuildFull {
									action = "full"
								}
								log.Printf("rebuilding (%s, %d files changed)...", action, len(events))
							}

							// Trigger rebuild
							if _, err := pipeline.Build(cfg); err != nil {
								log.Printf("rebuild failed: %v", err)
							} else {
								if !cfg.Quiet {
									log.Printf("rebuild complete")
								}
								srv.BroadcastReload()
							}

						case err, ok := <-watcher.Errors:
							if !ok {
								return
							}
							log.Printf("watcher error: %v", err)
						}
					}
				}()
			}

			// Wait for interrupt signal
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
	cmd.Flags().Bool("preview", false, "Serve production build preview")
	cmd.Flags().Bool("no-drafts", false, "Exclude draft content")
	cmd.Flags().Bool("refetch", false, "Bypass fetch cache")

	return cmd
}

// addRecursiveWatch adds a directory and all its subdirectories to the watcher.
func addRecursiveWatch(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible directories
		}
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}
