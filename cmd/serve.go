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
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/server"
	"github.com/zeroedin/alloy/internal/validation"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Build and serve the production site",
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

			// Production server always excludes drafts
			cfg.IncludeDrafts = false

			// Set up progress reporter for build
			var reporter pipeline.ProgressReporter
			if !cfg.Quiet {
				PrintBanner(cmd.OutOrStdout(), isTTY())
				if cfg.Verbose {
					reporter = pipeline.NewVerboseProgress(cmd.OutOrStdout())
				} else if isTTY() {
					reporter = pipeline.NewTTYProgress(cmd.OutOrStdout(), termWidth())
				}
			}

			// Run the full build pipeline (same as alloy build)
			// Its output path claims are kept so a watcher recopy can check a
			// destination before writing it (issue #1238).
			var outputClaims []validation.OutputPathEntry
			buildResult, err := pipeline.Build(cfg, pipeline.BuildOptions{Reporter: reporter})
			if err != nil {
				return fmt.Errorf("build failed: %w", err)
			}
			if buildResult != nil {
				outputClaims = buildResult.OutputClaims
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

			// Write server lockfile after successful start (issue #1094)
			if err := server.WriteLockfile(cfg.ProjectRoot, server.LockfileInfo{
				PID:       os.Getpid(),
				Port:      actualPort,
				Mode:      "serve",
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

			// Set up file watcher for live rebuild
			var watcher *fsnotify.Watcher
			watcher = startWatcher(cfg, srv, func(events []server.ChangeEvent, _ server.RebuildScope) {
				if !cfg.Quiet {
					log.Printf("rebuilding (%d files changed)...", len(events))
				}

				needsRebuild := false
				for _, ev := range events {
					switch ev.ChangeType {
					case server.ContentChange, server.LayoutChange, server.DataChange, server.ComponentChange, server.PluginChange:
						needsRebuild = true
					case server.AssetChange, server.StaticChange:
						copyChangedFileToOutput(ev.Path, cfg, outputClaims, srv)
					case server.PassthroughChange:
						if dest, err := server.RecopyPassthroughFile(ev.Path, cfg); err == nil {
							srcPath := ev.Path
							destRel := dest
							if cfg.ProjectRoot != "" {
								srcPath = filepath.Join(cfg.ProjectRoot, ev.Path)
								dest = filepath.Join(cfg.ProjectRoot, dest)
							}
							if !reportRecopyConflict(destRel, ev.Path, cfg, outputClaims, srv) {
								copyFileToPath(srcPath, dest, cfg)
							}
						}
					}
				}

				if needsRebuild {
					if rebuilt, err := pipeline.Build(cfg, pipeline.BuildOptions{Reporter: reporter}); err != nil {
						log.Printf("rebuild failed: %v", err)
						srv.Overlay().SetErrors([]server.BuildError{
							{Message: err.Error(), Stage: "rebuild"},
						})
					} else {
						if rebuilt != nil {
							outputClaims = rebuilt.OutputClaims
						}
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

			server.RemoveLockfileIfOwned(cfg.ProjectRoot, os.Getpid())

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

func copyChangedFileToOutput(relPath string, cfg *config.Config, claims []validation.OutputPathEntry, srv *server.Server) {
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
	if reportRecopyConflict(destRel, relPath, cfg, claims, srv) {
		return
	}
	copyFileToPath(srcPath, destPath, cfg)
}

// reportRecopyConflict reports whether an output-relative destination is
// already claimed by a different source. A conflicting recopy is surfaced in
// the error overlay and the write is skipped rather than performed, so a
// mid-session collision cannot silently replace a file (issue #1238). It never
// terminates the server.
func reportRecopyConflict(destRel, srcRel string, cfg *config.Config, claims []validation.OutputPathEntry, srv *server.Server) bool {
	outputDir := cfg.Build.Output
	if outputDir == "" {
		outputDir = "_site"
	}
	claimPath := filepath.ToSlash(destRel)
	if rel, err := filepath.Rel(outputDir, filepath.ToSlash(destRel)); err == nil && !strings.HasPrefix(rel, "..") {
		claimPath = filepath.ToSlash(rel)
	}
	source := filepath.ToSlash(srcRel)
	for _, c := range claims {
		if c.Path != claimPath || c.Source == source || strings.HasSuffix(c.Source, source) {
			continue
		}
		msg := fmt.Sprintf("output path conflict: %s is claimed by %s and %s — skipping copy",
			claimPath, c.Source, source)
		log.Printf("warning: %s", msg)
		if srv != nil {
			srv.Overlay().SetErrors([]server.BuildError{
				{FilePath: source, Message: msg, Stage: "output path conflict"},
			})
		}
		return true
	}
	return false
}

func copyFileToPath(src, dest string, cfg *config.Config) {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		log.Printf("warning: creating directory for %s: %v", dest, err)
		return
	}
	srcFile, err := os.Open(src)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("warning: opening %s: %v", src, err)
		}
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
