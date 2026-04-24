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

func newDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start the development server",
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

			noDrafts, _ := cmd.Flags().GetBool("no-drafts")
			cfg.IncludeDrafts = !noDrafts

			// Set up progress reporter for initial build
			if !cfg.Quiet {
				if cfg.Verbose {
					pipeline.SetReporter(pipeline.NewVerboseProgress(cmd.OutOrStdout()))
				} else if isTTY() {
					pipeline.SetReporter(pipeline.NewTTYProgress(cmd.OutOrStdout(), termWidth()))
				}
			}
			defer pipeline.SetReporter(nil)

			_, initialBuildErr := pipeline.Build(cfg)
			if initialBuildErr != nil {
				log.Printf("warning: initial build failed: %v", initialBuildErr)
			}

			srv := server.NewWithMode(cfg, server.ModeDev)
			srv.SetNoDrafts(noDrafts)

			if initialBuildErr != nil {
				srv.Overlay().SetErrors([]server.BuildError{
					{Message: initialBuildErr.Error(), Stage: "initial build"},
				})
			}

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

			// Set up plugin hooks for dev server
			hooks := plugin.NewHookRegistry()
			hooks.SetTimeout(cfg.Plugins.Timeout)
			pluginsDir := "plugins"
			if cfg.ProjectRoot != "" {
				pluginsDir = filepath.Join(cfg.ProjectRoot, "plugins")
			}
			registry := plugin.NewRegistry(pluginsDir)
			if err := registry.DiscoverPlugins(); err != nil {
				log.Printf("warning: plugin discovery: %v", err)
			}
			for _, w := range registry.LoadPlugins(hooks) {
				log.Printf("warning: %s", w)
			}
			if _, err := hooks.RunWithTimeout(plugin.OnDevServerStart, cfg); err != nil {
				log.Printf("warning: plugin hook onDevServerStart: %v", err)
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

							debouncer.Debounce(events)

							if _, err := hooks.RunWithTimeout(plugin.OnFileChanged, events); err != nil {
								log.Printf("warning: plugin hook onFileChanged: %v", err)
							}

							if !cfg.Quiet {
								log.Printf("rebuilding (%d files changed)...", len(events))
							}

							if _, err := pipeline.Build(cfg); err != nil {
								log.Printf("rebuild failed: %v", err)
								srv.Overlay().SetErrors([]server.BuildError{
									{Message: err.Error(), Stage: "rebuild"},
								})
								srv.BroadcastReload()
							} else {
								srv.Overlay().ClearErrors()
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
