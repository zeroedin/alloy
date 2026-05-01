package pipeline

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/assets"
	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/cascade"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/ssr"
	"github.com/zeroedin/alloy/internal/static"
	tmpl "github.com/zeroedin/alloy/internal/template"
	"github.com/zeroedin/alloy/internal/validation"
)

// activeReporter is the current progress reporter, set via SetReporter.
// Nil means no progress output. Safe for concurrent use since Build is
// single-threaded.
var activeReporter ProgressReporter

// SetReporter sets the progress reporter for subsequent Build/BuildIncremental calls.
// Pass nil to suppress progress output.
func SetReporter(r ProgressReporter) {
	activeReporter = r
}

// report calls a ProgressReporter method if a reporter is active.
func reportStartStage(name string, total int) {
	if activeReporter != nil {
		activeReporter.StartStage(name, total)
	}
}
func reportMessage(text string) {
	if activeReporter != nil {
		activeReporter.Message(text)
	}
}
func reportUpdate(current int, filePath string, elapsed time.Duration) {
	if activeReporter != nil {
		activeReporter.Update(current, filePath, elapsed)
	}
}
func reportEndStage() {
	if activeReporter != nil {
		activeReporter.EndStage()
	}
}
func reportSummary(pageCount int, duration time.Duration, pagesSkipped int) {
	if activeReporter != nil {
		activeReporter.Summary(pageCount, duration, pagesSkipped)
	}
}

// BuildOptions controls optional pipeline behavior.
type BuildOptions struct {
	SkipSSR       bool           // true = skip Phase 2 entirely, regardless of cfg.SSR
	PipelineState *PipelineState // pre-built state to reuse (BuildIncremental only)
	Profile       bool           // true = record per-stage timing in BuildResult.StageTimings
}

// BuildResult holds the outcome of a build.
type BuildResult struct {
	OutputDir           string
	PageCount           int
	PagesSkipped        int // pages skipped via cache (incremental only)
	SSRPagesRendered    int // pages that went through Phase 2 SSR
	Duration            time.Duration
	Errors              []error
	SSRSkipped          bool              // true when Phase 2 was skipped (no ssr: config or SkipSSR)
	PagesRendered       []string          // source paths of pages that were rendered
	RenderedContent     map[string]string // page key → final rendered HTML (RelPath for regular pages, URL for generated pages)
	ContentPassthroughs []string          // relative paths of non-content files copied from content/ to output
	StageTimings        []StageTiming     // per-stage durations (populated when BuildOptions.Profile is true)
}

// RenderContext bundles shared rendering state passed through the render call
// chain, reducing parameter counts on renderPages and related functions.
type RenderContext struct {
	Cfg            *config.Config
	SiteData       map[string]interface{}
	CollectionsCtx map[string]interface{}
	TaxonomiesCtx  map[string]interface{}
	LangContexts   []i18n.LanguageContext
	Pages          []*content.Page
	Engine         tmpl.TemplateEngine
	TemplateUsage  map[string][]string
}

