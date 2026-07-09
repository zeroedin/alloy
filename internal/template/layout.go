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

// recognizedLayoutExtensions contains file extensions that mark a layout name
// as extension-bearing (used as a literal filename, no engine extension appended).
var recognizedLayoutExtensions = map[string]bool{
	".liquid": true,
	".html":   true,
	".xml":    true,
	".json":   true,
	".txt":    true,
}

// hasRecognizedExtension returns true if the layout name ends with a recognized
// template/output extension, indicating it should be used as a literal filename.
func hasRecognizedExtension(name string) bool {
	return recognizedLayoutExtensions[filepath.Ext(name)]
}

// resolveNamedLayout resolves a layout specified by name (from front matter,
// cascade, or layout chain parent references).
// Extension-bearing names (e.g., "base.html") are used as literal filenames.
// Bare names (e.g., "base") use engine-specific candidate resolution
// (.liquid → .html for Liquid, .html only for gotemplate).
func resolveNamedLayout(layoutsDir, name, engine string) (string, bool) {
	if hasRecognizedExtension(name) {
		path := filepath.Join(layoutsDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return "", false
	}
	return resolveFirstExisting(layoutCandidates(layoutsDir, name, engine))
}

// ResolveLayout finds the correct layout file for a page following the lookup order:
// 1. Front matter layout (bare names get engine fallback; extension-bearing used as-is)
// 2. "post" (for pages in date-based permalink sections)
// 3. Section name (for index pages)
// 4. Filename (without extension)
// 5. "default"
// For the Liquid engine, auto candidates (steps 2-5) try .liquid then bare .html
// per candidate before moving to the next candidate (per-candidate interleaving).
// Returns error if no layout file is found on disk.
func ResolveLayout(page *content.Page, layoutsDir string, engine string, permalinkCfg map[string]string) (string, error) {
	// Check for layout: false
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// 1. Front matter layout — resolved via resolveNamedLayout
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		resolved, found := resolveNamedLayout(layoutsDir, layout, engine)
		if found {
			return resolved, nil
		}
		// Extension-bearing or Liquid bare name: hard error, no fall-through
		if engine == "liquid" || hasRecognizedExtension(layout) {
			return "", fmt.Errorf("layout %q not found for page %q", layout, page.RelPath)
		}
		// Gotemplate bare name: fall through to auto candidates (pre-existing behavior)
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

// resolveNamedFormatLayout resolves an explicit layout name for a format output.
// Extension-bearing names (e.g., "feed.xml") are used as literal filenames.
// Bare names get format infixed via formatLayoutCandidates.
func resolveNamedFormatLayout(layoutsDir, name, format, engine string) (string, bool) {
	if hasRecognizedExtension(name) {
		path := filepath.Join(layoutsDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return "", false
	}
	return resolveFirstExisting(formatLayoutCandidates(layoutsDir, name, format, engine))
}

// ResolveLayoutForFormat finds the correct layout for a specific output format.
// Mirrors ResolveLayout with the output format infixed before the engine extension.
// One algorithm, one lookup order — no "single" concept.
// For the Liquid engine, each candidate tries .format.liquid then bare .format.
func ResolveLayoutForFormat(page *content.Page, layoutsDir string, engine string, format string, permalinkCfg map[string]string) (string, error) {
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		if hasRecognizedExtension(layout) {
			bare := strings.TrimSuffix(layout, filepath.Ext(layout))
			return "", fmt.Errorf("extension-bearing layout %q cannot be used with format outputs; use `layout: %s` instead", layout, bare)
		}
		resolved, found := resolveNamedFormatLayout(layoutsDir, layout, format, engine)
		if found {
			return resolved, nil
		}
		return "", fmt.Errorf("layout %q not found for page %q with format %q", layout, page.RelPath, format)
	}

	var candidates []string

	if isDateBasedSection(page.Section, permalinkCfg) && !isIndexPage(page.RelPath) {
		candidates = append(candidates, formatLayoutCandidates(layoutsDir, "post", format, engine)...)
	}

	if isIndexPage(page.RelPath) && page.Section != "" {
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

// ResolveLayoutForFormatWithCascade resolves format layout considering cascade data.
// Mirrors ResolveLayoutWithCascade with the output format infixed.
// Delegates to ResolveLayoutForFormat for auto candidates.
func ResolveLayoutForFormatWithCascade(page *content.Page, layoutsDir, engine, format string, permalinkCfg map[string]string, cascadeData map[string]interface{}) (string, error) {
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		if hasRecognizedExtension(layout) {
			bare := strings.TrimSuffix(layout, filepath.Ext(layout))
			return "", fmt.Errorf("extension-bearing layout %q cannot be used with format outputs; use `layout: %s` instead", layout, bare)
		}
		resolved, found := resolveNamedFormatLayout(layoutsDir, layout, format, engine)
		if found {
			return resolved, nil
		}
		return "", fmt.Errorf("layout %q not found for page %q with format %q", layout, page.RelPath, format)
	}

	if cascadeData != nil {
		if layout, ok := cascadeData["layout"].(string); ok && layout != "" {
			if hasRecognizedExtension(layout) {
				bare := strings.TrimSuffix(layout, filepath.Ext(layout))
				return "", fmt.Errorf("extension-bearing layout %q cannot be used with format outputs; use `layout: %s` instead", layout, bare)
			}
			resolved, found := resolveNamedFormatLayout(layoutsDir, layout, format, engine)
			if found {
				return resolved, nil
			}
			return "", fmt.Errorf("layout %q not found for page %q with format %q", layout, page.RelPath, format)
		}
	}

	return ResolveLayoutForFormat(page, layoutsDir, engine, format, permalinkCfg)
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
// Parent names follow the same bare-name vs extension-bearing rules as front matter layout.
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
		parentPath, found := resolveNamedLayout(layoutsDir, parent, engine)
		if !found {
			return nil, fmt.Errorf("parent layout %q not found (referenced from %s)", parent, filepath.Base(current))
		}
		chain = append(chain, parentPath)
		current = parentPath
	}

	return nil, fmt.Errorf("layout chain exceeds maximum depth of %d levels", maxDepth)
}

// ResolveLayoutWithCascade resolves layout considering cascade data.
// Named layouts (front matter or cascade) use resolveNamedLayout: bare names get
// engine fallback (.liquid → .html), extension-bearing names are used as-is.
// When no explicit layout is set, falls through to ResolveLayout for auto candidates.
func ResolveLayoutWithCascade(page *content.Page, layoutsDir, engine string, permalinkCfg map[string]string, cascadeData map[string]interface{}) (string, error) {
	// Check for layout: false in front matter
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// 1. Front matter layout
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		resolved, found := resolveNamedLayout(layoutsDir, layout, engine)
		if found {
			return resolved, nil
		}
		return "", fmt.Errorf("layout %q not found for page %q", layout, page.RelPath)
	}

	// 2. Cascade layout
	if cascadeData != nil {
		if layout, ok := cascadeData["layout"].(string); ok && layout != "" {
			resolved, found := resolveNamedLayout(layoutsDir, layout, engine)
			if found {
				return resolved, nil
			}
			return "", fmt.Errorf("layout %q not found for page %q", layout, page.RelPath)
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
