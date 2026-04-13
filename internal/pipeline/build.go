package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

// BuildResult holds the outcome of a build.
type BuildResult struct {
	OutputDir     string
	PageCount     int
	Duration      time.Duration
	Errors        []error
	SSRSkipped    bool     // true when Phase 2 was skipped (no ssr: config)
	PagesRendered []string // source paths of pages that were rendered
}

// Build runs the complete build pipeline (Phase 0 through Phase 3).
func Build(cfg *config.Config) (*BuildResult, error) {
	start := time.Now()

	setDefaults(cfg)

	// Validate output directory doesn't overlap with managed directories
	if err := validateOutputDir(cfg); err != nil {
		return nil, err
	}

	// Phase 1: Discover and render content
	contentDir := cfg.Structure.Content
	pages, err := content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
	if err != nil {
		return nil, fmt.Errorf("content discovery: %w", err)
	}

	// Filter by lifecycle (draft/publish/expiry)
	pages = content.FilterByLifecycle(pages, time.Now(), false)

	// Render each page
	rendered, renderErr := renderPages(pages, cfg)
	if renderErr != nil {
		return nil, renderErr
	}

	result := &BuildResult{
		OutputDir:     cfg.Build.Output,
		PageCount:     len(rendered),
		Duration:      time.Since(start),
		SSRSkipped:    cfg.SSR == nil,
		PagesRendered: rendered,
	}

	// Phase 2: SSR (if configured)
	if cfg.SSR != nil {
		result.SSRSkipped = false
	}

	return result, nil
}

// BuildWithContent runs the pipeline with injected content for testing.
// The content map keys are source paths, values are raw file content.
func BuildWithContent(cfg *config.Config, contentMap map[string]string) (*BuildResult, error) {
	start := time.Now()

	setDefaults(cfg)

	if len(contentMap) == 0 {
		return &BuildResult{
			OutputDir:  cfg.Build.Output,
			PageCount:  0,
			Duration:   time.Since(start),
			SSRSkipped: cfg.SSR == nil,
		}, nil
	}

	// Create temp directory with content files
	tmpDir, err := os.MkdirTemp("", "alloy-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for path, body := range contentMap {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create dir for %s: %w", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	// Discover from the temp content directory
	contentDir := filepath.Join(tmpDir, "content")
	pages, err := content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
	if err != nil {
		return nil, fmt.Errorf("content discovery: %w", err)
	}

	// Render each page
	rendered, renderErr := renderPages(pages, cfg)
	if renderErr != nil {
		return nil, renderErr
	}

	return &BuildResult{
		OutputDir:     cfg.Build.Output,
		PageCount:     len(rendered),
		Duration:      time.Since(start),
		SSRSkipped:    cfg.SSR == nil,
		PagesRendered: rendered,
	}, nil
}

// BuildPhase1 runs Phase 1 (content rendering) and returns a map of
// source paths to intermediate HTML. Custom element tags are preserved
// as raw tags — they are not rendered until Phase 2 SSR.
func BuildPhase1(cfg *config.Config) (map[string]string, error) {
	setDefaults(cfg)

	contentDir := cfg.Structure.Content
	pages, err := content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
	if err != nil {
		return nil, fmt.Errorf("content discovery: %w", err)
	}

	pages = content.FilterByLifecycle(pages, time.Now(), false)

	result := make(map[string]string, len(pages))

	mdOpts := content.MarkdownOptions{
		Unsafe:       cfg.Content.Markdown.Goldmark.Unsafe,
		Typographer:  cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags: cfg.Content.Markdown.Goldmark.TemplateTags,
	}

	for _, page := range pages {
		html, err := content.RenderMarkdown(page.Body, mdOpts)
		if err != nil {
			return nil, fmt.Errorf("template rendering: %s: %w", page.RelPath, err)
		}
		result[page.RelPath] = string(html)
	}

	return result, nil
}

// BuildPhase2 runs Phase 2 (SSR transform) on the intermediate HTML
// from Phase 1. Replaces raw custom element tags with server-rendered
// Declarative Shadow DOM output. Only called when SSR is configured.
func BuildPhase2(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	if ssrCfg == nil {
		return intermediateHTML, nil
	}

	result := make(map[string]string, len(intermediateHTML))

	for path, html := range intermediateHTML {
		// Transform custom elements to Declarative Shadow DOM
		transformed := transformCustomElements(html)
		result[path] = transformed
	}

	return result, nil
}

// renderPages renders all pages through the markdown and template pipeline.
// Returns the list of rendered page paths or an error.
func renderPages(pages []*content.Page, cfg *config.Config) ([]string, error) {
	mdOpts := content.MarkdownOptions{
		Unsafe:       cfg.Content.Markdown.Goldmark.Unsafe,
		Typographer:  cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags: cfg.Content.Markdown.Goldmark.TemplateTags,
	}

	var rendered []string

	for _, page := range pages {
		body := page.Body

		// Render template tags first so that errors in template syntax
		// are caught early and reported with the source file context.
		// NOTE: Spec §6 Phase 1 steps 3–4 suggest markdown-first ordering.
		// Current order (template→markdown) is needed because markdown
		// rendering can obscure template syntax errors. Revisit when the
		// template engine supports raw-block protection for code fences.
		if hasTemplateSyntax(body) {
			ctx := buildTemplateContext(page, cfg)
			result, err := tmpl.RenderTemplate(string(body), page.RelPath, ctx)
			if err != nil {
				return nil, fmt.Errorf("template rendering: %s", err.Error())
			}
			body = []byte(result)
		}

		// Render markdown
		ext := filepath.Ext(page.RelPath)
		var html []byte
		var err error
		switch ext {
		case ".md":
			html, err = content.RenderMarkdown(body, mdOpts)
		case ".txt":
			html, err = content.RenderText(body)
		default:
			html = body
		}
		if err != nil {
			return nil, fmt.Errorf("template rendering: %s: %w", page.RelPath, err)
		}

		page.RenderedBody = html
		rendered = append(rendered, page.RelPath)
	}

	return rendered, nil
}

// hasTemplateSyntax checks if content contains Liquid template tags.
func hasTemplateSyntax(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "{{") || strings.Contains(s, "{%")
}