// Build runs the complete build pipeline (Phase 0 through Phase 3).
// Pass BuildOptions to control pipeline behavior (e.g., SkipSSR for dev mode).
func Build(cfg *config.Config, opts ...BuildOptions) (*BuildResult, error) {
	if len(opts) > 1 {
		return nil, fmt.Errorf("accepts at most one BuildOptions value, got %d", len(opts))
	}

	start := time.Now()

	var options BuildOptions
	if len(opts) == 1 {
		options = opts[0]
	}

	config.ApplyDefaults(cfg)

	// Intentional mutation: downstream code (resolveDir, layout resolution,
	// output writing, cache persistence) reads cfg.ProjectRoot pervasively.
	// Threading a local variable would require changing dozens of call sites.
	if cfg.ProjectRoot == "" {
		if wd, err := os.Getwd(); err == nil {
			cfg.ProjectRoot = wd
		}
	}

	var timer *StageTimer
	if options.Profile {
		timer = &StageTimer{}
	}

	// Track which pages use which layouts for cache invalidation
	templateUsage := make(map[string][]string) // page.RelPath → layoutPaths (relative)

	// Plugin system: discover plugins and set up hook registry
	timer.Start("Plugin discovery")
	registry, hooks, pluginWarnings := DiscoverPlugins(cfg)
	for _, w := range pluginWarnings {
		log.Printf("warning: %s", w)
	}
	for _, w := range registry.ConflictWarnings() {
		log.Printf("warning: %s", w)
	}

	// Fire onConfig hook — plugins can mutate config before validation
	if _, err := hooks.RunWithTimeout(plugin.OnConfig, cfg); err != nil {
		return nil, fmt.Errorf("plugin hook onConfig: %w", err)
	}

	// Build output path map for validation hooks
	outputPathMap := map[string]string{
		cfg.Build.Output: "build output",
	}

	// Fire onBeforeValidation hook — plugins can add entries (e.g. _redirects)
	if _, err := hooks.RunWithTimeout(plugin.OnBeforeValidation, outputPathMap); err != nil {
		return nil, fmt.Errorf("plugin hook onBeforeValidation: %w", err)
	}

	// Validate output directory doesn't overlap with managed directories
	if err := validateOutputDir(cfg); err != nil {
		return nil, err
	}

	// Fire onAfterValidation hook — validated manifest (read-only) + data cascade
	if _, err := hooks.RunWithTimeout(plugin.OnAfterValidation, outputPathMap); err != nil {
		return nil, fmt.Errorf("plugin hook onAfterValidation: %w", err)
	}

	// Stage 1: engine + plugins + cascade + site data
	timer.Start("Pipeline init (engine+data)")
	ps, err := InitPipelineState(cfg, registry, hooks)
	if err != nil {
		return nil, err
	}
	engine := ps.Engine
	siteData := ps.SiteData
	permalinkCfg := buildPermalinkCfg(ps, cfg.Permalinks)

	// ═══ Content pipeline ═══

	contentDir := ps.ContentDir
	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	engineName := cfg.Templates.Engine

	// Fetch external data sources and merge into siteData
	timer.Start("External sources")
	if len(cfg.Sources) > 0 {
		if siteData == nil {
			siteData = make(map[string]interface{})
		}
		cacheDir := resolveDir(cfg.ProjectRoot, ".alloy/fetch-cache")
		for name, src := range cfg.Sources {
			var fetched interface{}
			var fetchErr error
			switch src.Type {
			case "rest":
				fetched, fetchErr = fetch.FetchRESTWithRefetch(src.URL, cacheDir, cfg.Refetch)
			case "graphql":
				fetched, fetchErr = fetch.FetchGraphQL(src.Endpoint, src.Query)
			default:
				log.Printf("warning: unknown source type %q for %s", src.Type, name)
				continue
			}
			if fetchErr != nil {
				log.Printf("warning: fetching source %s: %v", name, fetchErr)
				continue
			}
			key := src.As
			if key == "" {
				key = name
			}
			if _, exists := siteData[key]; exists {
				log.Printf("warning: source %q overwrites existing data key %q", name, key)
			}
			siteData[key] = fetched
		}
	}

	if _, err := ps.Hooks.RunWithTimeout(plugin.OnDataFetched, siteData); err != nil {
		return nil, fmt.Errorf("plugin hook onDataFetched: %w", err)
	}

	// Keep PipelineState in sync after sources merge may have replaced the map
	ps.SiteData = siteData

	// Inject site data into plugin runtimes so alloy.data is available
	// during template filter/shortcode calls and post-discovery hooks (issue #339).
	timer.Start("Plugin data injection")
	for _, rt := range registry.Runtimes() {
		if err := rt.SetSiteData(siteData); err != nil {
			log.Printf("warning: setting site data for plugin: %v", err)
		}
	}

	// ═══ Unified two-pass pipeline (issue #280) ═══
	// Always operates on language batches. Single-language sites produce one batch.
	// Pass 1 (steps 3-11): discover + content-render per batch.
	// LinkTranslations between passes (no-op for single batch).
	// Pass 2 (steps 12-15): layout resolution + rendering per batch.

	var langContexts []i18n.LanguageContext
	if len(cfg.Languages) > 0 {
		var langErr error
		langContexts, langErr = i18n.BuildLanguageContexts(cfg.Languages)
		if langErr != nil {
			return nil, fmt.Errorf("i18n setup: %w", langErr)
		}
	} else {
		langContexts = []i18n.LanguageContext{{Code: cfg.Language, Root: true}}
	}
	multiLang := len(langContexts) > 1 || (len(langContexts) == 1 && !langContexts[0].Root)
	langCodes := make([]string, len(langContexts))
	for idx, lc := range langContexts {
		langCodes[idx] = lc.Code
	}

	type langBatch struct {
		ctx            i18n.LanguageContext
		pages          []*content.Page
		collections    map[string]interface{}
		taxonomies     map[string]*collection.TaxonomyCollection
		taxonomiesCtx  map[string]interface{}
	}
	var batches []langBatch
	var pages []*content.Page
	var rendered []string
	var contentPassthroughs []string

	// ── Pass 1a: discover + prepare per batch (steps 3-9) ──
	timer.Start("Pass 1a: discovery+collections")
	reportStartStage("Discovering", -1)
	for _, lc := range langContexts {
		// Content directory: content/<lang>/ for multi-language, content/ for single
		batchContentDir := contentDir
		if multiLang {
			batchContentDir = filepath.Join(contentDir, lc.Code)
		}

		batchPages, batchPassthroughs, err := content.DiscoverWithPassthrough(batchContentDir, cfg.Content.Formats)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("content discovery (%s): %w", lc.Code, err)
		}
		contentPassthroughs = append(contentPassthroughs, batchPassthroughs...)

		// Set lang + prefix RelPath for multi-language translation linking.
		// Single-language builds skip this to avoid injecting i18n context
		// (site.language, lang front matter) that wasn't present before.
		if multiLang {
			for _, page := range batchPages {
				page.FrontMatter["lang"] = lc.Code
				page.RelPath = lc.Code + "/" + page.RelPath
			}
		}

		// Lifecycle filter
		batchPages = content.FilterByLifecycle(batchPages, time.Now(), cfg.IncludeDrafts)

		// Permalink resolution
		prefix := i18n.OutputPrefix(lc.Code, lc.Root)
		langPrefix := lc.Code + "/"
		for _, page := range batchPages {
			if multiLang {
				// Strip lang prefix from RelPath so permalink resolver
				// doesn't double it (e.g., /es/es/about/).
				origRelPath := page.RelPath
				page.RelPath = strings.TrimPrefix(page.RelPath, langPrefix)
				url, err := permalink.ResolveForSection(page, permalinkCfg)
				page.RelPath = origRelPath
				if err != nil {
					return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
				}
				if url == "" || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "//") {
					page.URL = url
					continue
				}
				page.URL = "/" + prefix + strings.TrimPrefix(url, "/")
			} else {
				url, err := permalink.ResolveForSection(page, permalinkCfg)
				if err != nil {
					return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
				}
				page.URL = url
			}
		}

		// Cascade + collections + taxonomies
		bc := applyBatchContext(batchPages, cfg, ps, permalinkCfg)

		// Pagination
		batchPages = processPagination(batchPages, cfg, siteData, bc.Collections, engine)

		pages = append(pages, batchPages...)
		batches = append(batches, langBatch{
			ctx:           lc,
			pages:         batchPages,
			collections:   bc.Collections,
			taxonomies:    bc.Taxonomies,
			taxonomiesCtx: bc.TaxonomiesCtx,
		})
	}
	reportEndStage()

	// For single-language builds, don't pass langContexts to rendering helpers
	// so combinedSiteDataForPage doesn't inject site.language or override site.title.
	var renderLangContexts []i18n.LanguageContext
	if multiLang {
		renderLangContexts = langContexts
	}

	// ── Pass 1b: content rendering per batch (steps 10-11) ──
	timer.Start("Pass 1b: content render")
	reportMessage(fmt.Sprintf("%d pages found", len(pages)))
	reportStartStage("Rendering", len(pages))
	for i := range batches {
		rc := &RenderContext{
			Cfg:            cfg,
			SiteData:       siteData,
			CollectionsCtx: batches[i].collections,
			TaxonomiesCtx:  batches[i].taxonomiesCtx,
			LangContexts:   renderLangContexts,
			Pages:          pages,
			Engine:         engine,
			TemplateUsage:  templateUsage,
		}
		batchRendered, renderErr := renderPages(batches[i].pages, rc)
		if renderErr != nil {
			return nil, renderErr
		}
		rendered = append(rendered, batchRendered...)
	}
	reportEndStage()

	// Early return: no content found → zero pages
	if len(pages) == 0 {
		timer.Stop()
		r := &BuildResult{
			OutputDir:      cfg.Build.Output,
			PageCount:      0,
			Duration:       time.Since(start),
			SSRSkipped:     cfg.SSR == nil || options.SkipSSR,
			StageTimings:   timer.Timings(),
		}
		return r, nil
	}

	// Hooks between passes — all pages discovered and content-rendered
	timer.Start("Inter-pass hooks")
	if _, err := ps.Hooks.RunWithTimeout(plugin.OnContentLoaded, pages); err != nil {
		return nil, fmt.Errorf("plugin hook onContentLoaded: %w", err)
	}
	if _, err := ps.Hooks.RunWithTimeout(plugin.OnDataCascadeReady, pages); err != nil {
		return nil, fmt.Errorf("plugin hook onDataCascadeReady: %w", err)
	}

	// Link translations across all language trees (only for multi-language builds)
	if len(langCodes) > 1 {
		if err := i18n.LinkTranslations(pages, langCodes); err != nil {
			log.Printf("warning: translation linking: %v", err)
		}
	}

	if err := fireContentTransformedHooks(pages, ps.Hooks); err != nil {
		return nil, err
	}

	// ── Pass 2: layout resolution + rendering per batch (steps 12-15) ──
	timer.Start("Pass 2: layout render")
	for _, batch := range batches {
		rc := &RenderContext{
			Cfg:            cfg,
			SiteData:       siteData,
			CollectionsCtx: batch.collections,
			TaxonomiesCtx:  batch.taxonomiesCtx,
			LangContexts:   renderLangContexts,
			Pages:          pages,
			Engine:         engine,
			TemplateUsage:  templateUsage,
		}
		for _, page := range batch.pages {
			layoutPath, err := tmpl.ResolveLayout(page, layoutsDir, engineName, permalinkCfg)
			if err != nil {
				if layoutVal, hasLayout := page.FrontMatter["layout"]; hasLayout && layoutVal != nil {
					log.Printf("warning: layout %v not found for %s: %v", layoutVal, page.RelPath, err)
				}
				continue
			}
			if layoutPath == "" {
				continue
			}

			if err := renderPageThroughLayouts(page, layoutPath, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}

			if err := renderPageFormats(page, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}
		}

		if batch.taxonomies != nil && engine != nil {
			var taxLangCtx *i18n.LanguageContext
			if multiLang {
				taxLangCtx = &batch.ctx
			}
			taxPages, err := generateTaxonomyPages(batch.taxonomies, layoutsDir, engineName, rc, taxLangCtx)
			if err != nil {
				return nil, err
			}
			pages = append(pages, taxPages...)
		}
	}

	// Fire onPageRendered hook per-page with HTML string payload.
	// The hook receives a string and may return string or []byte.
	timer.Start("Post-render hooks")
	for _, page := range pages {
		result, err := ps.Hooks.RunWithTimeout(plugin.OnPageRendered, string(page.RenderedBody))
		if err != nil {
			return nil, fmt.Errorf("plugin hook onPageRendered (%s): %w", page.RelPath, err)
		}
		switch modified := result.(type) {
		case string:
			page.RenderedBody = []byte(modified)
		case []byte:
			page.RenderedBody = modified
		}
	}

	// Pre-build validation: permalink/alias conflicts
	timer.Start("Validation")
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
		// Add validation entries for additional output formats
		for format := range page.FormatBodies {
			fmtPath := formatOutputPath(outPath, format)
			outputEntries = append(outputEntries, validation.OutputPathEntry{
				Path: fmtPath, Source: page.RelPath + " (" + format + ")",
			})
		}
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

	// Phase 2: SSR runs when configured and BuildOptions.SkipSSR is false.
	// Must run before output writing so transformed HTML reaches disk
	// (spec §6: Phase 1 → Phase 2 → Phase 3).
	timer.Start("SSR (Phase 2)")
	ssrSkipped := cfg.SSR == nil || options.SkipSSR
	if cfg.SSR != nil && !options.SkipSSR {
		intermediateHTML := make(map[string]string, len(pages))
		for _, page := range pages {
			if len(page.RenderedBody) > 0 {
				intermediateHTML[page.RelPath] = string(page.RenderedBody)
			}
		}

		finalHTML, err := BuildPhase2(intermediateHTML, cfg.SSR)
		if err != nil {
			return nil, fmt.Errorf("ssr phase 2: %w", err)
		}

		for _, page := range pages {
			if transformed, ok := finalHTML[page.RelPath]; ok {
				page.RenderedBody = []byte(transformed)
			}
		}
		ssrSkipped = false
	}

	// Stage 6: Output writing
	timer.Start("Output writing")
	reportStartStage("Writing", -1)
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
		// Write additional output formats (spec §1e)
		for format, body := range page.FormatBodies {
			fmtPath := formatOutputPath(outPath, format)
			if err := output.WriteFile(outputDir, fmtPath, body); err != nil {
				return nil, fmt.Errorf("writing %s output %s: %w", format, fmtPath, err)
			}
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

	// Fire onAssetProcess hook — plugins can transform assets before copying
	assetInfo := map[string]interface{}{
		"assetsDir": resolveDir(cfg.ProjectRoot, cfg.Structure.Assets),
		"outputDir": outputDir,
	}
	if _, err := ps.Hooks.RunWithTimeout(plugin.OnAssetProcess, assetInfo); err != nil {
		return nil, fmt.Errorf("plugin hook onAssetProcess: %w", err)
	}

	// Stage 7: Static files, assets, and passthrough copy
	timer.Start("Static+asset copy")
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

	// Stage 7b: Copy content-colocated passthrough files
	if len(contentPassthroughs) > 0 {
		for _, relPath := range contentPassthroughs {
			src := filepath.Join(contentDir, relPath)
			dst := filepath.Join(outputDir, relPath)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return nil, fmt.Errorf("copying content passthrough: %w", err)
			}
			srcData, err := os.ReadFile(src)
			if err != nil {
				return nil, fmt.Errorf("copying content passthrough %s: %w", relPath, err)
			}
			if err := os.WriteFile(dst, srcData, 0644); err != nil {
				return nil, fmt.Errorf("copying content passthrough %s: %w", relPath, err)
			}
		}
	}

	// Stage 8: Sitemap generation
	timer.Start("Sitemap")
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
	timer.Start("Cache persistence")
	if cfg.ProjectRoot != "" {
		buildCache := cache.New()
		for _, page := range pages {
			buildCache.SetHash(page.RelPath, cache.HashContent(page.Content))
		}
		// Track template usage for incremental rebuild invalidation
		for pagePath, layoutPaths := range templateUsage {
			for _, layoutPath := range layoutPaths {
				buildCache.TrackTemplateUsage(pagePath, layoutPath)
			}
		}
		cacheDir := resolveDir(cfg.ProjectRoot, ".alloy")
		if err := buildCache.SaveTo(cacheDir); err != nil {
			log.Printf("warning: failed to save build cache: %v", err)
		}
	}

	renderedContent := make(map[string]string, len(pages))
	for _, page := range pages {
		if len(page.RenderedBody) > 0 {
			renderedContent[renderedContentKey(page)] = string(page.RenderedBody)
		}
	}

	timer.Stop()

	result := &BuildResult{
		OutputDir:           cfg.Build.Output,
		PageCount:           len(pages),
		Duration:            time.Since(start),
		SSRSkipped:          ssrSkipped,
		PagesRendered:       rendered,
		RenderedContent:     renderedContent,
		ContentPassthroughs: contentPassthroughs,
		StageTimings:        timer.Timings(),
	}

	reportEndStage()
	reportSummary(result.PageCount, result.Duration, 0)

	// Fire onBuildComplete hook — build is finished, plugins can run post-build tasks
	if _, err := ps.Hooks.RunWithTimeout(plugin.OnBuildComplete, result); err != nil {
		return nil, fmt.Errorf("plugin hook onBuildComplete: %w", err)
	}

	// Log any plugin timeout warnings
	for _, w := range ps.Hooks.Warnings() {
		log.Printf("warning: %s", w)
	}

	return result, nil
}

