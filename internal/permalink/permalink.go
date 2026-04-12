package permalink

import (
	"errors"

	"github.com/zeroedin/alloy/internal/content"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Resolve computes the output URL for a page using config patterns and front matter overrides.
func Resolve(pattern string, page *content.Page) (string, error) {
	return "", ErrNotImplemented
}

// ResolveTokens performs fast token replacement on a permalink pattern.
func ResolveTokens(pattern string, page *content.Page) string {
	return ""
}

// ContainsLiquidTags returns true if the string contains {{ }} Liquid expressions.
func ContainsLiquidTags(s string) bool {
	return false
}

// DefaultFromPath computes the default URL from a content file's relative path.
func DefaultFromPath(relPath string) string {
	return ""
}

// ResolveForSection computes the output URL for a page using section-to-pattern
// mapping from config. The lookup order is:
//  1. Front matter permalink (static or Liquid)
//  2. Section-specific pattern from permalinkCfg
//  3. "default" pattern from permalinkCfg
//  4. File path default (DefaultFromPath)
func ResolveForSection(page *content.Page, permalinkCfg map[string]string) (string, error) {
	return "", ErrNotImplemented
}

// ResolveAliases returns all alias output paths for a page, as specified
// in the page's front matter "aliases" list. These are additional output
// locations for the same rendered content — not redirects.
func ResolveAliases(page *content.Page) ([]string, error) {
	return nil, ErrNotImplemented
}
