package pipeline

import (
	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
)

// Builder abstracts the build pipeline so callers (e.g. cmd/dev.go)
// can be tested with a mock instead of running the full pipeline.
type Builder interface {
	Build(cfg *config.Config, opts ...BuildOptions) (*BuildResult, error)
	BuildIncremental(cfg *config.Config, contentMap map[string]string, previousCache *cache.Cache, changedFiles []string, opts ...BuildOptions) (*BuildResult, error)
}

// DefaultBuilder delegates to the package-level Build and BuildIncremental functions.
type DefaultBuilder struct{}

func (b *DefaultBuilder) Build(cfg *config.Config, opts ...BuildOptions) (*BuildResult, error) {
	return Build(cfg, opts...)
}

func (b *DefaultBuilder) BuildIncremental(cfg *config.Config, contentMap map[string]string, previousCache *cache.Cache, changedFiles []string, opts ...BuildOptions) (*BuildResult, error) {
	return BuildIncremental(cfg, contentMap, previousCache, changedFiles, opts...)
}