// BuildWithContent runs the pipeline with injected content for testing.
// The content map keys are source paths, values are raw file content.
func BuildWithContent(cfg *config.Config, contentMap map[string]string, opts ...BuildOptions) (*BuildResult, error) {
	if len(opts) > 1 {
		return nil, fmt.Errorf("accepts at most one BuildOptions value, got %d", len(opts))
	}

	if len(contentMap) == 0 {
		start := time.Now()
		config.ApplyDefaults(cfg)
		skipSSR := cfg.SSR == nil
		if len(opts) > 0 && opts[0].SkipSSR {
			skipSSR = true
		}
		return &BuildResult{
			OutputDir:  cfg.Build.Output,
			PageCount:  0,
			Duration:   time.Since(start),
			SSRSkipped: skipSSR,
		}, nil
	}

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

	cfgCopy := *cfg
	cfgCopy.ProjectRoot = tmpDir
	return Build(&cfgCopy, opts...)
}

// BuildIncremental renders only pages that have changed since the previous
// build (per the cache) or were invalidated by a layout/data change.
// Used by alloy dev for incremental rebuilds on file watcher events.
// When contentMap is nil, content is discovered from the filesystem.
// If previousCache is nil, all pages are rendered (equivalent to full build).
func BuildIncremental(cfg *config.Config, contentMap map[string]string, previousCache *cache.Cache, changedFiles []string, opts ...BuildOptions) (*BuildResult, error) {
	if len(opts) > 1 {
		return nil, fmt.Errorf("accepts at most one BuildOptions value, got %d", len(opts))
	}

	start := time.Now()

	var options BuildOptions
	if len(opts) == 1 {
		options = opts[0]
	}

	config.ApplyDefaults(cfg)

	if contentMap != nil && len(contentMap) == 0 {
		return &BuildResult{
			OutputDir:  cfg.Build.Output,
			PageCount:  0,
			SSRSkipped: cfg.SSR == nil || options.SkipSSR,
			Duration:   time.Since(start),
		}, nil
	}

	var allPages []*content.Page
	var fsMode bool

	if contentMap == nil {
		// Filesystem mode: discover content from the real project directory
		fsMode = true
		contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
		var err error
		allPages, err = content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
		if err != nil {
			if os.IsNotExist(err) {
				return &BuildResult{
					OutputDir:  cfg.Build.Output,
					PageCount:  0,
					SSRSkipped: cfg.SSR == nil || options.SkipSSR,
					Duration:   time.Since(start),
				}, nil
			}
			return nil, fmt.Errorf("content discovery: %w", err)
		}
	} else {
		// Test mode: create temp directory with content files
		tmpDir, err := os.MkdirTemp("", "alloy-incremental-*")
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

		contentDir := filepath.Join(tmpDir, "content")
		allPages, err = content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
		if err != nil {
			return nil, fmt.Errorf("content discovery: %w", err)
		}
	}

	// Determine which pages need rebuilding
	var pagesToRender []*content.Page
	skipped := 0

	if previousCache == nil {
		// No cache — render everything
		pagesToRender = allPages
	} else {
		// Check for layout changes in changedFiles
		layoutsDir := cfg.Structure.Layouts
		if layoutsDir == "" {
			layoutsDir = "layouts"
		}
		layoutPrefix := filepath.ToSlash(layoutsDir) + "/"
		var layoutChanges []string
		for _, f := range changedFiles {
			if strings.HasPrefix(filepath.ToSlash(f), layoutPrefix) {
				layoutChanges = append(layoutChanges, filepath.ToSlash(f))
			}
		}

		// Find pages invalidated by layout changes
		layoutInvalidated := make(map[string]bool)
		for _, layoutPath := range layoutChanges {
			for _, p := range previousCache.InvalidatedPages(layoutPath) {
				layoutInvalidated[p] = true
			}
		}

		for _, page := range allPages {
			// Page invalidated by layout change?
			if layoutInvalidated[page.RelPath] {
				pagesToRender = append(pagesToRender, page)
				continue
			}
			// Content changed?
			var contentBytes []byte
			if fsMode {
				contentBytes = page.Content
			} else {
				contentPrefix := cfg.Structure.Content
				if contentPrefix == "" {
					contentPrefix = "content"
				}
				contentBytes = []byte(contentMap[filepath.ToSlash(filepath.Join(contentPrefix, page.RelPath))])
			}
			if !previousCache.ShouldSkipFile(page.RelPath, contentBytes) {
				pagesToRender = append(pagesToRender, page)
				continue
			}
			// Unchanged — skip
			skipped++
		}
	}

	// Suppress per-page reporter calls for the remainder of this function.
	// Incremental rebuilds are fast and only emit a compact Summary line
	// (spec §259). Restore via defer so all renderPages calls (including
	// on-demand SSR rendering below) are covered.
	savedReporter := activeReporter
	activeReporter = nil
	defer func() { activeReporter = savedReporter }()

	// Reuse caller-provided pipeline state, or initialize from scratch
	ps := options.PipelineState
	if ps == nil {
		registry, hooks, pluginWarnings := DiscoverPlugins(cfg)
		for _, w := range pluginWarnings {
			log.Printf("warning: %s", w)
		}
		var psErr error
		ps, psErr = InitPipelineState(cfg, registry, hooks)
		if psErr != nil {
			return nil, psErr
		}
	}

	// Inject site data into plugin runtimes (issue #339).
	// Skip when PipelineState was provided by the caller — site data was
	// already injected during the initial full build, avoiding repeated
	// JSON serialization on every incremental rebuild.
	if options.PipelineState == nil && ps.Registry != nil {
		for _, rt := range ps.Registry.Runtimes() {
			if err := rt.SetSiteData(ps.SiteData); err != nil {
				log.Printf("warning: setting site data for plugin: %v", err)
			}
		}
	}

	allPages = content.FilterByLifecycle(allPages, time.Now(), cfg.IncludeDrafts)

	// Re-filter pagesToRender against lifecycle-filtered allPages
	filtered := make(map[*content.Page]bool, len(allPages))
	for _, p := range allPages {
		filtered[p] = true
	}
	var filteredToRender []*content.Page
	for _, p := range pagesToRender {
		if filtered[p] {
			filteredToRender = append(filteredToRender, p)
		}
	}
	pagesToRender = filteredToRender

	permalinkCfg := buildPermalinkCfg(ps, cfg.Permalinks)
	for _, page := range allPages {
		url, err := permalink.ResolveForSection(page, permalinkCfg)
		if err != nil {
			return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
		}
		page.URL = url
	}

	bc := applyBatchContext(allPages, cfg, ps, permalinkCfg)

	// Track which RelPaths need rendering before pagination expands them
	renderRelPaths := make(map[string]bool, len(pagesToRender))
	for _, p := range pagesToRender {
		renderRelPaths[p.RelPath] = true
	}

	allPages = processPagination(allPages, cfg, ps.SiteData, bc.Collections, ps.Engine)

	// Rebuild pagesToRender from post-pagination allPages so virtual pages
	// generated from a changed source page are included.
	pagesToRender = nil
	for _, p := range allPages {
		if renderRelPaths[p.RelPath] {
			pagesToRender = append(pagesToRender, p)
		}
	}

	rc := &RenderContext{
		Cfg:            cfg,
		SiteData:       ps.SiteData,
		CollectionsCtx: bc.Collections,
		TaxonomiesCtx:  bc.TaxonomiesCtx,
		Pages:          allPages,
		Engine:         ps.Engine,
	}
	rendered, renderErr := renderPages(pagesToRender, rc)
	if renderErr != nil {
		return nil, renderErr
	}

	renderedContent := make(map[string]string, len(pagesToRender))
	for _, page := range pagesToRender {
		if len(page.RenderedBody) > 0 {
			renderedContent[renderedContentKey(page)] = string(page.RenderedBody)
		}
	}

	// Phase 2: SSR for incremental rebuilds (preview mode)
	ssrSkipped := cfg.SSR == nil || options.SkipSSR
	ssrPagesRendered := 0

	if cfg.SSR != nil && !options.SkipSSR {
		// Collect pages needing SSR: rebuilt pages with custom elements.
		// Scan raw content (not rendered) since goldmark may strip raw HTML.
		contentPrefix := cfg.Structure.Content
		if contentPrefix == "" {
			contentPrefix = "content"
		}
		ssrHTML := make(map[string]string)
		for _, page := range pagesToRender {
			var rawContent string
			if fsMode {
				rawContent = string(page.Content)
			} else {
				rawContent = contentMap[filepath.ToSlash(filepath.Join(contentPrefix, page.RelPath))]
			}
			if tags := ssr.ScanComponents(rawContent); len(tags) > 0 {
				key := renderedContentKey(page)
				if html := renderedContent[key]; html != "" {
					ssrHTML[key] = html
				}
			}
		}

		// Also check for component definition changes — re-SSR pages
		// using changed components even if Phase 1 skipped them
		for _, f := range changedFiles {
			normalized := filepath.ToSlash(f)
			if strings.HasPrefix(normalized, "components/") {
				// Extract component name from path (e.g., "components/ds-card/ds-card.js" → "ds-card")
				parts := strings.SplitN(strings.TrimPrefix(normalized, "components/"), "/", 2)
				componentTag := parts[0]
				if fsMode {
					// Filesystem mode: scan all discovered pages for component usage
					for _, p := range allPages {
						pKey := renderedContentKey(p)
						if _, alreadyQueued := ssrHTML[pKey]; alreadyQueued {
							continue
						}
						if tags := ssr.ScanComponents(string(p.Content)); len(tags) > 0 {
							for _, tag := range tags {
								if tag == componentTag {
									if html := renderedContent[pKey]; html != "" {
										ssrHTML[pKey] = html
									} else {
										onDemand, renderErr := renderPages([]*content.Page{p}, rc)
										if renderErr == nil && len(onDemand) > 0 && len(p.RenderedBody) > 0 {
											ssrHTML[pKey] = string(p.RenderedBody)
										}
									}
									break
								}
							}
						}
					}
				} else {
					// Test mode: scan contentMap for component usage
					contentPrefix := cfg.Structure.Content
					if contentPrefix == "" {
						contentPrefix = "content"
					}
					for path, body := range contentMap {
						relPath := strings.TrimPrefix(path, contentPrefix+"/")
						if tags := ssr.ScanComponents(body); len(tags) > 0 {
							for _, tag := range tags {
								if tag == componentTag {
									for _, p := range allPages {
										if p.RelPath != relPath {
											continue
										}
										pKey := renderedContentKey(p)
										if _, alreadyQueued := ssrHTML[pKey]; alreadyQueued {
											continue
										}
										if html := renderedContent[pKey]; html != "" {
											ssrHTML[pKey] = html
										} else {
											onDemand, renderErr := renderPages([]*content.Page{p}, rc)
											if renderErr == nil && len(onDemand) > 0 && len(p.RenderedBody) > 0 {
												ssrHTML[pKey] = string(p.RenderedBody)
											}
										}
									}
									break
								}
							}
						}
					}
				}
			}
		}

		if len(ssrHTML) > 0 {
			ssrResult, err := BuildPhase2(ssrHTML, cfg.SSR)
			if err != nil {
				log.Printf("warning: incremental SSR failed: %v", err)
			} else {
				for relPath, html := range ssrResult {
					renderedContent[relPath] = html
				}
				ssrPagesRendered = len(ssrResult)
			}
		}
	}

	result := &BuildResult{
		OutputDir:        cfg.Build.Output,
		PageCount:        len(pagesToRender),
		PagesSkipped:     skipped,
		SSRPagesRendered: ssrPagesRendered,
		Duration:         time.Since(start),
		SSRSkipped:       ssrSkipped,
		PagesRendered:    rendered,
		RenderedContent:  renderedContent,
	}

	if savedReporter != nil {
		savedReporter.Summary(result.PageCount, result.Duration, result.PagesSkipped)
	}

	return result, nil
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

	pages = content.FilterByLifecycle(pages, time.Now(), cfg.IncludeDrafts)

	result := make(map[string]string, len(pages))

	mdOpts := content.MarkdownOptions{
		Unsafe:        cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:   cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags:  cfg.Content.Markdown.Goldmark.TemplateTagsValue(),
		AutoHeadingID: cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
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
// from Phase 1. For each page with custom elements, pipes the full page
// HTML to the ssr.command via stdin and reads transformed HTML from stdout.
// Pages without custom elements pass through unchanged.
// Mode "exec" (default): one process per page.
// Mode "stream": persistent process with NUL-delimited messages.
func BuildPhase2(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	if ssrCfg == nil {
		return intermediateHTML, nil
	}

	if ssrCfg.Command == "" {
		return nil, fmt.Errorf("ssr.command is empty")
	}

	// Stream mode: use a persistent process
	if ssrCfg.Mode == "stream" {
		return buildPhase2Stream(intermediateHTML, ssrCfg)
	}

	// Exec mode (default): one process per page
	return buildPhase2Exec(intermediateHTML, ssrCfg)
}

func buildPhase2Exec(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	timeout := 30 * time.Second
	if ssrCfg.Timeout != "" {
		d, err := time.ParseDuration(ssrCfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid ssr.timeout %q: %w", ssrCfg.Timeout, err)
		}
		timeout = d
	}

	result := make(map[string]string, len(intermediateHTML))

	for path, html := range intermediateHTML {
		tags := ssr.ScanComponents(html)
		if len(tags) == 0 {
			result[path] = html
			continue
		}

		// Extract body content — only pipe the body to the SSR command,
		// preserve the document skeleton (DOCTYPE, head, scripts).
		body, before, after := ssr.ExtractBody(html)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rendered, err := ssr.RenderPageWithTimeout(ctx, ssrCfg.Command, body)
		cancel()
		if err != nil {
			log.Printf("warning: SSR failed for %s: %v", path, err)
			result[path] = html
			continue
		}
		result[path] = ssr.ReassembleDocument(before, rendered, after)
	}

	return result, nil
}

func buildPhase2Stream(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	timeout := 30 * time.Second
	if ssrCfg.Timeout != "" {
		d, err := time.ParseDuration(ssrCfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid ssr.timeout %q: %w", ssrCfg.Timeout, err)
		}
		timeout = d
	}

	sr, err := ssr.NewStreamRenderer(ssrCfg.Command)
	if err != nil {
		return nil, fmt.Errorf("ssr stream start %q: %w", ssrCfg.Command, err)
	}
	defer sr.Close()

	result := make(map[string]string, len(intermediateHTML))

	for path, html := range intermediateHTML {
		tags := ssr.ScanComponents(html)
		if len(tags) == 0 {
			result[path] = html
			continue
		}

		body, before, after := ssr.ExtractBody(html)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rendered, err := sr.RenderPageWithTimeout(ctx, body)
		cancel()
		if err != nil {
			if restartErr := sr.Restart(); restartErr != nil {
				log.Printf("warning: SSR stream restart failed for %s: %v", path, restartErr)
				result[path] = html
				continue
			}
			retryCtx, retryCancel := context.WithTimeout(context.Background(), timeout)
			rendered, err = sr.RenderPageWithTimeout(retryCtx, body)
			retryCancel()
			if err != nil {
				log.Printf("warning: SSR stream failed after restart for %s: %v", path, err)
				result[path] = html
				continue
			}
		}
		result[path] = ssr.ReassembleDocument(before, rendered, after)
	}

	return result, nil
}

// renderPages renders all pages through the markdown and template pipeline.
// When engine is non-nil, it is used for template rendering (with custom filters).
// When engine is nil (incremental/SSR-on-demand paths), the standalone
// RenderTemplate is used with strict filters.
func renderPages(pages []*content.Page, rc *RenderContext) ([]string, error) {
	cfg := rc.Cfg
	mdOpts := content.MarkdownOptions{
		Unsafe:        cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:   cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags:  cfg.Content.Markdown.Goldmark.TemplateTagsValue(),
		AutoHeadingID: cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
	}

	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	engineName := cfg.Templates.Engine

	hooks, err := tmpl.DiscoverRenderHooks(layoutsDir, engineName)
	if err != nil {
		return nil, fmt.Errorf("render hook discovery: %w", err)
	}
	if len(hooks) > 0 && rc.Engine != nil {
		mdOpts.Hooks = hooks
		mdOpts.HookRenderer = func(source string, ctx map[string]interface{}) (string, error) {
			tpl, err := rc.Engine.Parse("_markup/hook", []byte(source))
			if err != nil {
				return "", err
			}
			out, err := tpl.Render(ctx)
			if err != nil {
				return "", err
			}
			return string(out), nil
		}
	}

	var rendered []string

	for i, page := range pages {
		var pageStart time.Time
		if activeReporter != nil {
			pageStart = time.Now()
		}
		body := page.Body

		ext := filepath.Ext(page.RelPath)
		var html []byte
		var err error
		switch ext {
		case ".md":
			var toc []content.TOCEntry
			html, toc, err = content.RenderMarkdownWithTOC(body, mdOpts)
			if err == nil {
				page.TOC = toc
			}
		case ".txt":
			html, err = content.RenderText(body)
		default:
			html = body
		}
		if err != nil {
			return nil, fmt.Errorf("content transformation: %s: %w", page.RelPath, err)
		}

		if ext == ".md" {
			html = escapeTemplateTagsInCode(html)
		}

		if hasTemplateSyntax(html) {
			tc := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "")
			tc.Content = string(html)
			ctx := tc.ToMap()
			if page.SourcePath != "" {
				ctx["_contentDir"] = filepath.Dir(page.SourcePath)
				contentRoot := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
				ctx["_contentRoot"] = contentRoot
			}
			if rc.Engine != nil {
				tpl, err := rc.Engine.Parse(page.RelPath, html)
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
		if activeReporter != nil {
			reportUpdate(i+1, page.RelPath, time.Since(pageStart))
		}
	}

	return rendered, nil
}

// processPagination detects pages with pagination: front matter, resolves
// data sources, and generates virtual or paginated pages. Original paginated
// pages are replaced by their expanded set.
func processPagination(pages []*content.Page, cfg *config.Config, siteData map[string]interface{}, collectionsCtx map[string]interface{}, engine tmpl.TemplateEngine) []*content.Page {
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

		// Check if the page has a template permalink (virtual page generation)
		permalinkStr, _ := page.FrontMatter["permalink"].(string)
		useTemplatePermalink := permalinkStr != "" && strings.Contains(permalinkStr, "{{")

		var contexts []pagination.PaginationContext
		var paths []string

		if useTemplatePermalink && perPage == 1 {
			var renderer pagination.TemplateRenderer
			if engine != nil {
				renderer = func(source string, ctx map[string]interface{}) (string, error) {
					tpl, err := engine.Parse("_permalink", []byte(source))
					if err != nil {
						return "", err
					}
					out, err := tpl.Render(ctx)
					if err != nil {
						return "", err
					}
					return string(out), nil
				}
			} else {
				renderer = func(source string, ctx map[string]interface{}) (string, error) {
					return tmpl.RenderTemplate(source, "_permalink", ctx)
				}
			}
			contexts, paths, err = pagination.PaginateWithTemplatePermalink(resolved, permalinkStr, asVar, renderer)
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
				interpolateFrontMatter(vp, asVar, pctx.Items[0], engine)
			} else {
				vp.FrontMatter["_paginationData"] = pctx.Items
			}
			result = append(result, vp)
		}
	}
	return result
}

// renderedContentKey returns the key to use in RenderedContent for a page.
// Regular pages use RelPath; generated pages (taxonomy, paginated) use URL.
func renderedContentKey(page *content.Page) string {
	if _, ok := page.FrontMatter["_paginationCtx"]; ok && page.URL != "" {
		return page.URL
	}
	if page.RelPath == "" && page.URL != "" {
		return page.URL
	}
	return page.RelPath
}

// copyFrontMatter creates a shallow copy of front matter.
func copyFrontMatter(fm map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(fm))
	for k, v := range fm {
		result[k] = v
	}
	return result
}

