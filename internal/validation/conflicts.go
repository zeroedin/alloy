package validation

import (
	"errors"

	"github.com/zeroedin/alloy/internal/content"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// OutputPathEntry represents a single claimed output path and its source.
type OutputPathEntry struct {
	Path   string
	Source string
}

// Conflict represents two or more sources targeting the same output path.
type Conflict struct {
	Path    string
	Sources []string
}

// DetectConflicts scans all output path entries and returns any conflicts found.
func DetectConflicts(entries []OutputPathEntry) ([]Conflict, error) {
	return nil, ErrNotImplemented
}

// ValidatePermalinkAliases checks for conflicting permalink:false + aliases.
// A page with permalink:false should not have aliases.
func ValidatePermalinkAliases(pages []*content.Page) []error {
	return nil
}
