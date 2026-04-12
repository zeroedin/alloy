package permalink

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/zeroedin/alloy/internal/content"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// Resolve computes the output URL for a page using config patterns and front matter overrides.
func Resolve(pattern string, page *content.Page) (string, error) {
	// Check for permalink: false
	if val, ok := page.FrontMatter["permalink"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
		// Static permalink string from front matter overrides pattern
		if s, ok := val.(string); ok && s != "" {
			return s, nil
		}
	}

	// Check for front matter slug override
	result := ResolveTokens(pattern, page)
	return result, nil
}

// ResolveTokens performs fast token replacement on a permalink pattern.
func ResolveTokens(pattern string, page *content.Page) string {
	result := pattern

	// :year - 4-digit year from page date
	if strings.Contains(result, ":year") {
		year := fmt.Sprintf("%04d", page.Date.Year())
		result = strings.ReplaceAll(result, ":year", year)
	}

	// :month - 2-digit month
	if strings.Contains(result, ":month") {
		month := fmt.Sprintf("%02d", int(page.Date.Month()))
		result = strings.ReplaceAll(result, ":month", month)
	}

	// :day - 2-digit day
	if strings.Contains(result, ":day") {
		day := fmt.Sprintf("%02d", page.Date.Day())
		result = strings.ReplaceAll(result, ":day", day)
	}

	// :slug - slugified title, or filename, or front matter slug
	if strings.Contains(result, ":slug") {
		slug := resolveSlug(page)
		result = strings.ReplaceAll(result, ":slug", slug)
	}

	// :section - top-level directory
	if strings.Contains(result, ":section") {
		result = strings.ReplaceAll(result, ":section", page.Section)
	}

	// :filename - source filename without extension
	if strings.Contains(result, ":filename") {
		filename := filenameWithoutExt(page.RelPath)
		result = strings.ReplaceAll(result, ":filename", filename)
	}

	// :title - raw title from front matter (not slugified)
	if strings.Contains(result, ":title") {
		title := ""
		if t, ok := page.FrontMatter["title"].(string); ok {
			title = t
		}
		result = strings.ReplaceAll(result, ":title", title)
	}

	return result
}

// ContainsLiquidTags returns true if the string contains {{ }} Liquid expressions.
func ContainsLiquidTags(s string) bool {
	return strings.Contains(s, "{{")
}

// DefaultFromPath computes the default URL from a content file's relative path.
func DefaultFromPath(relPath string) string {
	// blog/my-post.md -> /blog/my-post/
	withoutExt := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	withoutExt = filepath.ToSlash(withoutExt)

	// Handle index files
	if strings.HasSuffix(withoutExt, "/index") {
		withoutExt = strings.TrimSuffix(withoutExt, "/index")
	}
	if withoutExt == "index" {
		return "/"
	}

	return "/" + withoutExt + "/"
}

// ResolveForSection computes the output URL for a page using section-to-pattern
// mapping from config. The lookup order is:
//  1. Front matter permalink (static or Liquid)
//  2. Section-specific pattern from permalinkCfg
//  3. "default" pattern from permalinkCfg
//  4. File path default (DefaultFromPath)
func ResolveForSection(page *content.Page, permalinkCfg map[string]string) (string, error) {
	// 1. Front matter permalink takes priority
	if val, ok := page.FrontMatter["permalink"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
		if s, ok := val.(string); ok && s != "" {
			return s, nil
		}
	}

	// 2. Section-specific pattern
	if pattern, ok := permalinkCfg[page.Section]; ok {
		return ResolveTokens(pattern, page), nil
	}

	// 3. Default pattern
	if pattern, ok := permalinkCfg["default"]; ok {
		return ResolveTokens(pattern, page), nil
	}

	// 4. File path default
	return DefaultFromPath(page.RelPath), nil
}

// ResolveAliases returns all alias output paths for a page, as specified
// in the page's front matter "aliases" list. These are additional output
// locations for the same rendered content — not redirects.
func ResolveAliases(page *content.Page) ([]string, error) {
	if len(page.Aliases) == 0 {
		return nil, nil
	}
	return page.Aliases, nil
}

// resolveSlug determines the slug for a page. Priority:
// 1. Front matter slug field
// 2. Slugified title from front matter
// 3. Filename without extension
func resolveSlug(page *content.Page) string {
	// Front matter slug override
	if page.Slug != "" {
		return page.Slug
	}
	if s, ok := page.FrontMatter["slug"].(string); ok && s != "" {
		return s
	}

	// Slugified title
	if title, ok := page.FrontMatter["title"].(string); ok && title != "" {
		return slugify(title)
	}

	// Filename without extension
	return filenameWithoutExt(page.RelPath)
}

// filenameWithoutExt returns the base filename without its extension.
func filenameWithoutExt(relPath string) string {
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// nonAlphaNum matches any character that is not alphanumeric or a hyphen.
var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)
var multiHyphen = regexp.MustCompile(`-{2,}`)

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' {
			return r
		}
		return -1
	}, s)
	s = strings.ReplaceAll(s, " ", "-")
	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