// interpolateFrontMatter resolves template tags in string-valued front matter
// fields for single-item paginated virtual pages.
func interpolateFrontMatter(vp *content.Page, asVar string, item interface{}, engine tmpl.TemplateEngine) {
	skipKeys := map[string]bool{
		"permalink": true, "layout": true, "pagination": true,
	}
	ctx := map[string]interface{}{asVar: item}
	for k, v := range vp.FrontMatter {
		s, ok := v.(string)
		if !ok || (!strings.Contains(s, "{{") && !strings.Contains(s, "{%")) {
			continue
		}
		if skipKeys[k] || strings.HasPrefix(k, "_pagination") {
			continue
		}
		if engine != nil {
			tpl, err := engine.Parse("_fm_"+k, []byte(s))
			if err != nil {
				log.Printf("warning: front matter interpolation for %s.%s: %v", vp.RelPath, k, err)
				continue
			}
			out, err := tpl.Render(ctx)
			if err != nil {
				log.Printf("warning: front matter interpolation for %s.%s: %v", vp.RelPath, k, err)
				continue
			}
			vp.FrontMatter[k] = html.UnescapeString(strings.TrimSpace(string(out)))
		} else {
			vp.FrontMatter[k] = strings.TrimSpace(pagination.RenderSimpleLiquid(s, asVar, item))
		}
	}
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

// combinedSiteDataForPage returns site data with language-specific overrides
// when i18n is active. Falls back to combinedSiteData for single-language builds.
func combinedSiteDataForPage(cfg *config.Config, siteData map[string]interface{}, langContexts []i18n.LanguageContext, page *content.Page) map[string]interface{} {
	m := combinedSiteData(cfg, siteData)
	if len(langContexts) == 0 || page == nil {
		return m
	}
	langCode, _ := page.FrontMatter["lang"].(string)
	if langCode == "" {
		return m
	}
	for _, lc := range langContexts {
		if lc.Code == langCode {
			m["language"] = i18n.LanguageData(lc)
			m["title"] = i18n.LanguageSiteTitle(cfg.Title, cfg.Languages[langCode])
			break
		}
	}
	return m
}

// formatOutputPath computes the output path for a non-HTML format by replacing
// the .html extension with the format extension (e.g., "blog/post/index.json").
func formatOutputPath(htmlPath string, format string) string {
	return strings.TrimSuffix(htmlPath, ".html") + "." + format
}

// renderPageFormats renders additional output formats for a page (spec §1e).
// For each non-HTML format in page.Outputs, resolves a format-specific layout,
// renders through it, and stores the result in page.FormatBodies.
// Returns a build error if a declared format has no matching layout.
func renderPageFormats(page *content.Page, layoutsDir, engineName string, rc *RenderContext) error {
	if len(page.Outputs) <= 1 {
		return nil
	}
	page.FormatBodies = make(map[string][]byte)
	for _, format := range page.Outputs {
		if format == "html" {
			continue
		}
		fmtLayoutPath, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, engineName, format)
		if err != nil {
			return fmt.Errorf("no %s layout found for %s: %w", format, page.RelPath, err)
		}
		fmtTpl, err := parseLayout(fmtLayoutPath, rc.Engine)
		if err != nil {
			return fmt.Errorf("format layout: %w", err)
		}
		fmtCtx := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(rc.Cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "").ToMap()
		fmtCtx["content"] = string(page.RenderedBody)
		fmtResult, err := fmtTpl.Render(fmtCtx)
		if err != nil {
			return fmt.Errorf("rendering format layout %s: %w", fmtLayoutPath, err)
		}
		page.FormatBodies[format] = fmtResult
	}
	return nil
}

