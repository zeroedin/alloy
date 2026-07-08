package template

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/content"
)

// resolveFirstExisting returns the first candidate path that exists on disk.
func resolveFirstExisting(candidates []string) (string, bool) {
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

// layoutExtension returns the primary file extension for the given template engine.
func layoutExtension(engine string) string {
	switch engine {
	case "gotemplate":
		return ".html"
	default:
		return ".liquid"
	}
}

// layoutCandidates returns the file paths to try for an automatic layout candidate.
// For the Liquid engine, each candidate tries .liquid first then bare .html (per-candidate interleaving).
// For gotemplate, only the native .html extension is used.
func layoutCandidates(dir, name, engine string) []string {
	if engine == "liquid" {
		return []string{
			filepath.Join(dir, name+".liquid"),
			filepath.Join(dir, name+".html"),
		}
	}
	return []string{filepath.Join(dir, name+layoutExtension(engine))}
}

// formatLayoutCandidates returns file paths for a format-specific layout candidate.
// For Liquid: tries name.format.liquid then name.format (bare extension).
// For gotemplate: tries name.format.html.
func formatLayoutCandidates(dir, name, format, engine string) []string {
	if engine == "liquid" {
		return []string{
			filepath.Join(dir, name+"."+format+".liquid"),
			filepath.Join(dir, name+"."+format),
		}
	}
	return []string{filepath.Join(dir, name+"."+format+layoutExtension(engine))}
}

// resolveExplicitLayout checks whether an explicitly-named layout file exists.
// Returns its path if found, or an error if missing.
func resolveExplicitLayout(layoutsDir, name, ext, pageRelPath string) (string, error) {
	path := filepath.Join(layoutsDir, name+ext)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("layout %q not found for page %q", name+ext, pageRelPath)
}

// ResolveLayout finds the correct layout file for a page following the lookup order:
// 1. Front matter layout (explicit — Liquid: strict hard error; gotemplate: fall through)
// 2. "post" (for pages in date-based permalink sections)
// 3. Section name (for index pages)
// 4. Filename (without extension)
// 5. "default"
// For the Liquid engine, auto candidates (steps 2-5) try .liquid then bare .html
// per candidate before moving to the next candidate (per-candidate interleaving).
// Returns error if no layout file is found on disk.
func ResolveLayout(page *content.Page, layoutsDir string, engine string, permalinkCfg map[string]string) (string, error) {
	ext := layoutExtension(engine)

	// Check for layout: false
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// 1. Explicit front matter layout
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		resolved, err := resolveExplicitLayout(layoutsDir, layout, ext, page.RelPath)
		if err == nil {
			return resolved, nil
		}
		// Liquid engine: strict — hard error, no fallback to auto candidates
		if engine == "liquid" {
			return "", err
		}
		// Other engines: fall through to auto candidates (preserves pre-existing behavior)
	}

	// 2-5. Auto candidates with per-candidate interleaved fallback
	var candidates []string

	if isDateBasedSection(page.Section, permalinkCfg) && !isIndexPage(page.RelPath) {
		candidates = append(candidates, layoutCandidates(layoutsDir, "post", engine)...)
	}

	if isIndexPage(page.RelPath) && page.Section != "" {
		candidates = append(candidates, layoutCandidates(layoutsDir, page.Section, engine)...)
	}

	filename := filenameWithoutExt(page.RelPath)
	candidates = append(candidates, layoutCandidates(layoutsDir, filename, engine)...)

	candidates = append(candidates, layoutCandidates(layoutsDir, "default", engine)...)

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for page %q", page.RelPath)
}

// ResolveLayoutForFormat finds the correct layout for a specific output format.
// For the Liquid engine, each candidate tries .format.liquid then bare .format.
func ResolveLayoutForFormat(page *content.Page, layoutsDir string, engine string, format string) (string, error) {
	var candidates []string

	candidates = append(candidates, formatLayoutCandidates(layoutsDir, "single", format, engine)...)

	if page.Section != "" {
		candidates = append(candidates, formatLayoutCandidates(layoutsDir, page.Section, format, engine)...)
	}

	filename := filenameWithoutExt(page.RelPath)
	candidates = append(candidates, formatLayoutCandidates(layoutsDir, filename, format, engine)...)

	candidates = append(candidates, formatLayoutCandidates(layoutsDir, "default", format, engine)...)

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for page %q with format %q", page.RelPath, format)
}

// ResolveTaxonomyLayout finds the layout for a taxonomy page.
// Lookup per candidate: taxonomies/<name> then root <name>, with per-candidate
// .liquid → .html interleaving for the Liquid engine.
func ResolveTaxonomyLayout(taxonomyName string, layoutOverride string, layoutsDir string, engine string) (string, error) {
	name := taxonomyName
	if layoutOverride != "" {
		name = layoutOverride
	}

	var candidates []string
	candidates = append(candidates, layoutCandidates(filepath.Join(layoutsDir, "taxonomies"), name, engine)...)
	candidates = append(candidates, layoutCandidates(layoutsDir, name, engine)...)

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for taxonomy %q", taxonomyName)
}

