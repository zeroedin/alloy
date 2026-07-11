package permalink

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/zeroedin/alloy/internal/content"
)

// PermalinkRenderer renders a template permalink string with a page context.
type PermalinkRenderer func(source string, ctx map[string]interface{}) (string, error)

// Resolve computes the output URL for a page using config patterns and front matter overrides.
// When a front matter permalink contains {{ }}, it is rendered through the provided renderer.
// Token syntax and template syntax are mutually exclusive — when {{ is detected, tokens
// like :year and :slug are not resolved.
func Resolve(pattern string, page *content.Page, renderers ...PermalinkRenderer) (string, error) {
	// Check for permalink: false
	if val, ok := page.FrontMatter["permalink"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
		if s, ok := val.(string); ok && s != "" {
			if ContainsLiquidTags(s) {
				return renderTemplatePermalink(s, page, renderers)
			}
			return s, nil
		}
	}

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

// ResolveFromCascade computes the output URL for a page using permalink
// patterns from the _data.yaml cascade. The lookup order is:
//  1. Front matter permalink (static or template)
//  2. Cascade "permalink" pattern from _data.yaml (supports both tokens and templates)
//  3. File path default (DefaultFromPath)
func ResolveFromCascade(page *content.Page, cascadeData map[string]interface{}, renderers ...PermalinkRenderer) (string, error) {
	if val, ok := page.FrontMatter["permalink"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
		if s, ok := val.(string); ok && s != "" {
			if ContainsLiquidTags(s) {
				return renderTemplatePermalink(s, page, renderers)
			}
			return s, nil
		}
	}

	if isIndexFile(page.RelPath) {
		return DefaultFromPath(page.RelPath), nil
	}

	if pattern, ok := cascadeData["permalink"].(string); ok && pattern != "" {
		if ContainsLiquidTags(pattern) {
			return renderTemplatePermalink(pattern, page, renderers)
		}
		return ResolveTokens(pattern, page), nil
	}

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

// renderTemplatePermalink renders a template permalink string through
// the provided renderer. Returns an error if no renderer is provided,
// the renderer fails, or the result is empty/whitespace.
func renderTemplatePermalink(source string, page *content.Page, renderers []PermalinkRenderer) (string, error) {
	if len(renderers) == 0 || renderers[0] == nil {
		return "", fmt.Errorf("template permalink %q requires a renderer but none was provided", source)
	}

	ctx := buildPermalinkContext(page)
	result, err := renderers[0](source, ctx)
	if err != nil {
		return "", fmt.Errorf("template permalink rendering: %w", err)
	}

	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("template permalink %q rendered to empty string for %s", source, page.RelPath)
	}

	return result, nil
}

// buildPermalinkContext builds the template context map for permalink rendering.
// Includes all front matter fields, page date, slug, summary, and collection.
// Actively excludes url to prevent circular references.
func buildPermalinkContext(page *content.Page) map[string]interface{} {
	pageCtx := make(map[string]interface{}, len(page.FrontMatter)+4)
	for k, v := range page.FrontMatter {
		pageCtx[k] = v
	}
	if !page.Date.IsZero() {
		pageCtx["date"] = page.Date
	}
	if page.Slug != "" {
		pageCtx["slug"] = page.Slug
	}
	if page.Summary != "" {
		pageCtx["summary"] = page.Summary
	}
	if page.Section != "" {
		pageCtx["collection"] = page.Section
	}
	delete(pageCtx, "url")
	delete(pageCtx, "permalink")

	return map[string]interface{}{
		"page": pageCtx,
	}
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

// isIndexFile returns true if the relative path refers to an index file
// (e.g., index.md, index.html, blog/index.md).
func isIndexFile(relPath string) bool {
	base := filepath.Base(relPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return name == "index"
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