// renderPageThroughLayouts resolves the layout chain for a page, strips front
// matter from each layout, and renders inside-out. Updates page.RenderedBody
// and tracks all layouts in templateUsage for cache invalidation.
func renderPageThroughLayouts(page *content.Page, layoutPath, layoutsDir, engineName string, rc *RenderContext) error {
	chain, err := tmpl.ResolveLayoutChain(layoutPath, layoutsDir, engineName)
	if err != nil {
		return fmt.Errorf("layout chain for %s: %w", page.RelPath, err)
	}

	if rc.TemplateUsage != nil {
		for _, lp := range chain {
			trackedLayout := filepath.ToSlash(filepath.Clean(lp))
			if relLayout, relErr := filepath.Rel(rc.Cfg.ProjectRoot, lp); relErr == nil {
				trackedLayout = filepath.ToSlash(relLayout)
			}
			rc.TemplateUsage[page.RelPath] = append(rc.TemplateUsage[page.RelPath], trackedLayout)
		}
	}

	for _, lp := range chain {
		tpl, err := parseLayout(lp, rc.Engine)
		if err != nil {
			return err
		}

		tc := tmpl.BuildTemplateContext(page, combinedSiteDataForPage(rc.Cfg, rc.SiteData, rc.LangContexts, page), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "")
		ctx := tc.ToMap()
		ctx["content"] = string(page.RenderedBody)
		layoutResult, err := tpl.Render(ctx)
		if err != nil {
			return fmt.Errorf("rendering layout %s: %w", lp, err)
		}
		page.RenderedBody = layoutResult
	}

	return nil
}

