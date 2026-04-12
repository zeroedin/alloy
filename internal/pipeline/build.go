package pipeline

import (
	"errors"
	"time"

	"github.com/zeroedin/alloy/internal/config"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// BuildResult holds the outcome of a build.
type BuildResult struct {
	OutputDir     string
	PageCount     int
	Duration      time.Duration
	Errors        []error
	SSRSkipped    bool     // true when Phase 2 was skipped (no ssr: config)
	PagesRendered []string // source paths of pages that were rendered
}

// Build runs the complete build pipeline (Phase 0 through Phase 3).
func Build(cfg *config.Config) (*BuildResult, error) {
	return nil, ErrNotImplemented
}

// BuildWithContent runs the pipeline with injected content for testing.
// The content map keys are source paths, values are raw file content.
func BuildWithContent(cfg *config.Config, content map[string]string) (*BuildResult, error) {
	return nil, ErrNotImplemented
}

// BuildPhase1 runs Phase 1 (content rendering) and returns a map of
// source paths to intermediate HTML. Custom element tags are preserved
// as raw tags — they are not rendered until Phase 2 SSR.
func BuildPhase1(cfg *config.Config) (map[string]string, error) {
	return nil, ErrNotImplemented
}

// BuildPhase2 runs Phase 2 (SSR transform) on the intermediate HTML
// from Phase 1. Replaces raw custom element tags with server-rendered
// Declarative Shadow DOM output. Only called when SSR is configured.
func BuildPhase2(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	return nil, ErrNotImplemented
}
