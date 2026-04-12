package static

import (
	"errors"

	"github.com/zeroedin/alloy/internal/config"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// CopyStatic copies all files from staticDir to outputDir.
func CopyStatic(staticDir, outputDir string) error {
	return ErrNotImplemented
}

// CopyPassthrough copies files according to passthrough mappings.
func CopyPassthrough(mappings []config.PassthroughMapping, projectRoot, outputDir string) error {
	return ErrNotImplemented
}

// CopyPassthroughWithValidation copies files according to passthrough mappings,
// silently ignoring any mapping where the "from" path resolves to a managed directory
// (content, layouts, assets, static, data).
func CopyPassthroughWithValidation(mappings []config.PassthroughMapping, projectRoot, outputDir string, managedDirs []string) error {
	return ErrNotImplemented
}
