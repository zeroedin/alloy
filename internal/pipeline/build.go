package pipeline

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"

	"github.com/zeroedin/alloy/internal/assets"
	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/cascade"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/fileutil"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/static"
	tmpl "github.com/zeroedin/alloy/internal/template"
	"github.com/zeroedin/alloy/internal/validation"
)

func reportStartStage(r ProgressReporter, name string, total int) {
	if r != nil {
		r.StartStage(name, total)
	}
}
func reportMessage(r ProgressReporter, text string) {
	if r != nil {
		r.Message(text)
	}
}
func reportUpdate(r ProgressReporter, current int, filePath string, elapsed time.Duration) {
	if r != nil {
		r.Update(current, filePath, elapsed)
	}
}
func reportEndStage(r ProgressReporter) {
	if r != nil {
		r.EndStage()
	}
}
func reportSummary(r ProgressReporter, pageCount int, duration time.Duration, pagesSkipped int) {
	if r != nil {
		r.Summary(pageCount, duration, pagesSkipped)
	}
}

// BuildOptions controls optional pipeline behavior.
type BuildOptions struct {
	SkipSSR       bool               // true = skip Phase 2 entirely, regardless of cfg.SSR
	PipelineState *PipelineState     // pre-built state to reuse (BuildIncremental only)
	Profile       bool               // true = record per-stage timing in BuildResult.StageTimings
	Reporter      ProgressReporter   // progress output; nil = silent
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
	Cache               *cache.Cache      // in-memory cache with content hashes for incremental rebuild (issue #639)
	SiteData            map[string]interface{} // enriched site data (data files + external sources + hooks)
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
	LayoutCache    map[string]tmpl.Template
	Goldmark       goldmark.Markdown
	PermalinkCfg   map[string]string
	Registry       *plugin.Registry
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
	reporter := options.Reporter

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

	// Validate output directory doesn't overlap with managed directories.
	// Must run before any filesystem operations (clean, static copy) to
	// prevent destructive actions on managed dirs like content/ (issue #492).
	if err := validateOutputDir(cfg); err != nil {
		return nil, err
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

	// Deferred cleanup — declared before onConfig so it runs on all exit
	// paths, including onConfig errors. registry.Close() shuts down all
	// runtimes (primary bridges + worker pools).
	var workerPoolReady chan struct{}
	defer func() {
		if workerPoolReady != nil {
			<-workerPoolReady
		}
		registry.Close()
	}()

	// Fire onConfig hook — plugins can mutate config via the mutable allowlist
	// (see PLAN.md §onConfig mutation semantics). Must run before output dir
	// resolution and worker pool spawn.
	if hooks.HasHooks(plugin.OnConfig) {
		result, err := hooks.RunWithTimeout(plugin.OnConfig, cfg)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onConfig: %w", err)
		}
		if err := applyOnConfigResult(cfg, result); err != nil {
			return nil, fmt.Errorf("plugin hook onConfig: %w", err)
		}
		hooks.SetTimeout(cfg.Plugins.Timeout)
	}

	// Spawn worker pools asynchronously so bridge startup overlaps with pipeline init.
	if hooks.HasHooks(plugin.OnPageRendered) {
		workerPoolReady = make(chan struct{})
		go func() {
			defer close(workerPoolReady)
			numWorkers := plugin.ResolveWorkerCount(cfg.Plugins.Workers)
			for _, rt := range registry.Runtimes() {
				if wp, ok := rt.(interface {
					PrepareWorkerPool(int) error
				}); ok {
					if err := wp.PrepareWorkerPool(numWorkers); err != nil {
						log.Printf("warning: worker pool setup: %v", err)
					}
				}
			}
		}()
	}

	// Re-validate output dir after hooks in case a plugin changed Build.Output.
	if err := validateOutputDir(cfg); err != nil {
		return nil, err
	}

	// Output dir creation/cleaning + background static copy (issue #492, #503).
	// Starts after all validation hooks so plugin-mutated paths are respected
	// and validation failures don't leave partial copies as debris.
	outputDir := resolveDir(cfg.ProjectRoot, cfg.Build.Output)
	if cfg.Build.CleanValue() {
		if _, statErr := os.Stat(outputDir); statErr == nil {
			if err := output.CleanOutputDir(outputDir); err != nil {
				return nil, fmt.Errorf("cleaning output directory: %w", err)
			}
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	staticDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Static)
	assetsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Assets)

	// Stage 1: engine + plugins + cascade + site data
	timer.Start("Pipeline init (engine+data)")
	ps, err := InitPipelineState(cfg, registry, hooks)
	if err != nil {
		return nil, err
	}
	engine := ps.Engine
	siteData := ps.SiteData
	permalinkCfg := buildPermalinkCfg(ps)

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
			case "plugin":
				if !cfg.Refetch && src.Cache > 0 {
					if cached, found := fetch.GetCached(src.Plugin, cacheDir, src.Cache); found {
						fetched = cached
						break
					}
				}
				configMap := map[string]interface{}{
					"type":   src.Type,
					"plugin": src.Plugin,
					"cache":  src.Cache,
					"as":     src.As,
				}
				fetched, fetchErr = fetch.FetchPluginSource(src.Plugin, configMap)
				if fetchErr == nil {
					if cacheErr := fetch.SaveCache(src.Plugin, cacheDir, fetched); cacheErr != nil {
						log.Printf("warning: caching plugin source %s: %v", src.Plugin, cacheErr)
					}
				}
			default:
				log.Printf("warning: unknown source type %q for %s", src.Type, name)
				continue
			}
			if fetchErr != nil {
				if src.Type == "plugin" {
					return nil, fmt.Errorf("source %q (plugin %q): %w", name, src.Plugin, fetchErr)
				}
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

	if siteData == nil {
		siteData = make(map[string]interface{})
	}
	dataResult, err := ps.Hooks.RunWithTimeout(plugin.OnDataFetched, siteData)
	if err != nil {
		return nil, fmt.Errorf("plugin hook onDataFetched: %w", err)
	}
	switch modified := dataResult.(type) {
	case map[string]interface{}:
		for k, v := range modified {
			siteData[k] = v
		}
	case *ordered.Map:
		for _, entry := range modified.Entries() {
			siteData[entry.Key] = entry.Value
		}
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
	reportStartStage(reporter, "Discovering", -1)
	// Track filesystem-discovered page RelPaths so virtual pages injected
	// by onPagesReady can be identified and recorded in the build cache
	// for BuildIncremental parity (issue #970).
	discoveredPaths := make(map[string]bool)
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

		plRenderer := newPermalinkRenderer(engine)

		// Permalink resolution — uses per-page cascade data so nested
		// _data.yaml permalink patterns are respected (issue #910).
		prefix := i18n.OutputPrefix(lc.Code, lc.Root)
		langPrefix := lc.Code + "/"
		for _, page := range batchPages {
			if multiLang {
				// Strip lang prefix from RelPath so permalink resolver
				// doesn't double it (e.g., /es/es/about/). The cascade
				// lookup must use the original (un-stripped) RelPath so
				// language-specific _data.yaml entries are found (issue #914).
				origRelPath := page.RelPath
				page.RelPath = strings.TrimPrefix(page.RelPath, langPrefix)
				url, err := resolvePagePermalink(page, ps, plRenderer, origRelPath)
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
				url, err := resolvePagePermalink(page, ps, plRenderer)
				if err != nil {
					return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
				}
				page.URL = url
			}
		}

		// Snapshot discovered pages before onPagesReady can inject virtual pages (issue #970).
		for _, p := range batchPages {
			discoveredPaths[p.RelPath] = true
		}

		// Cascade + onPagesReady + collections + taxonomies
		var bc *batchContext
		batchPages, bc, err = applyBatchContext(batchPages, cfg, ps, permalinkCfg)
		if err != nil {
			return nil, err
		}

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
	reportEndStage(reporter)

	// Pre-build validation: permalink/alias conflicts.
	// Runs after onPagesReady (permalinks, aliases, and output formats are final)
	// but before content rendering so conflicts fail fast without wasted work.
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
		for _, format := range page.Outputs {
			if format == "html" {
				continue
			}
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
	for _, batch := range batches {
		urlPrefix := i18n.OutputPrefix(batch.ctx.Code, batch.ctx.Root)
		for taxName, tc := range batch.taxonomies {
			taxCfg := cfg.Taxonomies[taxName]
			if taxCfg == nil {
				continue
			}
			taxPages := collection.GenerateTaxonomyPages(tc, taxCfg)
			for _, tp := range taxPages {
				taxURL := tp.URL
				if urlPrefix != "" {
					taxURL = "/" + urlPrefix + strings.TrimPrefix(taxURL, "/")
				}
				outPath := output.ComputeOutputPath(taxURL)
				source := "taxonomy:" + taxName
				if tp.Kind == "taxonomy_term" {
					if t, ok := tp.FrontMatter["title"].(string); ok {
						source += "/" + t
					}
				}
				outputEntries = append(outputEntries, validation.OutputPathEntry{
					Path:   outPath,
					Source: source,
				})
			}
		}
	}
	// Fire onBeforeValidation — plugins can register additional output paths
	// (e.g., _redirects for Netlify). Payload: { outputPaths: [...] }.
	// Return: { addOutputs: { "path": "source" } }. Added paths feed into
	// DetectConflicts(). Fires here — after output path computation, after
	// onPagesReady, after taxonomy collection and pagination (issue #975).
	if hooks.HasHooks(plugin.OnBeforeValidation) {
		outputPaths := make([]string, len(outputEntries))
		for i, e := range outputEntries {
			outputPaths[i] = e.Path
		}
		payload := map[string]interface{}{
			"outputPaths": outputPaths,
		}
		err := hooks.RunEachWithTimeout(plugin.OnBeforeValidation,
			func(_ int, _ *plugin.HookScope) interface{} { return payload },
			func(_ int, _ *plugin.HookScope, result interface{}) error {
				resultMap, ok := toGoMap(result)
				if !ok {
					return nil
				}
				for key := range resultMap {
					if key != "addOutputs" {
						return fmt.Errorf("onBeforeValidation returned unrecognized key %q — only \"addOutputs\" is recognized", key)
					}
				}
				addOutputsVal, hasAddOutputs := resultMap["addOutputs"]
				if !hasAddOutputs {
					return nil
				}
				addOutputsMap, ok := toGoMap(addOutputsVal)
				if !ok {
					return fmt.Errorf("onBeforeValidation addOutputs must be a map, got %T", addOutputsVal)
				}
				for path, source := range addOutputsMap {
					outputEntries = append(outputEntries, validation.OutputPathEntry{
						Path:   path,
						Source: fmt.Sprintf("%v", source),
					})
				}
				return nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onBeforeValidation: %w", err)
		}
	}

	if conflicts, _ := validation.DetectConflicts(outputEntries); len(conflicts) > 0 {
		c := conflicts[0]
		return nil, fmt.Errorf("output path conflict: %q claimed by %s and %s",
			c.Path, c.Sources[0], c.Sources[1])
	}

	// Fire onAfterValidation — validated output manifest + mutable cascade.
	// Payload: { outputPaths: [...], cascade: { ...siteData... } }.
	// Return: { cascade: { ... } } merged into siteData. outputPaths changes
	// in the return are ignored. Fires after conflict detection passes (issue #975).
	if hooks.HasHooks(plugin.OnAfterValidation) {
		validatedPaths := make([]string, len(outputEntries))
		for i, e := range outputEntries {
			validatedPaths[i] = e.Path
		}
		err := hooks.RunEachWithTimeout(plugin.OnAfterValidation,
			func(_ int, _ *plugin.HookScope) interface{} {
				return map[string]interface{}{
					"outputPaths": validatedPaths,
					"cascade":     siteData,
				}
			},
			func(_ int, _ *plugin.HookScope, result interface{}) error {
				resultMap, ok := toGoMap(result)
				if !ok {
					return nil
				}
				for key := range resultMap {
					if key != "cascade" && key != "outputPaths" {
						return fmt.Errorf("onAfterValidation returned unrecognized key %q — only \"cascade\" and \"outputPaths\" are recognized", key)
					}
				}
				cascadeVal, hasCascade := resultMap["cascade"]
				if !hasCascade {
					return nil
				}
				cascadeMap, ok := toGoMap(cascadeVal)
				if !ok {
					return fmt.Errorf("onAfterValidation cascade must be a map, got %T", cascadeVal)
				}
				for k, v := range cascadeMap {
					siteData[k] = v
				}
				return nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onAfterValidation: %w", err)
		}
	}

	// For single-language builds, don't pass langContexts to rendering helpers
	// so combinedSiteDataForPage doesn't inject site.language or override site.title.
	var renderLangContexts []i18n.LanguageContext
	if multiLang {
		renderLangContexts = langContexts
	}

	// ── Pass 1b: content rendering per batch (steps 10-11) ──
	timer.Start("Pass 1b: content render")
	reportMessage(reporter, fmt.Sprintf("%d pages found", len(pages)))
	reportStartStage(reporter, "Rendering", len(pages))

	// Create the goldmark instance once for all batches. Hook discovery
	// and options are identical across batches since they come from the
	// same config and engine.
	var sharedGoldmark goldmark.Markdown
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
			Goldmark:       sharedGoldmark,
			PermalinkCfg:   permalinkCfg,
			Registry:       registry,
		}
		batchRendered, renderErr := renderPages(batches[i].pages, rc, reporter)
		if renderErr != nil {
			return nil, renderErr
		}
		rendered = append(rendered, batchRendered...)
		if sharedGoldmark == nil {
			sharedGoldmark = rc.Goldmark
		}
	}
	reportEndStage(reporter)

	// Early return: no content found → zero pages
	// Still copy static/asset files so static-only sites produce output.
	if len(pages) == 0 {
		timer.Start("Static+asset copy")
		if err := static.CopyStatic(staticDir, outputDir); err != nil {
			return nil, fmt.Errorf("copying static files: %w", err)
		}
		if err := assets.ProcessAssets(assetsDir, outputDir, assetHookFn(ps.Hooks)); err != nil {
			return nil, fmt.Errorf("processing asset files: %w", err)
		}
		if len(cfg.Passthrough) > 0 {
			managedDirs := []string{
				cfg.Structure.Content,
				cfg.Structure.Layouts,
				cfg.Structure.Assets,
				cfg.Structure.Static,
				cfg.Structure.Data,
				cfg.Structure.Plugins,
				".alloy",
			}
			if err := static.CopyPassthroughWithValidation(cfg.Passthrough, cfg.ProjectRoot, outputDir, managedDirs); err != nil {
				return nil, fmt.Errorf("copying passthrough files: %w", err)
			}
		}
		timer.Stop()
		r := &BuildResult{
			OutputDir:      cfg.Build.Output,
			PageCount:      0,
			Duration:       time.Since(start),
			SSRSkipped:     cfg.SSR == nil || options.SkipSSR,
			StageTimings:   timer.Timings(),
			SiteData:       siteData,
		}
		return r, nil
	}

	// Hooks between passes — all pages discovered and content-rendered
	timer.Start("Inter-pass hooks")
	if ps.Hooks.HasHooks(plugin.OnContentLoaded) {
		contentScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnContentLoaded))
		contentPayload := serializePagesForHook(pages, contentScope)
		contentResult, err := ps.Hooks.RunWithTimeout(plugin.OnContentLoaded, contentPayload)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onContentLoaded: %w", err)
		}
		if returnedPages, ok := contentResult.([]interface{}); ok {
			inputLen := len(contentPayload)
			if len(returnedPages) > inputLen {
				return nil, fmt.Errorf("plugin hook onContentLoaded: returned %d pages but input had %d — virtual page injection must use onPagesReady instead", len(returnedPages), inputLen)
			}
			if len(returnedPages) < inputLen {
				return nil, fmt.Errorf("plugin hook onContentLoaded: returned %d pages but input had %d — plugins must not remove pages", len(returnedPages), inputLen)
			}
			scopedPaths := make(map[string]bool, len(contentPayload))
			for _, cp := range contentPayload {
				scopedPaths[cp.Path] = true
			}
			pathToIdx := make(map[string]int, len(pages))
			for i, page := range pages {
				pathToIdx[page.RelPath] = i
			}
			for _, rp := range returnedPages {
				if pageMap, ok := toGoMap(rp); ok {
					returnedPath, _ := pageMap["path"].(string)
					if !scopedPaths[returnedPath] {
						continue
					}
					origIdx, found := pathToIdx[returnedPath]
					if !found {
						continue
					}
					if fm, ok := toGoMap(pageMap["frontMatter"]); ok {
						for k, v := range fm {
							pages[origIdx].FrontMatter[k] = v
						}
					}
					if htmlVal, ok := pageMap["html"]; ok {
						if htmlStr, ok := htmlVal.(string); ok {
							pages[origIdx].SetRenderedBody([]byte(htmlStr))
						}
					}
				}
			}
		}
	}
	if ps.Hooks.HasHooks(plugin.OnDataCascadeReady) {
		cascadeScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnDataCascadeReady))
		cascadePayload := serializePagesForCascadeHook(pages, cascadeScope)
		cascadeResult, err := ps.Hooks.RunWithTimeout(plugin.OnDataCascadeReady, cascadePayload)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onDataCascadeReady: %w", err)
		}
		if returnedPages, ok := cascadeResult.([]interface{}); ok {
			inputLen := len(cascadePayload)
			if len(returnedPages) > inputLen {
				return nil, fmt.Errorf("plugin hook onDataCascadeReady: returned %d pages but input had %d — virtual page injection must use onPagesReady instead", len(returnedPages), inputLen)
			}
			if len(returnedPages) < inputLen {
				return nil, fmt.Errorf("plugin hook onDataCascadeReady: returned %d pages but input had %d — plugins must not remove pages", len(returnedPages), inputLen)
			}
			scopedPaths := make(map[string]bool, len(cascadePayload))
			for _, cp := range cascadePayload {
				scopedPaths[cp.Path] = true
			}
			pathToIdx := make(map[string]int, len(pages))
			for i, page := range pages {
				pathToIdx[page.RelPath] = i
			}
			for _, rp := range returnedPages {
				if pageMap, ok := toGoMap(rp); ok {
					if data, ok := toGoMap(pageMap["data"]); ok {
						returnedPath, _ := pageMap["path"].(string)
						if !scopedPaths[returnedPath] {
							continue
						}
						if origIdx, ok := pathToIdx[returnedPath]; ok {
							for k, v := range data {
								pages[origIdx].FrontMatter[k] = v
							}
						}
					}
				}
			}
		}
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
	reportStartStage(reporter, "Layouts", len(pages))
	layoutPageIdx := 0
	layoutCache := make(map[string]tmpl.Template)
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
			LayoutCache:    layoutCache,
			PermalinkCfg:   permalinkCfg,
			Registry:       registry,
		}
		for _, page := range batch.pages {
			var pageStart time.Time
			if reporter != nil {
				pageStart = time.Now()
			}
			layoutPath, err := tmpl.ResolveLayout(page, layoutsDir, engineName, permalinkCfg)
			if err != nil {
				if layoutVal, hasLayout := page.FrontMatter["layout"]; hasLayout && layoutVal != nil {
					log.Printf("warning: layout %v not found for %s: %v", layoutVal, page.RelPath, err)
				}
				layoutPageIdx++
				if reporter != nil {
					reportUpdate(reporter, layoutPageIdx, page.RelPath, time.Since(pageStart))
				}
				continue
			}
			if layoutPath == "" {
				layoutPageIdx++
				if reporter != nil {
					reportUpdate(reporter, layoutPageIdx, page.RelPath, time.Since(pageStart))
				}
				continue
			}

			if err := renderPageThroughLayouts(page, layoutPath, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}

			if err := renderPageFormats(page, layoutsDir, engineName, rc); err != nil {
				return nil, err
			}
			layoutPageIdx++
			if reporter != nil {
				reportUpdate(reporter, layoutPageIdx, page.RelPath, time.Since(pageStart))
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
	reportEndStage(reporter)

	// Wait for worker pool before dispatching hooks.
	if workerPoolReady != nil {
		select {
		case <-workerPoolReady:
		default:
			reportMessage(reporter, "Waiting for plugin workers...")
			<-workerPoolReady
		}
	}

	// Fire onPageRendered hooks with batch dispatch for subprocess plugins.
	// Worker pool distributes pages across multiple subprocesses.
	if ps.Hooks.HasHooks(plugin.OnPageRendered) {
		timer.Start("Post-render hooks")
		reportStartStage(reporter, "Transforms", len(pages))
		payloads := make([]interface{}, len(pages))
		for i, page := range pages {
			payloads[i] = page.HTML()
		}
		var progressFn plugin.BatchProgressFunc
		if reporter != nil {
			var mu sync.Mutex
			var highWater int
			progressFn = func(completed, total int) {
				mu.Lock()
				if completed > highWater {
					highWater = completed
					reportUpdate(reporter, completed, "", 0)
				}
				mu.Unlock()
			}
		}
		results, err := ps.Hooks.RunBatchWithProgress(plugin.OnPageRendered, payloads, progressFn)
		if err != nil {
			return nil, fmt.Errorf("plugin hook onPageRendered: %w", err)
		}
		for i, result := range results {
			switch modified := result.(type) {
			case string:
				pages[i].SetRenderedBody([]byte(modified))
			case []byte:
				pages[i].SetRenderedBody(modified)
			}
		}
		reportEndStage(reporter)
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
				intermediateHTML[renderedContentKey(page)] = page.HTML()
			}
		}

		finalHTML, err := BuildPhase2(intermediateHTML, cfg.SSR)
		if err != nil {
			return nil, fmt.Errorf("ssr phase 2: %w", err)
		}

		for _, page := range pages {
			if transformed, ok := finalHTML[renderedContentKey(page)]; ok {
				page.SetRenderedBody([]byte(transformed))
			}
		}
		ssrSkipped = false
	}

	// Stage 6: Output writing
	timer.Start("Output writing")
	reportStartStage(reporter, "Writing", len(pages))
	dirCache := output.NewDirectoryCache()
	writeIdx := 0
	for _, page := range pages {
		var pageStart time.Time
		if reporter != nil {
			pageStart = time.Now()
		}
		if !output.ShouldWrite(page.URL) {
			writeIdx++
			if reporter != nil {
				reportUpdate(reporter, writeIdx, page.RelPath, time.Since(pageStart))
			}
			continue
		}
		outPath := output.ComputeOutputPath(page.URL)
		if err := output.WriteFileCached(outputDir, outPath, page.RenderedBody, dirCache); err != nil {
			return nil, fmt.Errorf("writing output %s: %w", outPath, err)
		}
		// Write additional output formats (spec §1e)
		for format, body := range page.FormatBodies {
			fmtPath := formatOutputPath(outPath, format)
			if err := output.WriteFileCached(outputDir, fmtPath, body, dirCache); err != nil {
				return nil, fmt.Errorf("writing %s output %s: %w", format, fmtPath, err)
			}
		}
		// Write alias output paths (same content at additional URLs)
		aliases, err := permalink.ResolveAliases(page)
		if err != nil {
			return nil, fmt.Errorf("resolving aliases for %s: %w", page.RelPath, err)
		}
		if len(aliases) > 0 {
			if err := output.WriteAliasesCached(outputDir, aliases, page.RenderedBody, dirCache); err != nil {
				return nil, fmt.Errorf("writing aliases for %s: %w", page.RelPath, err)
			}
		}
		writeIdx++
		if reporter != nil {
			reportUpdate(reporter, writeIdx, page.RelPath, time.Since(pageStart))
		}
	}
	reportEndStage(reporter)

	// Steps 16-18b: Synchronous static/asset/passthrough copy (issue #507).
	// Runs as its own stage after rendering and hooks — no cross-stage overlap.
	timer.Start("Static+asset copy")
	reportStartStage(reporter, "Copying", -1)
	reportMessage(reporter, "Copying static files…")
	if err := static.CopyStatic(staticDir, outputDir); err != nil {
		reportEndStage(reporter)
		return nil, fmt.Errorf("copying static files: %w", err)
	}
	if err := assets.ProcessAssets(assetsDir, outputDir, assetHookFn(ps.Hooks)); err != nil {
		reportEndStage(reporter)
		return nil, fmt.Errorf("processing asset files: %w", err)
	}
	if len(cfg.Passthrough) > 0 {
		managedDirs := []string{
			cfg.Structure.Content,
			cfg.Structure.Layouts,
			cfg.Structure.Assets,
			cfg.Structure.Static,
			cfg.Structure.Data,
			cfg.Structure.Plugins,
			".alloy",
		}
		if err := static.CopyPassthroughWithValidation(cfg.Passthrough, cfg.ProjectRoot, outputDir, managedDirs); err != nil {
			reportEndStage(reporter)
			return nil, fmt.Errorf("copying passthrough files: %w", err)
		}
	}

	// Copy content-colocated passthrough files (depends on content discovery)
	if len(contentPassthroughs) > 0 {
		for _, relPath := range contentPassthroughs {
			src := filepath.Join(contentDir, relPath)
			dst := filepath.Join(outputDir, relPath)
			if err := fileutil.CopyFile(src, dst); err != nil {
				reportEndStage(reporter)
				return nil, fmt.Errorf("copying content passthrough %s: %w", relPath, err)
			}
		}
	}
	reportEndStage(reporter)

	// Stage 8: Sitemap generation
	timer.Start("Sitemap")
	reportStartStage(reporter, "Finalizing", -1)
	if cfg.Sitemap.Enabled && len(pages) > 0 {
		reportMessage(reporter, "Generating sitemap…")
		sitemapXML, err := output.GenerateSitemap(pages, cfg.Sitemap, cfg.BaseURL)
		if err != nil {
			reportEndStage(reporter)
			return nil, fmt.Errorf("generating sitemap: %w", err)
		}
		if err := output.WriteFile(outputDir, "sitemap.xml", sitemapXML); err != nil {
			reportEndStage(reporter)
			return nil, fmt.Errorf("writing sitemap: %w", err)
		}
	}

	// Stage 9: Build in-memory cache for incremental rebuilds
	timer.Start("Cache")
	buildCache := cache.New()
	for _, page := range pages {
		buildCache.SetHash(page.RelPath, cache.HashContent(page.Content))
	}
	for pagePath, layoutPaths := range templateUsage {
		for _, layoutPath := range layoutPaths {
			buildCache.TrackTemplateUsage(pagePath, layoutPath)
		}
	}
	// Track virtual page RelPaths so the first BuildIncremental after
	// a full Build() knows which pages were injected by onPagesReady
	// (issue #970). Build→BuildIncremental cache handoff in cmd/dev.go.
	for _, page := range pages {
		if !discoveredPaths[page.RelPath] {
			buildCache.TrackVirtualPage(page.RelPath)
		}
	}

	renderedContent := make(map[string]string, len(pages))
	for _, page := range pages {
		if len(page.RenderedBody) > 0 {
			renderedContent[renderedContentKey(page)] = page.HTML()
		}
	}
	reportEndStage(reporter)

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
		Cache:               buildCache,
		SiteData:            siteData,
	}

	reportSummary(reporter, result.PageCount, result.Duration, 0)

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
// batchContext holds the per-batch pipeline state produced by applyBatchContext.
type batchContext struct {
	Collections   map[string]interface{}
	Taxonomies    map[string]*collection.TaxonomyCollection
	TaxonomiesCtx map[string]interface{}
}