// DetectCircularLayouts checks for circular references in layout chains.
func DetectCircularLayouts(layoutsDir string) error {
	// Scan all layout files for "layout:" or "extends:" directives
	layouts := make(map[string]string) // file -> parent layout

	err := filepath.WalkDir(layoutsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		parent := ExtractLayoutParent(path)
		if parent != "" {
			rel, _ := filepath.Rel(layoutsDir, path)
			layouts[rel] = parent
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error scanning layouts: %w", err)
	}

	// Detect cycles
	for start := range layouts {
		visited := make(map[string]bool)
		current := start
		for {
			if visited[current] {
				return fmt.Errorf("circular layout reference detected: %s", current)
			}
			visited[current] = true
			parent, ok := layouts[current]
			if !ok {
				break
			}
			current = parent
		}
	}

	return nil
}

// ExtractLayoutParent reads a layout file and looks for a parent layout reference
// in its front matter. Returns the parent layout name, or "" if none found.
func ExtractLayoutParent(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			if inFrontMatter {
				break // end of front matter
			}
			inFrontMatter = true
			continue
		}
		if inFrontMatter && strings.HasPrefix(line, "layout:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(strings.Trim(parts[1], `"' `))
			}
		}
	}
	return ""
}

// StripLayoutFrontMatter removes YAML front matter (--- delimited) from layout content.
// Returns the content after the closing --- delimiter.
func StripLayoutFrontMatter(s string) string {
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := s[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	// Handle empty front matter (---\n---\n)
	if strings.HasPrefix(rest, "---") {
		body := rest[3:]
		if len(body) > 0 && body[0] == '\n' {
			body = body[1:]
		}
		return body
	}
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return s
	}
	body := rest[idx+4:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	return body
}

// ResolveLayoutChain follows layout: directives in layout front matter to build
// the full chain from innermost to root. Returns the ordered list of layout file paths.
// For the Liquid engine, parent resolution tries .liquid then bare .html.
// Returns error if the chain exceeds maxDepth (10) or if a referenced layout is not found.
func ResolveLayoutChain(layoutPath string, layoutsDir string, engine string) ([]string, error) {
	const maxDepth = 10
	chain := []string{layoutPath}

	current := layoutPath
	for i := 0; i < maxDepth; i++ {
		parent := ExtractLayoutParent(current)
		if parent == "" {
			return chain, nil
		}
		parentPath, found := resolveFirstExisting(layoutCandidates(layoutsDir, parent, engine))
		if !found {
			return nil, fmt.Errorf("parent layout %q not found (referenced from %s)", parent, filepath.Base(current))
		}
		chain = append(chain, parentPath)
		current = parentPath
	}

	return nil, fmt.Errorf("layout chain exceeds maximum depth of %d levels", maxDepth)
}

// ResolveLayoutWithCascade resolves layout considering cascade data.
// Explicit layout names (front matter or cascade) are strict — only the engine's
// primary extension is tried, and missing = hard error with no fallback.
// When no explicit layout is set, falls through to ResolveLayout which applies
// bare-extension fallback for auto candidates.
func ResolveLayoutWithCascade(page *content.Page, layoutsDir, engine string, permalinkCfg map[string]string, cascadeData map[string]interface{}) (string, error) {
	ext := layoutExtension(engine)

	// Check for layout: false in front matter
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// 1. Explicit front matter layout — strict, no bare-extension fallback
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		return resolveExplicitLayout(layoutsDir, layout, ext, page.RelPath)
	}

	// 2. Explicit cascade layout — strict, no bare-extension fallback
	if cascadeData != nil {
		if layout, ok := cascadeData["layout"].(string); ok && layout != "" {
			return resolveExplicitLayout(layoutsDir, layout, ext, page.RelPath)
		}
	}

	// 3. Fall back to standard resolution (with bare-extension fallback for auto candidates)
	return ResolveLayout(page, layoutsDir, engine, permalinkCfg)
}

// isDateBasedSection checks if a section has a date-based permalink pattern.
func isDateBasedSection(section string, permalinkCfg map[string]string) bool {
	pattern, ok := permalinkCfg[section]
	if !ok {
		return false
	}
	return strings.Contains(pattern, ":year") ||
		strings.Contains(pattern, ":month") ||
		strings.Contains(pattern, ":day")
}

// isIndexPage checks if a page is an index file.
func isIndexPage(relPath string) bool {
	base := filepath.Base(relPath)
	return base == "index.md" || base == "index.html"
}

// filenameWithoutExt returns the base filename without its extension.
func filenameWithoutExt(relPath string) string {
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
