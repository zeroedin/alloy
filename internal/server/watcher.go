package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/static"
)

// ChangeType classifies a file change to determine rebuild scope.
type ChangeType int

const (
	// ContentChange means a file in content/ was modified.
	ContentChange ChangeType = iota + 1
	// LayoutChange means a file in layouts/ was modified.
	LayoutChange
	// DataChange means a file in data/ or a _data.yaml was modified.
	DataChange
	// AssetChange means a file in assets/ was modified.
	AssetChange
	// StaticChange means a file in static/ was modified.
	StaticChange
	// ComponentChange means a component source file was modified.
	ComponentChange
	// PassthroughChange means a file in a passthrough from: directory was modified.
	PassthroughChange
)

// ChangeEvent represents a single file change detected by the watcher.
type ChangeEvent struct {
	Path       string
	ChangeType ChangeType
}

// RebuildScope indicates whether to do an incremental or full rebuild.
type RebuildScope int

const (
	// RebuildIncremental means only affected pages are rebuilt.
	RebuildIncremental RebuildScope = iota + 1
	// RebuildFull means all pages are rebuilt (triggered by bulk changes, config, etc.).
	RebuildFull
	// RebuildPipeline means the change requires running the pipeline (content, layouts, data).
	RebuildPipeline
	// RebuildRecopy means the change only requires recopying files (static, assets, passthrough).
	RebuildRecopy
)

// structureDir returns a config directory or falls back to the default name.
func structureDir(configured, fallback string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/\\")
	}
	return fallback
}

// WatchDirs returns the list of directories to watch for file changes,
// derived from the project config. Always includes content/, layouts/,
// data/, assets/, static/. Adds component source dirs when SSR is configured.
func WatchDirs(cfg *config.Config) []string {
	dirs := []string{
		structureDir(cfg.Structure.Content, "content"),
		structureDir(cfg.Structure.Layouts, "layouts"),
		structureDir(cfg.Structure.Data, "data"),
		structureDir(cfg.Structure.Assets, "assets"),
		structureDir(cfg.Structure.Static, "static"),
	}
	if cfg.SSR != nil {
		dirs = append(dirs, "components")
	}
	for _, pt := range cfg.Passthrough {
		if static.ContainsGlobChars(pt.From) {
			dirs = append(dirs, static.GlobRoot(pt.From))
		} else {
			dirs = append(dirs, pt.From)
		}
	}
	for _, w := range cfg.Watch {
		if static.ContainsGlobChars(w.From) {
			dirs = append(dirs, static.GlobRoot(w.From))
		} else {
			dirs = append(dirs, w.From)
		}
	}
	return dirs
}

// ClassifyChange determines the ChangeType for a modified file path
// based on which watched directory it falls under.
func ClassifyChange(path string, cfg *config.Config) ChangeType {
	contentDir := structureDir(cfg.Structure.Content, "content")
	layoutsDir := structureDir(cfg.Structure.Layouts, "layouts")
	dataDir := structureDir(cfg.Structure.Data, "data")
	assetsDir := structureDir(cfg.Structure.Assets, "assets")
	staticDir := structureDir(cfg.Structure.Static, "static")

	switch {
	case hasPathPrefix(path, contentDir):
		return ContentChange
	case hasPathPrefix(path, layoutsDir):
		return LayoutChange
	case hasPathPrefix(path, dataDir):
		return DataChange
	case hasPathPrefix(path, assetsDir):
		return AssetChange
	case hasPathPrefix(path, staticDir):
		return StaticChange
	case hasPathPrefix(path, "components"):
		return ComponentChange
	default:
		for _, w := range cfg.Watch {
			dir := strings.TrimRight(w.From, "/\\")
			if static.ContainsGlobChars(dir) {
				dir = static.GlobRoot(dir)
			}
			if hasPathPrefix(path, dir) {
				switch w.Type {
				case "content":
					return ContentChange
				case "layout":
					return LayoutChange
				case "data":
					return DataChange
				default:
					continue
				}
			}
		}
		for _, pt := range cfg.Passthrough {
			dir := pt.From
			if static.ContainsGlobChars(dir) {
				dir = static.GlobRoot(dir)
			}
			if hasPathPrefix(path, dir) {
				return PassthroughChange
			}
		}
		return ContentChange
	}
}