func generateTaxonomyPages(taxonomies map[string]*collection.TaxonomyCollection, layoutsDir, engineName string, rc *RenderContext, langCtx *i18n.LanguageContext) ([]*content.Page, error) {
	cfg := rc.Cfg
	for taxName, tc := range taxonomies {
		taxCfg := cfg.Taxonomies[taxName]
		if taxCfg != nil && !taxCfg.ShouldRender() {
			continue
		}
		dupes := collection.DetectDuplicateTermSlugs(tc)
		if len(dupes) > 0 {
			return nil, fmt.Errorf("taxonomy %q has duplicate term slugs: %v", taxName, dupes)
		}
	}

	var urlPrefix string
	var langCode string
	if langCtx != nil {
		urlPrefix = i18n.OutputPrefix(langCtx.Code, langCtx.Root)
		langCode = langCtx.Code
	}

	var result []*content.Page
	for taxName, tc := range taxonomies {
		taxCfg := cfg.Taxonomies[taxName]
		if !taxCfg.ShouldRender() {
			continue
		}
		layoutPath, err := tmpl.ResolveTaxonomyLayout(taxName, taxCfg.Layout, layoutsDir, engineName)
		if err != nil {
			return nil, fmt.Errorf("taxonomy %q layout: %w", taxName, err)
		}
		tpl, err := parseLayout(layoutPath, rc.Engine)
		if err != nil {
			return nil, fmt.Errorf("taxonomy %q layout: %w", taxName, err)
		}

		taxPages := collection.GenerateTaxonomyPages(tc, taxCfg)
		for _, taxPage := range taxPages {
			if langCode != "" {
				taxPage.FrontMatter["lang"] = langCode
				taxPage.URL = "/" + urlPrefix + strings.TrimPrefix(taxPage.URL, "/")
			}

			ctx := tmpl.BuildTemplateContext(taxPage, combinedSiteDataForPage(cfg, rc.SiteData, rc.LangContexts, taxPage), rc.Pages, rc.CollectionsCtx, rc.TaxonomiesCtx, nil, "").ToMap()
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
			if err := renderPageFormats(taxPage, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}
			result = append(result, taxPage)
		}
	}
	return result, nil
}

