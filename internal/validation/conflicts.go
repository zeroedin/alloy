package validation

import (
	"fmt"

	"github.com/zeroedin/alloy/internal/content"
)

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
	// Group entries by output path
	groups := make(map[string][]string)
	// Preserve order of first seen path
	var order []string
	for _, e := range entries {
		if _, exists := groups[e.Path]; !exists {
			order = append(order, e.Path)
		}
		groups[e.Path] = append(groups[e.Path], e.Source)
	}

	var conflicts []Conflict
	for _, path := range order {
		sources := groups[path]
		if len(sources) > 1 {
			conflicts = append(conflicts, Conflict{
				Path:    path,
				Sources: sources,
			})
		}
	}
	return conflicts, nil
}

// ValidatePermalinkAliases checks for conflicting permalink:false + aliases.
// A page with permalink:false should not have aliases.
func ValidatePermalinkAliases(pages []*content.Page) []error {
	var errs []error
	for _, p := range pages {
		// permalink:false is represented as empty URL (no resolved output path)
		if p.URL == "" && len(p.Aliases) > 0 {
			errs = append(errs, fmt.Errorf(
				"%s: page with permalink:false cannot have aliases", p.RelPath))
		}
	}
	return errs
}
