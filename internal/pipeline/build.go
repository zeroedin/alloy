package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/assets"
	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/static"
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

	config.ApplyDefaults(cfg)

	// Validate output directory doesn't overlap with managed directories
	if err := validateOutputDir(cfg); err != nil {
		return nil, err
	}

	// Stage 1: Create template engine and register built-in filters
	var engine tmpl.TemplateEngine
	if cfg.Templates.Engine == "gotemplate" {
		engine = tmpl.NewGoEngine()
	} else {
		engine = tmpl.NewLiquidEngine()
	}
	if err := tmpl.RegisterBuiltinFilters(engine); err != nil {
		return nil, fmt.Errorf("registering template filters: %w", err)
	}

	// Configure include/render tag resolution from layouts directory
	if setter, ok := engine.(interface{ SetIncludesDir(string) }); ok {
		setter.SetIncludesDir(resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts))
	}

	// Discover content
	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	pages, err := content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
	if err != nil {
		// Stage 2: Handle missing content directory gracefully
		if errors.Is(err, fs.ErrNotExist) {
			return &BuildResult{
				OutputDir:  cfg.Build.Output,
				PageCount:  0,
				Duration:   time.Since(start),
				SSRSkipped: cfg.SSR == nil,
			}, nil
		}
		return nil, fmt.Errorf("content discovery: %w", err)
	}

	// Filter by lifecycle (draft/publish/expiry)
	pages = content.FilterByLifecycle(pages, time.Now(), false)

	// Stage 3: Resolve permalinks
	for _, page := range pages {
		url, err := permalink.ResolveForSection(page, cfg.Permalinks)
		if err != nil {
			return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
		}
		page.URL = url
	}

	// Load data files
	siteData := loadSiteData(cfg)

	// Build collections and taxonomies
	collectionsCtx := buildCollectionsContext(pages, cfg)

	// Stage 4: Render each page (with engine for custom filter support)
	rendered, renderErr := renderPages(pages, cfg, siteData, collectionsCtx, engine)
	if renderErr != nil {
		return nil, renderErr
	}

	// Stage 5: Layout resolution and rendering
	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	engineName := cfg.Templates.Engine
	for _, page := range pages {
		layoutPath, err := tmpl.ResolveLayout(page, layoutsDir, engineName, cfg.Permalinks)
		if err != nil {
			// No layout found on disk — skip layout wrapping
			continue
		}
		if layoutPath == "" {
			// layout: false — skip
			continue
		}

		layoutContent, err := os.ReadFile(layoutPath)
		if err != nil {
			return nil, fmt.Errorf("reading layout %s: %w", layoutPath, err)
		}

		tpl, err := engine.Parse(layoutPath, layoutContent)
		if err != nil {
			return nil, fmt.Errorf("parsing layout %s: %w", layoutPath, err)
		}

		ctx := buildTemplateContext(page, cfg, pages, siteData, collectionsCtx, page.RenderedBody)
		ctx["content"] = string(page.RenderedBody) // spec §6 step 14: top-level {{ content }} for layouts
		layoutResult, err := tpl.Render(ctx)
		if err != nil {
			return nil, fmt.Errorf("rendering layout %s: %w", layoutPath, err)
		}
		page.RenderedBody = layoutResult
	}

	// Stage 6: Output writing
	outputDir := resolveDir(cfg.ProjectRoot, cfg.Build.Output)
	if cfg.Build.Clean {
		if _, statErr := os.Stat(outputDir); statErr == nil {
			if err := output.CleanOutputDir(outputDir); err != nil {
				return nil, fmt.Errorf("cleaning output directory: %w", err)
			}
		}
	}
	for _, page := range pages {
		if !output.ShouldWrite(page.URL) {
			continue
		}
		outPath := output.ComputeOutputPath(page.URL)
		if err := output.WriteFile(outputDir, outPath, page.RenderedBody); err != nil {
			return nil, fmt.Errorf("writing output %s: %w", outPath, err)
		}
	}

	// Stage 7: Static files, assets, and passthrough copy
	staticDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Static)
	if err := static.CopyStatic(staticDir, outputDir); err != nil {
		return nil, fmt.Errorf("copying static files: %w", err)
	}
	assetsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Assets)
	if err := assets.CopyAssets(assetsDir, outputDir); err != nil {
		return nil, fmt.Errorf("copying assets: %w", err)
	}
	if len(cfg.Passthrough) > 0 {
		managedDirs := []string{
			cfg.Structure.Content,
			cfg.Structure.Layouts,
			cfg.Structure.Assets,
			cfg.Structure.Static,
			cfg.Structure.Data,
		}
		if err := static.CopyPassthroughWithValidation(cfg.Passthrough, cfg.ProjectRoot, outputDir, managedDirs); err != nil {
			return nil, fmt.Errorf("copying passthrough files: %w", err)
		}
	}

	// Stage 8: Sitemap generation
	if len(pages) > 0 {
		sitemapXML, err := output.GenerateSitemap(pages, cfg.Sitemap, cfg.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("generating sitemap: %w", err)
		}
		if err := output.WriteFile(outputDir, "sitemap.xml", sitemapXML); err != nil {
			return nil, fmt.Errorf("writing sitemap: %w", err)
		}
	}

	// Stage 9: Cache persistence (non-fatal, only with a real project root)
	if cfg.ProjectRoot != "" {
		buildCache := cache.New()
		for _, page := range pages {
			buildCache.SetHash(page.RelPath, cache.HashContent(page.Content))
		}
		cacheDir := resolveDir(cfg.ProjectRoot, ".alloy")
		if err := buildCache.SaveTo(cacheDir); err != nil {
			log.Printf("warning: failed to save build cache: %v", err)
		}
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

	config.ApplyDefaults(cfg)

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

	// Render each page (no data files, collections, or engine in injected content mode)
	rendered, renderErr := renderPages(pages, cfg, nil, nil, nil)
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
	config.ApplyDefaults(cfg)

	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
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
// When engine is non-nil, it is used for template rendering (with custom filters).
// When engine is nil (BuildWithContent path), the standalone RenderTemplate is
// used with strict filters to catch undefined filter usage in tests.
func renderPages(pages []*content.Page, cfg *config.Config, siteData map[string]interface{}, collectionsCtx map[string]interface{}, engine tmpl.TemplateEngine) ([]string, error) {
	mdOpts := content.MarkdownOptions{
		Unsafe:       cfg.Content.Markdown.Goldmark.Unsafe,
		Typographer:  cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags: cfg.Content.Markdown.Goldmark.TemplateTags,
	}

	var rendered []string

	for _, page := range pages {
		body := page.Body

		// Step 1: Render markdown first per spec §6 Phase 1 steps 3–4.
		// Goldmark's TemplateTags extension preserves {{ }} and {% %} as
		// raw nodes. Code fences protect their contents automatically
		// (goldmark's parsers take precedence over the template tag extension).
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
			return nil, fmt.Errorf("content transformation: %s: %w", page.RelPath, err)
		}

		// Step 2: Render template tags with full page/site context.
		if hasTemplateSyntax(html) {
			ctx := buildTemplateContext(page, cfg, pages, siteData, collectionsCtx, html)
			if engine != nil {
				tpl, err := engine.Parse(page.RelPath, html)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				rendered, err := tpl.Render(ctx)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				html = rendered
			} else {
				result, err := tmpl.RenderTemplate(string(html), page.RelPath, ctx)
				if err != nil {
					return nil, fmt.Errorf("template rendering: %s", err.Error())
				}
				html = []byte(result)
			}
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

// buildTemplateContext creates the template rendering context for a page
// per spec §3 (Template Context). renderedHTML is the markdown-rendered
// content (pre-template processing), exposed as page.content.
func buildTemplateContext(page *content.Page, cfg *config.Config, allPages []*content.Page, siteData map[string]interface{}, collectionsCtx map[string]interface{}, renderedHTML []byte) map[string]interface{} {
	// Page context: start with front matter, then overlay computed fields.
	pageCtx := make(map[string]interface{}, len(page.FrontMatter)+5)
	for k, v := range page.FrontMatter {
		pageCtx[k] = v
	}
	if page.URL != "" {
		pageCtx["url"] = page.URL
	}
	if !page.Date.IsZero() {
		pageCtx["date"] = page.Date
	}
	if page.Section != "" {
		pageCtx["collection"] = page.Section
	}
	if len(renderedHTML) > 0 {
		pageCtx["content"] = string(renderedHTML)
	}

	// Site context
	site := map[string]interface{}{
		"title":   cfg.Title,
		"baseURL": cfg.BaseURL,
	}
	if siteData != nil {
		site["data"] = siteData
	} else {
		site["data"] = make(map[string]interface{})
	}
	if allPages != nil {
		site["pages"] = allPages
	}
	if collectionsCtx != nil {
		site["collections"] = collectionsCtx
	}

	ctx := make(map[string]interface{})
	ctx["page"] = pageCtx
	ctx["site"] = site
	if collectionsCtx != nil {
		ctx["collections"] = collectionsCtx
	}
	return ctx
}

// loadSiteData loads data files from the configured data directory.
// Returns nil if the directory doesn't exist. Logs a warning if the
// directory exists but contains files that fail to parse.
func loadSiteData(cfg *config.Config) map[string]interface{} {
	dataDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Data)
	if dataDir == "" {
		return nil
	}
	loaded, err := data.LoadDirectory(dataDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("warning: failed to load data directory %s: %v", dataDir, err)
		}
		return nil
	}
	return loaded
}

// buildCollectionsContext builds section collections and taxonomies,
// returning them as a template-friendly map.
func buildCollectionsContext(pages []*content.Page, cfg *config.Config) map[string]interface{} {
	result := make(map[string]interface{})

	// Section collections
	colls := collection.BuildCollections(pages, cfg.Permalinks)
	for name, coll := range colls {
		result[name] = coll.Pages
	}

	// Taxonomy collections
	if cfg.Taxonomies != nil {
		taxonomies := collection.BuildTaxonomies(pages, cfg.Taxonomies)
		for name, tc := range taxonomies {
			result[name] = tc.Terms
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// resolveDir resolves a relative directory against the project root.
// If projectRoot is empty, the directory is returned as-is (relative to CWD).
func resolveDir(projectRoot, dir string) string {
	if projectRoot == "" || filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(projectRoot, dir)
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
		// Check for exact match or parent/child nesting — not substring.
		// "my_content_site" must NOT match "content", but "content" and
		// "content/output" must match.
		if outputClean == managedClean ||
			strings.HasPrefix(outputClean, managedClean+string(filepath.Separator)) ||
			strings.HasPrefix(managedClean, outputClean+string(filepath.Separator)) {
			return fmt.Errorf("output directory %q conflicts with managed directory %q", outputClean, managedClean)
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