// loadSiteData loads data files from the configured data directory and
// merges any external file mappings from data.files config.
// Returns an error if an external file is missing or collides with a
// data directory entry.
func loadSiteData(cfg *config.Config) (map[string]interface{}, error) {
	var result map[string]interface{}

	dataDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Data)
	if dataDir != "" {
		loaded, err := data.LoadDirectory(dataDir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Printf("warning: failed to load data directory %s: %v", dataDir, err)
			}
		} else {
			result = loaded
		}
	}

	if len(cfg.Data.Files) > 0 {
		external, err := data.LoadExternalFiles(cfg.Data.Files, cfg.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("external data files: %w", err)
		}
		if len(external) > 0 {
			if result == nil {
				result = make(map[string]interface{})
			}
			sortedKeys := make([]string, 0, len(external))
			for k := range external {
				sortedKeys = append(sortedKeys, k)
			}
			sort.Strings(sortedKeys)
			for _, k := range sortedKeys {
				if _, exists := result[k]; exists {
					return nil, fmt.Errorf("external data files: key %q collides with data directory entry", k)
				}
				result[k] = external[k]
			}
		}
	}

	return result, nil
}

// buildCollectionsContext builds section collections (directory-based),
// returning them as a template-friendly map. Taxonomies are handled
// separately via buildTaxonomiesContext.
func buildCollectionsContext(pages []*content.Page, permalinkCfg map[string]string) map[string]interface{} {
	result := make(map[string]interface{})

	// Section collections — convert pages to template-friendly maps so
	// Liquid can access fields like {{ post.title }} and {{ post.url }}.
	colls := collection.BuildCollections(pages, permalinkCfg)
	for name, coll := range colls {
		items := make([]interface{}, len(coll.Pages))
		for i, p := range coll.Pages {
			items[i] = p.ToTemplateMap()
		}
		result[name] = items
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// buildTaxonomiesContext converts taxonomy collections into a template-friendly
// map for the top-level taxonomies.* namespace. Each term's pages are converted
// via ToTemplateMap() so Liquid can access fields like {{ p.title }}.
func buildTaxonomiesContext(taxonomies map[string]*collection.TaxonomyCollection) map[string]interface{} {
	if len(taxonomies) == 0 {
		return nil
	}
	taxMap := make(map[string]interface{})
	for name, tc := range taxonomies {
		termMap := make(map[string]interface{})
		for term, termPages := range tc.Terms {
			items := make([]interface{}, len(termPages))
			for i, p := range termPages {
				items[i] = p.ToTemplateMap()
			}
			termMap[term] = items
		}
		taxMap[name] = termMap
	}
	if len(taxMap) == 0 {
		return nil
	}
	return taxMap
}

// fireContentTransformedHooks fires onContentTransformed once per page
// with a page object payload containing html, toc, path, url, and frontMatter.
// Applies returned modifications back to page fields.
func fireContentTransformedHooks(pages []*content.Page, hooks *plugin.HookRegistry) error {
	for _, page := range pages {
		payload := map[string]interface{}{
			"html":        string(page.RenderedBody),
			"toc":         serializeTOC(page.TOC),
			"path":        page.RelPath,
			"url":         page.URL,
			"frontMatter": page.FrontMatter,
		}

		result, err := hooks.RunWithTimeout(plugin.OnContentTransformed, payload)
		if err != nil {
			return fmt.Errorf("plugin hook onContentTransformed (%s): %w", page.RelPath, err)
		}

		switch modified := result.(type) {
		case map[string]interface{}:
			if html, ok := modified["html"].(string); ok {
				page.RenderedBody = []byte(html)
			}
			if tocSlice, ok := modified["toc"].([]interface{}); ok {
				page.TOC = deserializeTOC(tocSlice)
			}
			if fm, ok := modified["frontMatter"].(map[string]interface{}); ok {
				page.FrontMatter = fm
			}
		case string:
			page.RenderedBody = []byte(modified)
		case []byte:
			page.RenderedBody = modified
		}
	}
	return nil
}

func serializeTOC(entries []content.TOCEntry) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		m := map[string]interface{}{
			"id":    entry.ID,
			"text":  entry.Text,
			"level": entry.Level,
		}
		if len(entry.Children) > 0 {
			m["children"] = serializeTOC(entry.Children)
		}
		result = append(result, m)
	}
	return result
}

func deserializeTOC(items []interface{}) []content.TOCEntry {
	var entries []content.TOCEntry
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		text, _ := entry["text"].(string)
		level := 0
		switch v := entry["level"].(type) {
		case int:
			level = v
		case float64:
			level = int(v)
		}
		tocEntry := content.TOCEntry{ID: id, Text: text, Level: level}
		if children, ok := entry["children"].([]interface{}); ok {
			tocEntry.Children = deserializeTOC(children)
		}
		entries = append(entries, tocEntry)
	}
	return entries
}