// applyBatchContext applies cascade data, runs onPagesReady hook for virtual
// page injection, then builds collections and taxonomies for a set of pages.
// Used in both Build()'s per-batch loop and BuildIncremental().
// Returns the updated pages slice (which may have grown via virtual page injection)
// and the batch context.
// applyBatchContext applies data cascade, fires onPagesReady, and builds taxonomy context.
// In multi-language builds this runs once per language batch — plugins receive only that
// language's pages, not the full site. Cross-language virtual page injection is not supported.
func applyBatchContext(pages []*content.Page, cfg *config.Config, ps *PipelineState, permalinkCfg map[string]string) ([]*content.Page, *batchContext, error) {
	for _, page := range pages {
		var dirData map[string]interface{}
		if len(ps.CascadeData) > 0 {
			dirData = cascade.FindCascadeData(ps.CascadeData, ps.ContentBase, page.RelPath)
		}
		pctx := cascade.BuildContext(ps.SiteData, dirData, page.FrontMatter)
		page.FrontMatter = pctx.ToMap()
	}

	if ps.Hooks.HasHooks(plugin.OnPagesReady) {
		var err error
		pages, err = runOnPagesReady(pages, ps)
		if err != nil {
			return nil, nil, err
		}
	}

	bc := &batchContext{
		Collections: buildCollectionsContext(pages, permalinkCfg, collectionNames(cfg)),
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
	return pages, bc, nil
}
// resolvePagePermalink resolves the permalink for a single page using its
// nearest cascade data from _data.yaml. Shared by Build and BuildIncremental.
// cascadeLookupPath overrides the path used for FindCascadeData when the
// page's RelPath has been stripped (e.g., lang prefix removed for token
// resolution) but cascade lookup needs the original path (issue #914).
func resolvePagePermalink(page *content.Page, ps *PipelineState, renderer permalink.PermalinkRenderer, cascadeLookupPath ...string) (string, error) {
	lookupPath := page.RelPath
	if len(cascadeLookupPath) > 0 {
		lookupPath = cascadeLookupPath[0]
	}
	pageCascade := cascade.FindCascadeData(ps.CascadeData, ps.ContentBase, lookupPath)
	return permalink.ResolveFromCascade(page, pageCascade, renderer)
}

// buildPermalinkCfg builds a section-to-pattern permalink map by extracting
// permalink patterns from cascade _data.yaml. Only top-level section
// directories are extracted (e.g. content/blog/ → "blog"). Nested patterns
// are not included — this map is used by buildCollectionsContext (collection
// membership via date-token detection) and ResolveLayout (layout auto-detection),
// which only need section-level granularity. Permalink resolution uses
// per-page cascade data via resolvePagePermalink instead.
func buildPermalinkCfg(ps *PipelineState) map[string]string {
	result := make(map[string]string)
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

// newPermalinkRenderer creates a PermalinkRenderer from the configured template
// engine. When engine is nil, falls back to Liquid-only RenderTemplate.
// Applies html.UnescapeString to output because Go's html/template escapes
// special characters (&, <, >) which corrupts URL paths. Recovers panics
// from malformed templates to return an actionable error.
func newPermalinkRenderer(engine tmpl.TemplateEngine) permalink.PermalinkRenderer {
	if engine != nil {
		return func(source string, ctx map[string]interface{}) (result string, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("template permalink panic: %v", r)
				}
			}()
			tpl, err := engine.Parse("_permalink", []byte(source))
			if err != nil {
				return "", err
			}
			out, err := tpl.Render(ctx)
			if err != nil {
				return "", err
			}
			return html.UnescapeString(string(out)), nil
		}
	}
	return func(source string, ctx map[string]interface{}) (result string, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("template permalink panic: %v", r)
			}
		}()
		return tmpl.RenderTemplate(source, "_permalink", ctx)
	}
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
// managed project directories (content, layouts, assets, static, data,
// plugins, .alloy cache).
func validateOutputDir(cfg *config.Config) error {
	managedDirs := []string{
		cfg.Structure.Content,
		cfg.Structure.Layouts,
		cfg.Structure.Plugins,
		".alloy",
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

// assetHookFn returns a per-asset hook callback for ProcessAssets. If no
// onAssetProcess hooks are registered, returns nil (plain copy). Otherwise each
// asset is dispatched through the hook chain via RunEachWithTimeout — the path
// is preserved across chained hooks so every plugin receives {path, content}
// regardless of what the previous hook returned. Only "content" from the return
// value is applied; path changes are ignored.
func assetHookFn(hooks *plugin.HookRegistry) func(assets.AssetFile) (assets.AssetFile, error) {
	if hooks == nil || !hooks.HasHooks(plugin.OnAssetProcess) {
		return nil
	}
	return func(af assets.AssetFile) (assets.AssetFile, error) {
		currentContent := string(af.Content)

		err := hooks.RunEachWithTimeout(plugin.OnAssetProcess,
			func(i int, scope *plugin.HookScope) interface{} {
				return plugin.HookAssetPayload{
					Path:    af.Path,
					Content: currentContent,
				}
			},
			func(i int, scope *plugin.HookScope, result interface{}) error {
				if newContent, ok := extractAssetContent(result); ok {
					currentContent = newContent
				}
				return nil
			},
		)
		if err != nil {
			return af, fmt.Errorf("plugin hook onAssetProcess: %w", err)
		}
		af.Content = []byte(currentContent)
		return af, nil
	}
}

// extractAssetContent extracts the "content" key from a hook result.
// Handles both map[string]interface{} (QuickJS/WASM) and *ordered.Map (Node bridge).
// Returns ("", false) when the result is nil, not a map, or has no "content" key.
func extractAssetContent(result interface{}) (string, bool) {
	switch m := result.(type) {
	case map[string]interface{}:
		if c, exists := m["content"]; exists {
			if s, ok := c.(string); ok {
				return s, true
			}
			log.Printf("warning: onAssetProcess: content key exists but is %T, not string — original content preserved", c)
		}
	case *ordered.Map:
		if c, exists := m.GetValue("content"); exists {
			if s, ok := c.(string); ok {
				return s, true
			}
			log.Printf("warning: onAssetProcess: content key exists but is %T, not string — original content preserved", c)
		}
	}
	return "", false
}