// hasPathPrefix checks if a file path is under the given directory.
func hasPathPrefix(path, dir string) bool {
	return strings.HasPrefix(path, dir+"/") || strings.HasPrefix(path, dir+"\\")
}

// RebuildScopeForChangeType returns the rebuild scope for a given change type.
func RebuildScopeForChangeType(ct ChangeType) RebuildScope {
	switch ct {
	case StaticChange, AssetChange, PassthroughChange:
		return RebuildRecopy
	default:
		return RebuildPipeline
	}
}

// RecopyPassthroughFile computes the output path for a changed passthrough file.
func RecopyPassthroughFile(path string, cfg *config.Config) (string, error) {
	outputDir := cfg.Build.Output
	if outputDir == "" {
		outputDir = "_site"
	}

	slashedPath := filepath.ToSlash(path)

	for _, pt := range cfg.Passthrough {
		if static.ContainsGlobChars(pt.From) {
			root := static.GlobRoot(pt.From)
			if !hasPathPrefix(path, root) {
				continue
			}
			slashedFrom := filepath.ToSlash(pt.From)
			slashedRoot := filepath.ToSlash(root)
			relGlob := strings.TrimPrefix(slashedFrom, slashedRoot+"/")
			matched, err := doublestar.Match(relGlob, strings.TrimPrefix(slashedPath, slashedRoot+"/"))
			if err != nil {
				return "", fmt.Errorf("passthrough glob %q: %w", pt.From, err)
			}
			if !matched {
				continue
			}
			relPath, _ := filepath.Rel(root, path)
			normalized := static.NormalizeExcludePatterns(pt.Exclude)
			if len(normalized) > 0 {
				excluded, err := static.MatchExcludeNormalized(normalized, relPath)
				if err != nil {
					return "", err
				}
				if excluded {
					return "", fmt.Errorf("path %q is excluded by passthrough mapping", path)
				}
			}
			return filepath.Join(outputDir, pt.To, relPath), nil
		}

		if !hasPathPrefix(path, pt.From) {
			continue
		}
		relPath, _ := filepath.Rel(pt.From, path)
		normalized := static.NormalizeExcludePatterns(pt.Exclude)
		if len(normalized) > 0 {
			excluded, err := static.MatchExcludeNormalized(normalized, relPath)
			if err != nil {
				return "", err
			}
			if excluded {
				return "", fmt.Errorf("path %q is excluded by passthrough mapping", path)
			}
		}
		return filepath.Join(outputDir, pt.To, relPath), nil
	}
	return "", fmt.Errorf("path %q does not match any passthrough mapping", path)
}

// ReloadMessage returns the JSON message sent to the browser via WebSocket
// to trigger a full page reload.
func ReloadMessage() []byte {
	msg, _ := json.Marshal(map[string]string{"type": "reload"})
	return msg
}

// Debouncer collects rapid file change events and fires a single callback
// after a quiet period (default 50ms). If the number of events within a
// single debounce window exceeds the bulk threshold, it signals a full
// rebuild instead of incremental.
type Debouncer struct {
	interval      time.Duration
	bulkThreshold int
}

// NewDebouncer creates a debouncer with the given quiet interval and bulk
// change threshold.
func NewDebouncer(interval time.Duration, bulkThreshold int) *Debouncer {
	return &Debouncer{
		interval:      interval,
		bulkThreshold: bulkThreshold,
	}
}

// Debounce accepts a stream of change events and calls onRebuild once after
// the quiet interval elapses. Returns the accumulated events and the
// recommended rebuild scope (incremental vs full).
func (d *Debouncer) Debounce(events []ChangeEvent) ([]ChangeEvent, RebuildScope) {
	if len(events) > d.bulkThreshold {
		return events, RebuildFull
	}
	return events, RebuildIncremental
}