// PipelineState holds shared state initialized once per build.
// Used by both Build() and BuildIncremental() to avoid duplicating setup.
type PipelineState struct {
	Engine      tmpl.TemplateEngine
	Registry    *plugin.Registry
	Hooks       *plugin.HookRegistry
	CascadeData map[string]map[string]interface{}
	SiteData    map[string]interface{}
	ContentDir  string
	ContentBase string
}

// DiscoverPlugins creates a plugin registry and hook system, discovers
// plugins on disk, and loads them into the hook registry.
// Returns warnings for the caller to log (respects --quiet).
func DiscoverPlugins(cfg *config.Config) (*plugin.Registry, *plugin.HookRegistry, []string) {
	hooks := plugin.NewHookRegistry()
	hooks.SetTimeout(cfg.Plugins.Timeout)
	pluginsDir := resolveDir(cfg.ProjectRoot, "plugins")
	registry := plugin.NewRegistry(pluginsDir)
	if cfg.ProjectRoot != "" {
		registry.SetWASMCacheDir(resolveDir(cfg.ProjectRoot, ".alloy/wasm-cache"))
	}
	var warnings []string
	if err := registry.DiscoverPlugins(); err != nil {
		warnings = append(warnings, fmt.Sprintf("plugin discovery: %v", err))
	}
	for _, w := range registry.LoadPlugins(hooks) {
		warnings = append(warnings, w)
	}
	return registry, hooks, warnings
}

// InitPipelineState creates the template engine with plugin extensions,
// loads cascade and site data. Shared by Build() and BuildIncremental().
func InitPipelineState(cfg *config.Config, registry *plugin.Registry, hooks *plugin.HookRegistry) (*PipelineState, error) {
	engine, err := createEngine(cfg)
	if err != nil {
		return nil, err
	}
	if err := registerPluginExtensions(registry, engine); err != nil {
		return nil, err
	}
	if setter, ok := engine.(interface{ SetIncludesDir(string) }); ok {
		setter.SetIncludesDir(resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts))
	}

	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	cascadeData, cascadeErr := cascade.LoadDirectoryCascade(contentDir)
	if cascadeErr != nil {
		log.Printf("warning: loading cascade data: %v", cascadeErr)
	}

	siteData, siteDataErr := loadSiteData(cfg)
	if siteDataErr != nil {
		return nil, siteDataErr
	}

	return &PipelineState{
		Engine:      engine,
		Registry:    registry,
		Hooks:       hooks,
		CascadeData: cascadeData,
		SiteData:    siteData,
		ContentDir:  contentDir,
		ContentBase: filepath.Base(contentDir),
	}, nil
}

// batchContext holds the per-batch pipeline state produced by applyBatchContext.
type batchContext struct {
	Collections   map[string]interface{}
	Taxonomies    map[string]*collection.TaxonomyCollection
	TaxonomiesCtx map[string]interface{}
}

// applyBatchContext applies cascade data, builds collections and taxonomies
// for a set of pages. Used in both Build()'s per-batch loop and BuildIncremental().
func applyBatchContext(pages []*content.Page, cfg *config.Config, ps *PipelineState, permalinkCfg map[string]string) *batchContext {
	for _, page := range pages {
		var dirData map[string]interface{}
		if len(ps.CascadeData) > 0 {
			dirData = cascade.FindCascadeData(ps.CascadeData, ps.ContentBase, page.RelPath)
		}
		pctx := cascade.BuildContext(ps.SiteData, dirData, page.FrontMatter)
		page.FrontMatter = pctx.ToMap()
	}

	bc := &batchContext{
		Collections: buildCollectionsContext(pages, permalinkCfg),
	}
	if cfg.Taxonomies != nil {
		bc.Taxonomies = collection.BuildTaxonomies(pages, cfg.Taxonomies)
		bc.TaxonomiesCtx = buildTaxonomiesContext(bc.Taxonomies)
		if cfg.Verbose {
			for taxName, tc := range bc.Taxonomies {
				totalPages := 0
				for _, termPages := range tc.Terms {
					totalPages += len(termPages)
				}
				log.Printf("taxonomy %q: %d terms, %d page assignments", taxName, len(tc.Terms), totalPages)
			}
		}
	}
	return bc
}

// buildPermalinkCfg builds a section-to-pattern permalink map by extracting
// permalink patterns from cascade _data.yaml, with cfg.Permalinks as fallback.
// Only top-level section directories are extracted (e.g. content/blog/ → "blog"),
// matching ResolveForSection's section-name lookup. Nested _data.yaml permalink
// patterns (e.g. content/blog/2026/) are not extracted — ResolveForSection only
// looks up by section name (first path component).
func buildPermalinkCfg(ps *PipelineState, fallback map[string]string) map[string]string {
	result := make(map[string]string, len(fallback))
	for k, v := range fallback {
		result[k] = v
	}
	for key, data := range ps.CascadeData {
		pattern, ok := data["permalink"].(string)
		if !ok || pattern == "" {
			continue
		}
		trimmed := strings.TrimPrefix(key, ps.ContentBase+"/")
		trimmed = strings.TrimSuffix(trimmed, "/")
		if trimmed == "" {
			result["default"] = pattern
		} else if !strings.Contains(trimmed, "/") {
			result[trimmed] = pattern
		}
	}
	return result
}

func parseLayout(path string, engine tmpl.TemplateEngine) (tmpl.Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading layout %s: %w", path, err)
	}
	stripped := tmpl.StripLayoutFrontMatter(string(content))
	tpl, err := engine.Parse(path, []byte(stripped))
	if err != nil {
		return nil, fmt.Errorf("parsing layout %s: %w", path, err)
	}
	return tpl, nil
}

func createEngine(cfg *config.Config) (tmpl.TemplateEngine, error) {
	var engine tmpl.TemplateEngine
	if cfg.Templates.Engine == "gotemplate" {
		engine = tmpl.NewGoEngine()
	} else {
		engine = tmpl.NewLiquidEngine()
	}
	tmpl.InitMarkdownify(content.MarkdownOptions{
		Unsafe:        cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:   cfg.Content.Markdown.Goldmark.Typographer,
		AutoHeadingID: cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
	})
	if err := tmpl.RegisterBuiltinFilters(engine); err != nil {
		return nil, fmt.Errorf("registering template filters: %w", err)
	}
	tmpl.RegisterInlineTag(engine)
	return engine, nil
}

func registerPluginExtensions(registry *plugin.Registry, engine tmpl.TemplateEngine) error {
	for _, rt := range registry.Runtimes() {
		for _, filterName := range rt.RegisteredFilters() {
			name := filterName
			runtime := rt
			if err := engine.AddFilter(name, func(input interface{}, args ...interface{}) interface{} {
				result, err := runtime.CallFilter(name, input, args...)
				if err != nil {
					return input
				}
				return result
			}); err != nil {
				return fmt.Errorf("registering plugin filter %q: %w", name, err)
			}
		}
		for _, scName := range rt.RegisteredShortcodes() {
			name := scName
			runtime := rt
			if err := engine.AddTag(name, func(args []string, content string) string {
				result, err := runtime.CallShortcode(name, args, content)
				if err != nil {
					return ""
				}
				return result
			}); err != nil {
				return fmt.Errorf("registering plugin shortcode %q: %w", name, err)
			}
		}
	}
	return nil
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
