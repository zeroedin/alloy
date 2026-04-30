package template

import (
	"bufio"
	"fmt"
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

// layoutExtension returns the file extension for the given template engine.
func layoutExtension(engine string) string {
	switch engine {
	case "gotemplate":
		return ".html"
	default:
		return ".liquid"
	}
}

// ResolveLayout finds the correct layout file for a page following the lookup order:
// 1. Front matter layout
// 2. "post" (for pages in date-based permalink sections)
// 3. Section name (for index pages)
// 4. Filename (without extension)
// 5. "default"
// Returns error if no layout file is found on disk.
func ResolveLayout(page *content.Page, layoutsDir string, engine string, permalinkCfg map[string]string) (string, error) {
	ext := layoutExtension(engine)

	// Check for layout: false
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// Build lookup chain
	var candidates []string

	// 1. Front matter layout
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		candidates = append(candidates, filepath.Join(layoutsDir, layout+ext))
	}

	// 2. For pages in date-based permalink sections, try "post" layout
	if isDateBasedSection(page.Section, permalinkCfg) && !isIndexPage(page.RelPath) {
		candidates = append(candidates, filepath.Join(layoutsDir, "post"+ext))
	}

	// 3. Section name (for index pages)
	if isIndexPage(page.RelPath) && page.Section != "" {
		candidates = append(candidates, filepath.Join(layoutsDir, page.Section+ext))
	}

	// 4. Filename (without extension)
	filename := filenameWithoutExt(page.RelPath)
	candidates = append(candidates, filepath.Join(layoutsDir, filename+ext))

	// 5. Default
	candidates = append(candidates, filepath.Join(layoutsDir, "default"+ext))

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for page %q", page.RelPath)
}

// ResolveLayoutForFormat finds the correct layout for a specific output format.
func ResolveLayoutForFormat(page *content.Page, layoutsDir string, engine string, format string) (string, error) {
	ext := layoutExtension(engine)

	var candidates []string

	// Format-specific layout: single.<format>.<engine-ext>
	candidates = append(candidates, filepath.Join(layoutsDir, "single."+format+ext))

	// Section-specific format layout
	if page.Section != "" {
		candidates = append(candidates, filepath.Join(layoutsDir, page.Section+"."+format+ext))
	}

	// Filename-specific format layout
	filename := filenameWithoutExt(page.RelPath)
	candidates = append(candidates, filepath.Join(layoutsDir, filename+"."+format+ext))

	// Default format layout
	candidates = append(candidates, filepath.Join(layoutsDir, "default."+format+ext))

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for page %q with format %q", page.RelPath, format)
}

// ResolveTaxonomyLayout finds the layout for a taxonomy page.
// Lookup: layouts/taxonomies/<name>.<ext> → layouts/<name>.<ext>
func ResolveTaxonomyLayout(taxonomyName string, layoutOverride string, layoutsDir string, engine string) (string, error) {
	ext := layoutExtension(engine)
	name := taxonomyName
	if layoutOverride != "" {
		name = layoutOverride
	}

	candidates := []string{
		filepath.Join(layoutsDir, "taxonomies", name+ext),
		filepath.Join(layoutsDir, name+ext),
	}

	if path, ok := resolveFirstExisting(candidates); ok {
		return path, nil
	}
	return "", fmt.Errorf("no layout found for taxonomy %q", taxonomyName)
}

// DetectCircularLayouts checks for circular references in layout chains.
func DetectCircularLayouts(layoutsDir string) error {
	// Scan all layout files for "layout:" or "extends:" directives
	layouts := make(map[string]string) // file -> parent layout

	err := filepath.Walk(layoutsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
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
// Returns error if the chain exceeds maxDepth (10) or if a referenced layout is not found.
func ResolveLayoutChain(layoutPath string, layoutsDir string, engine string) ([]string, error) {
	const maxDepth = 10
	ext := layoutExtension(engine)
	chain := []string{layoutPath}

	current := layoutPath
	for i := 0; i < maxDepth; i++ {
		parent := ExtractLayoutParent(current)
		if parent == "" {
			return chain, nil
		}
		parentPath := filepath.Join(layoutsDir, parent+ext)
		if _, err := os.Stat(parentPath); err != nil {
			return nil, fmt.Errorf("parent layout %q not found (referenced from %s)", parent, filepath.Base(current))
		}
		chain = append(chain, parentPath)
		current = parentPath
	}

	return nil, fmt.Errorf("layout chain exceeds maximum depth of %d levels", maxDepth)
}

// ResolveLayoutWithCascade resolves layout considering cascade data.
func ResolveLayoutWithCascade(page *content.Page, layoutsDir, engine string, permalinkCfg map[string]string, cascadeData map[string]interface{}) (string, error) {
	ext := layoutExtension(engine)

	// Check for layout: false in front matter
	if val, ok := page.FrontMatter["layout"]; ok {
		if b, ok := val.(bool); ok && !b {
			return "", nil
		}
	}

	// 1. Front matter layout takes priority
	if layout, ok := page.FrontMatter["layout"].(string); ok && layout != "" {
		return filepath.Join(layoutsDir, layout+ext), nil
	}

	// 2. Cascade data layout
	if cascadeData != nil {
		if layout, ok := cascadeData["layout"].(string); ok && layout != "" {
			return filepath.Join(layoutsDir, layout+ext), nil
		}
	}

	// 3. Fall back to standard resolution
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
