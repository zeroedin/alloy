package server

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/config"
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
)

// WatchDirs returns the list of directories to watch for file changes,
// derived from the project config. Always includes content/, layouts/,
// data/, assets/, static/. Adds component source dirs when SSR is configured.
func WatchDirs(cfg *config.Config) []string {
	dirs := []string{"content", "layouts", "data", "assets", "static"}
	if cfg.SSR != nil {
		dirs = append(dirs, "components")
	}
	return dirs
}

// ClassifyChange determines the ChangeType for a modified file path
// based on which watched directory it falls under.
func ClassifyChange(path string, cfg *config.Config) ChangeType {
	switch {
	case strings.HasPrefix(path, "content/") || strings.HasPrefix(path, "content\\"):
		return ContentChange
	case strings.HasPrefix(path, "layouts/") || strings.HasPrefix(path, "layouts\\"):
		return LayoutChange
	case strings.HasPrefix(path, "data/") || strings.HasPrefix(path, "data\\"):
		return DataChange
	case strings.HasPrefix(path, "assets/") || strings.HasPrefix(path, "assets\\"):
		return AssetChange
	case strings.HasPrefix(path, "static/") || strings.HasPrefix(path, "static\\"):
		return StaticChange
	case strings.HasPrefix(path, "components/") || strings.HasPrefix(path, "components\\"):
		return ComponentChange
	default:
		return ContentChange
	}
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
