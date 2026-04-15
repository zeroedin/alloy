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
	"github.com/zeroedin/alloy/internal/cascade"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/static"
	tmpl "github.com/zeroedin/alloy/internal/template"
	"github.com/zeroedin/alloy/internal/validation"
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

	// Load directory data cascade (_data.yaml files)
	cascadeData, cascadeErr := cascade.LoadDirectoryCascade(contentDir)
	if cascadeErr != nil {
		log.Printf("warning: loading cascade data: %v", cascadeErr)
	}

	// Load data files (Global cascade level 1)
	siteData := loadSiteData(cfg)

	// Build PageContexts per spec §3: shared pointers for Global/Directory,
	// per-page FrontMatter. Levels 4/5 (Computed/PluginData) are nil until
	// plugin hooks populate them.
	contentBase := filepath.Base(contentDir)
	for _, page := range pages {
		var dirData map[string]interface{}
		if len(cascadeData) > 0 {
			dirData = cascade.FindCascadeData(cascadeData, contentBase, page.RelPath)
		}
		pctx := cascade.BuildContext(siteData, dirData, page.FrontMatter)
		// Flatten cascade into FrontMatter so downstream consumers (taxonomy
		// building, collection sorting) see the effective values. The PageContext
		// is the source of truth; FrontMatter becomes the resolved view.
		page.FrontMatter = pctx.ToMap()
	}

	// Build taxonomies once (used for both template context and page generation)
	var taxonomies map[string]*collection.TaxonomyCollection
	if cfg.Taxonomies != nil {
		taxonomies = collection.BuildTaxonomies(pages, cfg.Taxonomies)
	}

	// Build collections and taxonomies for template context
	collectionsCtx := buildCollectionsContext(pages, cfg, taxonomies)

	// Process pagination: detect pages with pagination front matter,
	// resolve data sources, and generate virtual/paginated pages.
	pages = processPagination(pages, cfg, siteData, collectionsCtx)

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

		tc := tmpl.BuildTemplateContext(page, combinedSiteData(cfg, siteData), pages, collectionsCtx, nil, "")
		ctx := tc.ToMap()
		ctx["content"] = string(page.RenderedBody) // spec §6 step 14: top-level {{ content }} for layouts
		layoutResult, err := tpl.Render(ctx)
		if err != nil {
			return nil, fmt.Errorf("rendering layout %s: %w", layoutPath, err)
		}
		page.RenderedBody = layoutResult
	}

	// Generate and render taxonomy pages (virtual index + per-term pages)
	if taxonomies != nil && engine != nil {

		// Detect duplicate term slugs before generating pages
		for taxName, tc := range taxonomies {
			dupes := collection.DetectDuplicateTermSlugs(tc)
			if len(dupes) > 0 {
				return nil, fmt.Errorf("taxonomy %q has duplicate term slugs: %v", taxName, dupes)
			}
		}

		for taxName, tc := range taxonomies {
			taxCfg := cfg.Taxonomies[taxName]

			// Resolve layout once per taxonomy (not per page)
			layoutPath, err := tmpl.ResolveTaxonomyLayout(taxName, taxCfg.Layout, layoutsDir, engineName)
			if err != nil {
				return nil, fmt.Errorf("taxonomy %q layout: %w", taxName, err)
			}
			layoutContent, err := os.ReadFile(layoutPath)
			if err != nil {
				return nil, fmt.Errorf("reading taxonomy layout %s: %w", layoutPath, err)
			}
			tpl, err := engine.Parse(layoutPath, layoutContent)
			if err != nil {
				return nil, fmt.Errorf("parsing taxonomy layout %s: %w", layoutPath, err)
			}

			taxPages := collection.GenerateTaxonomyPages(tc, taxCfg)
			for _, taxPage := range taxPages {
				ctx := tmpl.BuildTemplateContext(taxPage, combinedSiteData(cfg, siteData), pages, collectionsCtx, nil, "").ToMap()
				term := ""
				if taxPage.Kind == "taxonomy_term" {
					if t, ok := taxPage.FrontMatter["title"].(string); ok {
						term = t
					}
				}
				ctx["taxonomy"] = collection.BuildTaxonomyPageContext(tc, term).ToMap()
				ctx["content"] = ""
				out, err := tpl.Render(ctx)
				if err != nil {
					return nil, fmt.Errorf("rendering taxonomy page %s: %w", taxPage.URL, err)
				}
				taxPage.RenderedBody = out
				pages = append(pages, taxPage)
			}
		}
	}

	// Pre-build validation: permalink/alias conflicts
	if aliasErrs := validation.ValidatePermalinkAliases(pages); len(aliasErrs) > 0 {
		return nil, aliasErrs[0]
	}
	var outputEntries []validation.OutputPathEntry
	for _, page := range pages {
		if !output.ShouldWrite(page.URL) {
			continue
		}
		outPath := output.ComputeOutputPath(page.URL)
		outputEntries = append(outputEntries, validation.OutputPathEntry{
			Path: outPath, Source: page.RelPath,
		})
		aliases, _ := permalink.ResolveAliases(page)
		for _, alias := range aliases {
			aliasPath := output.ComputeOutputPath(alias)
			outputEntries = append(outputEntries, validation.OutputPathEntry{
				Path: aliasPath, Source: page.RelPath + " (alias)",
			})
		}
	}
	if conflicts, _ := validation.DetectConflicts(outputEntries); len(conflicts) > 0 {
		c := conflicts[0]
		return nil, fmt.Errorf("output path conflict: %q claimed by %s and %s",
			c.Path, c.Sources[0], c.Sources[1])
	}

	// Phase 2: SSR (if configured) — must run before output writing so
	// transformed HTML reaches disk (spec §6: Phase 1 → Phase 2 → Phase 3).
	ssrSkipped := cfg.SSR == nil
	if cfg.SSR != nil {
		intermediateHTML := make(map[string]string, len(pages))
		for _, page := range pages {
			if len(page.RenderedBody) > 0 {
				intermediateHTML[page.RelPath] = string(page.RenderedBody)
			}
		}

		finalHTML, err := BuildPhase2(intermediateHTML, cfg.SSR)
		if err != nil {
			return nil, fmt.Errorf("SSR Phase 2: %w", err)
		}

		for _, page := range pages {
			if transformed, ok := finalHTML[page.RelPath]; ok {
				page.RenderedBody = []byte(transformed)
			}
		}
		ssrSkipped = false
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
		// Write alias output paths (same content at additional URLs)
		aliases, err := permalink.ResolveAliases(page)
		if err != nil {
			return nil, fmt.Errorf("resolving aliases for %s: %w", page.RelPath, err)
		}
		if len(aliases) > 0 {
			if err := output.WriteAliases(outputDir, aliases, page.RenderedBody); err != nil {
				return nil, fmt.Errorf("writing aliases for %s: %w", page.RelPath, err)
			}
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

	return &BuildResult{
		OutputDir:     cfg.Build.Output,
		PageCount:     len(pages),
		Duration:      time.Since(start),
		SSRSkipped:    ssrSkipped,
		PagesRendered: rendered,
	}, nil
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

	// Phase 2: SSR (if configured)
	ssrSkipped := cfg.SSR == nil
	if cfg.SSR != nil {
		intermediateHTML := make(map[string]string, len(pages))
		for _, page := range pages {
			if len(page.RenderedBody) > 0 {
				intermediateHTML[page.RelPath] = string(page.RenderedBody)
			}
		}
		finalHTML, err := BuildPhase2(intermediateHTML, cfg.SSR)
		if err != nil {
			return nil, fmt.Errorf("SSR Phase 2: %w", err)
		}
		for _, page := range pages {
			if transformed, ok := finalHTML[page.RelPath]; ok {
				page.RenderedBody = []byte(transformed)
			}
		}
		ssrSkipped = false
	}

	return &BuildResult{
		OutputDir:     cfg.Build.Output,
		PageCount:     len(rendered),
		Duration:      time.Since(start),
		SSRSkipped:    ssrSkipped,
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

		// Protect template tags inside <code> blocks from Liquid processing.
		// After markdown rendering, template tags in code fences are literal text
		// inside <code> elements — escape their braces so Liquid ignores them.
		html = escapeTemplateTagsInCode(html)

		// Step 2: Render template tags with full page/site context.
		if hasTemplateSyntax(html) {
			tc := tmpl.BuildTemplateContext(page, combinedSiteData(cfg, siteData), pages, collectionsCtx, nil, "")
			tc.Content = string(html)
			ctx := tc.ToMap()
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

// processPagination detects pages with pagination: front matter, resolves
// data sources, and generates virtual or paginated pages. Original paginated
// pages are replaced by their expanded set.
func processPagination(pages []*content.Page, cfg *config.Config, siteData map[string]interface{}, collectionsCtx map[string]interface{}) []*content.Page {
	var result []*content.Page
	for _, page := range pages {
		paginationRaw, ok := page.FrontMatter["pagination"]
		if !ok {
			result = append(result, page)
			continue
		}
		paginationMap, ok := paginationRaw.(map[string]interface{})
		if !ok {
			result = append(result, page)
			continue
		}

		dataRef, _ := paginationMap["data"].(string)
		if dataRef == "" {
			result = append(result, page)
			continue
		}

		// Resolve data source — siteData is already the raw data map
		resolved, err := pagination.ResolveDataSource(dataRef, siteData, collectionsCtx)
		if err != nil {
			log.Printf("warning: pagination data source %q: %v", dataRef, err)
			result = append(result, page)
			continue
		}

		perPage := 1
		if pp, ok := paginationMap["perPage"].(int); ok && pp > 0 {
			perPage = pp
		} else if pp, ok := paginationMap["perPage"].(float64); ok && int(pp) > 0 {
			perPage = int(pp)
		}
		asVar, _ := paginationMap["as"].(string)
		if asVar == "" {
			asVar = "item"
		}

		// Check if the page has a Liquid permalink (virtual page generation)
		permalinkStr, _ := page.FrontMatter["permalink"].(string)
		useLiquidPermalink := permalinkStr != "" && strings.Contains(permalinkStr, "{{")

		var contexts []pagination.PaginationContext
		var paths []string

		if useLiquidPermalink && perPage == 1 {
			contexts, paths, err = pagination.PaginateWithLiquidPermalink(resolved, permalinkStr, asVar)
		} else {
			basePath := page.URL
			if basePath == "" {
				basePath = permalink.DefaultFromPath(page.RelPath)
			}
			pathSegment := cfg.Pagination.Path
			contexts, paths, err = pagination.Paginate(resolved, perPage, basePath, pathSegment)
		}
		if err != nil {
			log.Printf("warning: pagination for %s: %v", page.RelPath, err)
			result = append(result, page)
			continue
		}

		// Generate virtual pages from pagination contexts
		for i, pctx := range contexts {
			vp := &content.Page{
				RelPath:     page.RelPath,
				Body:        page.Body,
				FrontMatter: copyFrontMatter(page.FrontMatter),
				Section:     page.Section,
				URL:         paths[i],
				Layout:      page.Layout,
				Kind:        "page",
			}
			// Store pagination context for top-level template injection.
			// Keys prefixed with "_pagination" are hoisted by buildTemplateContext
			// to the top level (not nested under page.*) per spec §1c.
			vp.FrontMatter["_paginationCtx"] = map[string]interface{}{
				"pageNumber":   pctx.PageNumber,
				"totalPages":   pctx.TotalPages,
				"previousPage": pctx.PreviousPage,
				"nextPage":     pctx.NextPage,
				"first":        pctx.First,
				"last":         pctx.Last,
				"items":        pctx.Items,
			}
			vp.FrontMatter["_paginationAs"] = asVar
			// Make the data items available under the 'as' variable name
			if perPage == 1 && len(pctx.Items) == 1 {
				vp.FrontMatter["_paginationData"] = pctx.Items[0]
			} else {
				vp.FrontMatter["_paginationData"] = pctx.Items
			}
			result = append(result, vp)
		}
	}
	return result
}

// copyFrontMatter creates a shallow copy of front matter.
func copyFrontMatter(fm map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(fm))
	for k, v := range fm {
		result[k] = v
	}
	return result
}

// codeBlockPattern matches <code> elements (including those with attributes).
// The non-greedy .*? matches to the first </code>, so nested <code> tags would
// not be handled correctly. This is fine because goldmark does not produce
// nested <code> elements — inline code and fenced code blocks each emit a
// single <code>…</code> pair.
var codeBlockPattern = regexp.MustCompile(`(?s)<code[^>]*>.*?</code>`)

// escapeTemplateTagsInCode replaces {{ }}, {% %} inside <code> elements with
// HTML entities so Liquid won't process them. This preserves template syntax
// examples in code fences for display purposes.
func escapeTemplateTagsInCode(html []byte) []byte {
	return codeBlockPattern.ReplaceAllFunc(html, func(match []byte) []byte {
		s := string(match)
		s = strings.ReplaceAll(s, "{{", "&#123;&#123;")
		s = strings.ReplaceAll(s, "}}", "&#125;&#125;")
		s = strings.ReplaceAll(s, "{%", "&#123;%")
		s = strings.ReplaceAll(s, "%}", "%&#125;")
		return []byte(s)
	})
}

// hasTemplateSyntax checks if content contains Liquid template tags.
func hasTemplateSyntax(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "{{") || strings.Contains(s, "{%")
}

// combinedSiteData builds the site data map expected by BuildTemplateContext,
// combining config-level fields (title, baseURL) with data/ directory files.
func combinedSiteData(cfg *config.Config, siteData map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{
		"title":   cfg.Title,
		"baseURL": cfg.BaseURL,
	}
	if siteData != nil {
		m["data"] = siteData
	}
	return m
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

// buildCollectionsContext builds section collections and includes pre-built
// taxonomies, returning them as a template-friendly map.
func buildCollectionsContext(pages []*content.Page, cfg *config.Config, taxonomies map[string]*collection.TaxonomyCollection) map[string]interface{} {
	result := make(map[string]interface{})

	// Section collections — convert pages to template-friendly maps so
	// Liquid can access fields like {{ post.title }} and {{ post.url }}.
	colls := collection.BuildCollections(pages, cfg.Permalinks)
	for name, coll := range colls {
		items := make([]interface{}, len(coll.Pages))
		for i, p := range coll.Pages {
			items[i] = p.ToTemplateMap()
		}
		result[name] = items
	}

	// Taxonomy collections
	for name, tc := range taxonomies {
		result[name] = tc.Terms
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
