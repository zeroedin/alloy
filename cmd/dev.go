package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/cache"
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
			configPath := resolveConfigPath(cmd)

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

			// Set up progress reporter for all builds (initial + watcher rebuilds)
			var reporter pipeline.ProgressReporter
			if !cfg.Quiet {
				PrintBanner(cmd.OutOrStdout(), isTTY())
				if cfg.Verbose {
					reporter = pipeline.NewVerboseProgress(cmd.OutOrStdout())
				} else if isTTY() {
					reporter = pipeline.NewTTYProgress(cmd.OutOrStdout(), termWidth())
				}
			}

			initialResult, initialBuildErr := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true, Reporter: reporter})
			if initialBuildErr != nil {
				log.Printf("warning: initial build failed: %v", initialBuildErr)
			}
			var previousCache *cache.Cache
			if initialResult != nil {
				previousCache = initialResult.Cache
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

			// Set up plugin hooks for dev server — uses the same init path as Build()
			// so WASM cache, conflict detection, and path resolution are consistent.
			registry, hooks, pluginWarnings := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			for _, w := range pluginWarnings {
				log.Printf("warning: %s", w)
			}
			for _, w := range registry.ConflictWarnings() {
				log.Printf("warning: %s", w)
			}
			if _, err := hooks.RunWithTimeout(plugin.OnDevServerStart, cfg); err != nil {
				log.Printf("warning: plugin hook onDevServerStart: %v", err)
			}

			// Create cached pipeline state for incremental rebuilds —
			// avoids re-discovering plugins and re-creating the engine on every file change
			ps, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			if psErr != nil {
				log.Printf("warning: pipeline state init: %v", psErr)
			}
			if ps != nil && initialResult != nil && initialResult.SiteData != nil {
				ps.SiteData = initialResult.SiteData
				if ps.Registry != nil {
					for _, rt := range ps.Registry.Runtimes() {
						if err := rt.SetSiteData(ps.SiteData); err != nil {
							log.Printf("warning: updating plugin site data: %v", err)
						}
					}
				}
			}

			// Set up file watcher for live rebuild
			watcher := startWatcher(cfg, srv, func(events []server.ChangeEvent, rebuildScope server.RebuildScope) {
				if _, err := hooks.RunWithTimeout(plugin.OnFileChanged, events); err != nil {
					log.Printf("warning: plugin hook onFileChanged: %v", err)
				}

				needsRebuild := false
				for _, ev := range events {
					if server.RebuildScopeForChangeType(ev.ChangeType) == server.RebuildPipeline {
						needsRebuild = true
						break
					}
				}

				if !needsRebuild {
					srv.BroadcastReload()
					return
				}

				if !cfg.Quiet {
					log.Printf("rebuilding (%d files changed)...", len(events))
				}

				hasComponentChange := false
				for _, ev := range events {
					if ev.ChangeType == server.ComponentChange {
						hasComponentChange = true
						break
					}
				}

				if hasComponentChange || rebuildScope == server.RebuildFull {
					if fullResult, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true, Reporter: reporter}); err != nil {
						log.Printf("rebuild failed: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "rebuild"},
						})
					} else {
						if fullResult != nil && fullResult.Cache != nil {
							previousCache = fullResult.Cache
						}
						if ps != nil && fullResult != nil && fullResult.SiteData != nil {
							ps.SiteData = fullResult.SiteData
							if ps.Registry != nil {
								for _, rt := range ps.Registry.Runtimes() {
									if err := rt.SetSiteData(ps.SiteData); err != nil {
										log.Printf("warning: updating plugin site data: %v", err)
									}
								}
							}
						}
						srv.Overlay().ClearErrors()
						if !cfg.Quiet {
							log.Printf("rebuild complete")
						}
					}
				} else {
					var changedFiles []string
					for _, ev := range events {
						changedFiles = append(changedFiles, ev.Path)
					}
					if incrResult, err := pipeline.BuildIncremental(cfg, nil, previousCache, changedFiles, pipeline.BuildOptions{SkipSSR: true, PipelineState: ps, Reporter: reporter}); err != nil {
						log.Printf("rebuild failed: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "rebuild"},
						})
					} else {
						if incrResult != nil && incrResult.Cache != nil {
							previousCache = incrResult.Cache
						}
						srv.Overlay().ClearErrors()
						if !cfg.Quiet {
							log.Printf("rebuild complete (incremental)")
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
	cmd.Flags().Bool("no-drafts", false, "Exclude draft content")
	cmd.Flags().Bool("refetch", false, "Bypass fetch cache")

	return cmd
}
