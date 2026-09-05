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

	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fileutil"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/server"
	"github.com/zeroedin/alloy/internal/validation"
)

func newDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start the development server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRequiredConfig(cmd)
			if err != nil {
				return err
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

			if err := config.Validate(cfg); err != nil {
				return err
			}

			// Check for another running alloy instance (issue #1094)
			if warnings := server.CheckAndWarnLockfile(cfg.ProjectRoot); len(warnings) > 0 {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", w)
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
			// Output path claims from the last full build. The watcher recopies
			// static, asset, and passthrough files itself — BuildIncremental
			// copies none of them — so the claim set has to outlive Build() for
			// a mid-session collision to be caught (issue #1238).
			var outputClaims []validation.OutputPathEntry
			var copyOrigins map[string]string
			if initialResult != nil {
				previousCache = initialResult.Cache
				outputClaims = initialResult.OutputClaims
				copyOrigins = initialResult.CopyOrigins
			}
			if copyOrigins == nil {
				copyOrigins = map[string]string{}
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

			// Write server lockfile after successful start (issue #1094)
			if err := server.WriteLockfile(cfg.ProjectRoot, server.LockfileInfo{
				PID:       os.Getpid(),
				Port:      actualPort,
				Mode:      "dev",
				StartedAt: time.Now().Format(time.RFC3339),
			}); err != nil {
				log.Printf("warning: write server lockfile: %v", err)
			}

			if !cfg.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Serving at http://localhost:%d\n", actualPort)
				if cfg.UpdateCheckValue() {
					maybeNotifyUpdate(cmd.OutOrStdout(), Version)
				}
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
				// Process onFileChanged return value for dependency-based
				// invalidation and Node bridge restart (issue #1100).
				var depInvalidPaths []string
				var needsRestart bool
				hookResult, hookErr := hooks.RunWithTimeout(plugin.OnFileChanged, events)
				if hookErr != nil {
					log.Printf("warning: plugin hook onFileChanged: %v", hookErr)
				} else if parsed := plugin.ParseFileChangedResult(hookResult); parsed != nil {
					for _, w := range parsed.Warnings {
						log.Printf("warning: onFileChanged: %s", w)
					}
					depInvalidPaths = parsed.InvalidateByDependency
					needsRestart = parsed.Restart
				}

				// Recopy static/asset/passthrough files before any rebuild decision.
				// Runs unconditionally so mixed batches (content + static) don't
				// lose static changes — BuildIncremental doesn't recopy these.
				outputDir := cfg.Build.Output
				if !filepath.IsAbs(outputDir) {
					outputDir = filepath.Join(cfg.ProjectRoot, outputDir)
				}
				var recopyConflicts []server.BuildError
				for _, ev := range events {
					if server.RebuildScopeForChangeType(ev.ChangeType) != server.RebuildRecopy {
						continue
					}
					srcPath := filepath.Join(cfg.ProjectRoot, ev.Path)
					var destPath string
					switch ev.ChangeType {
					case server.StaticChange:
						rel, _ := filepath.Rel(cfg.Structure.Static, ev.Path)
						destPath = filepath.Join(outputDir, rel)
					case server.AssetChange:
						rel, _ := filepath.Rel(cfg.Structure.Assets, ev.Path)
						destPath = filepath.Join(outputDir, rel)
					case server.PassthroughChange:
						dest, err := server.RecopyPassthroughFile(ev.Path, cfg)
						if err != nil {
							log.Printf("warning: passthrough recopy: %v", err)
							continue
						}
						destPath = filepath.Join(cfg.ProjectRoot, dest)
					}
					if destPath == "" {
						continue
					}
					// Whether this event may touch the destination depends on
					// who else claims it (issue #1238). A path this very file
					// already owns is not a conflict — matching is by source
					// file, not by the claim's display label, which for a
					// passthrough names the mapping rather than the file.
					source := filepath.ToSlash(ev.Path)
					claimPath := ""
					if rel, relErr := filepath.Rel(outputDir, destPath); relErr == nil {
						claimPath = filepath.ToSlash(rel)
					}
					existing, clash := recopyBlockedBy(outputClaims, copyOrigins, claimPath, source)

					if ev.IsRemove {
						// Deleting a file that lost a conflict must not remove
						// the output the winning source owns — this file was
						// never the one written there.
						if clash {
							continue
						}
						if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
							log.Printf("warning: recopy remove %s: %v", ev.Path, err)
						}
						outputClaims, copyOrigins = releaseClaim(outputClaims, copyOrigins, claimPath, source)
						continue
					}
					// A recopy must not perform a colliding write. Report it in
					// the overlay and skip the write; the server keeps running
					// and the next clean rebuild clears the overlay.
					if clash {
						msg := fmt.Sprintf("output path conflict: %s is claimed by %s and %s — skipping copy",
							claimPath, existing, source)
						log.Printf("warning: %s", msg)
						recopyConflicts = append(recopyConflicts, server.BuildError{
							FilePath: source,
							Message:  msg,
							Stage:    "output path conflict",
						})
						continue
					}
					if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
						log.Printf("warning: recopy mkdir: %v", err)
						continue
					}
					if err := fileutil.CopyFile(srcPath, destPath); err != nil {
						if !errors.Is(err, fs.ErrNotExist) {
							log.Printf("warning: recopy %s: %v", ev.Path, err)
						}
						continue
					}
					// A file copied after the last build claims its path from
					// now on, so a second source added later collides with it.
					outputClaims, copyOrigins = recordClaim(outputClaims, copyOrigins, claimPath, source)
				}
				if len(recopyConflicts) > 0 {
					srv.Overlay().SetErrors(recopyConflicts)
				}

				needsRebuild := false
				for _, ev := range events {
					if server.RebuildScopeForChangeType(ev.ChangeType) == server.RebuildPipeline {
						needsRebuild = true
						break
					}
				}

				// Dependency invalidation from onFileChanged forces an
				// incremental rebuild even when the watcher events are
				// passthrough-only (e.g., component JS recopy). The dep
				// paths are added to changedFiles so BuildIncremental can
				// look up affected pages via the cache (issue #1100).
				if len(depInvalidPaths) > 0 {
					needsRebuild = true
				}

				if !needsRebuild {
					srv.BroadcastReload()
					return
				}

				if !cfg.Quiet {
					log.Printf("rebuilding (%d files changed)...", len(events))
				}

				hasFullRebuildChange := false
				for _, ev := range events {
					if ev.ChangeType == server.ComponentChange || ev.ChangeType == server.PluginChange {
						hasFullRebuildChange = true
						break
					}
				}

				// Restart Node bridges before rebuild when the plugin
				// requested it — clears Node's ESM module cache so
				// import()ed component definitions are re-read (issue #1100).
				// On failure, show an overlay error and skip the rebuild —
				// proceeding with stale bridges would produce wrong output.
				if needsRestart {
					if err := registry.RestartNodeRuntimes(); err != nil {
						log.Printf("warning: restarting Node bridge: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "runtime restart"},
						})
						srv.BroadcastReload()
						return
					}
				}

				if hasFullRebuildChange || rebuildScope == server.RebuildFull {
					if fullResult, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true, Reporter: reporter}); err != nil {
						log.Printf("rebuild failed: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "rebuild"},
						})
					} else {
						if fullResult != nil && fullResult.Cache != nil {
							previousCache = fullResult.Cache
						}
						// Refresh the claim set so a conflict the author has
						// since resolved stops being reported (issue #1238).
						if fullResult != nil {
							outputClaims = fullResult.OutputClaims
							copyOrigins = fullResult.CopyOrigins
							if copyOrigins == nil {
								copyOrigins = map[string]string{}
							}
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
					for _, dp := range depInvalidPaths {
						changedFiles = append(changedFiles, filepath.ToSlash(filepath.Clean(dp)))
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
						// An incremental rebuild re-checks its output path
						// claims and reports collisions without failing, so the
						// server keeps serving (issue #1238).
						var claimErrs []server.BuildError
						if incrResult != nil {
							if incrResult.OutputClaims != nil {
								outputClaims = incrResult.OutputClaims
							}
							if incrResult.CopyOrigins != nil {
								copyOrigins = incrResult.CopyOrigins
							}
							for _, e := range incrResult.Errors {
								if e == nil {
									continue
								}
								log.Printf("warning: %v", e)
								claimErrs = append(claimErrs, server.BuildError{
									Message: e.Error(),
									Stage:   "output path conflict",
								})
							}
						}
						if len(claimErrs) > 0 {
							srv.Overlay().SetErrors(claimErrs)
						} else {
							srv.Overlay().ClearErrors()
						}
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

			server.RemoveLockfileIfOwned(cfg.ProjectRoot, os.Getpid())

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

// recopyBlockedBy reports the claim that prevents source from writing relPath,
// or ok=false when the write is allowed. A path whose recorded origin is this
// same source file is the file's own claim, not a collision — origins are
// matched rather than display labels, because a passthrough claim is labelled
// with its mapping ("passthrough \"vendor-css\" → \"css\"") while the watcher
// reports the individual file that changed.
func recopyBlockedBy(claims []validation.OutputPathEntry, origins map[string]string, relPath, source string) (string, bool) {
	if relPath == "" {
		return "", false
	}
	if origin, ok := origins[relPath]; ok && origin == source {
		return "", false
	}
	for _, c := range claims {
		if c.Path == relPath {
			return c.Source, true
		}
	}
	return "", false
}

// recordClaim registers a path written by a watcher recopy, so a second source
// appearing later is detected as a collision rather than silently overwriting.
func recordClaim(claims []validation.OutputPathEntry, origins map[string]string, relPath, source string) ([]validation.OutputPathEntry, map[string]string) {
	if relPath == "" {
		return claims, origins
	}
	if origin, ok := origins[relPath]; ok && origin == source {
		return claims, origins
	}
	origins[relPath] = source
	for _, c := range claims {
		if c.Path == relPath {
			return claims, origins
		}
	}
	return append(claims, validation.OutputPathEntry{Path: relPath, Source: source}), origins
}

// releaseClaim drops the claim a removed source held, so a different source may
// legitimately take that path afterwards.
func releaseClaim(claims []validation.OutputPathEntry, origins map[string]string, relPath, source string) ([]validation.OutputPathEntry, map[string]string) {
	if relPath == "" {
		return claims, origins
	}
	if origin, ok := origins[relPath]; !ok || origin != source {
		return claims, origins
	}
	delete(origins, relPath)
	kept := claims[:0]
	for _, c := range claims {
		if c.Path == relPath && c.Source == source {
			continue
		}
		kept = append(kept, c)
	}
	return kept, origins
}