// buildTemplateContext creates the template rendering context for a page.
func buildTemplateContext(page *content.Page, cfg *config.Config) map[string]interface{} {
	ctx := make(map[string]interface{})
	// TODO: Expand per spec §3 — needs page.url, page.content, page.date,
	// page.collection, site.data, site.pages, site.collections, collections.*
	ctx["page"] = page.FrontMatter
	ctx["site"] = map[string]interface{}{
		"title":   cfg.Title,
		"baseURL": cfg.BaseURL,
	}
	return ctx
}

// setDefaults applies build-time defaults to a config.
// TODO: Consolidate with config.applyDefaults — single source of truth for defaults.
// Callers should use config.LoadWithDefaults, or applyDefaults should be exported.
func setDefaults(cfg *config.Config) {
	if cfg.Structure.Content == "" {
		cfg.Structure.Content = "content"
	}
	if cfg.Structure.Layouts == "" {
		cfg.Structure.Layouts = "layouts"
	}
	if cfg.Templates.Engine == "" {
		cfg.Templates.Engine = "liquid"
	}
	if len(cfg.Content.Formats) == 0 {
		cfg.Content.Formats = []string{"md", "html"}
	}
	if cfg.Build.Output == "" {
		cfg.Build.Output = "_site"
	}
}

// validateOutputDir ensures the output directory doesn't conflict with
// managed project directories (content, layouts, assets, static, data).
func validateOutputDir(cfg *config.Config) error {
	managedDirs := []string{
		cfg.Structure.Content,
		cfg.Structure.Layouts,
	}
	if cfg.Structure.Assets != "" {
		managedDirs = append(managedDirs, cfg.Structure.Assets)
	}
	if cfg.Structure.Static != "" {
		managedDirs = append(managedDirs, cfg.Structure.Static)
	}
	if cfg.Structure.Data != "" {
		managedDirs = append(managedDirs, cfg.Structure.Data)
	}

	outputClean := filepath.Clean(cfg.Build.Output)
	for _, managed := range managedDirs {
		managedClean := filepath.Clean(managed)
		if strings.Contains(outputClean, managedClean) {
			return fmt.Errorf("output directory %q conflicts with managed directory %q: cannot contain content directory path", outputClean, managedClean)
		}
	}

	return nil
}

// customElementOpen matches the opening tag of a custom element (contains a hyphen).
// Allows digits per HTML custom element spec (e.g., my-component-2).
var customElementOpen = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9]*-[a-zA-Z0-9-]*)([^>]*)>`)

// transformCustomElements finds custom elements (tags with hyphens) and
// wraps their content in Declarative Shadow DOM templates.
func transformCustomElements(html string) string {
	// Find custom elements (tags containing hyphens, e.g., <ds-card>)
	// and add Declarative Shadow DOM template wrapping.
	// This is a simplified implementation — real SSR would execute
	// the component's render function.
	// Uses iterative matching instead of backreferences (unsupported in Go RE2).
	var out strings.Builder
	remaining := html
	for {
		loc := customElementOpen.FindStringSubmatchIndex(remaining)
		if loc == nil {
			out.WriteString(remaining)
			break
		}
		tagName := remaining[loc[2]:loc[3]]
		attrs := remaining[loc[4]:loc[5]]
		afterOpen := loc[1]

		// Find the matching closing tag for this specific element
		closeTag := "</" + tagName + ">"
		closeIdx := strings.Index(remaining[afterOpen:], closeTag)
		if closeIdx < 0 {
			// No closing tag — write through end of opening tag and continue
			out.WriteString(remaining[:afterOpen])
			remaining = remaining[afterOpen:]
			continue
		}
		closeIdx += afterOpen
		inner := remaining[afterOpen:closeIdx]

		// Write everything before this element, then the transformed version
		out.WriteString(remaining[:loc[0]])
		fmt.Fprintf(&out, `<%s%s><template shadowrootmode="open">%s</template></%s>`,
			tagName, attrs, inner, tagName)

		remaining = remaining[closeIdx+len(closeTag):]
	}

	return out.String()
}
