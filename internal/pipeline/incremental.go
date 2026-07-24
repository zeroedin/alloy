package pipeline

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/ssr"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

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
	reporter := options.Reporter

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
	layoutInvalidated := make(map[string]bool)
	untrackedPartial := false

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
		for _, layoutPath := range layoutChanges {
			pages := previousCache.InvalidatedPages(layoutPath)
			if pages == nil {
				untrackedPartial = true
			}
			for _, p := range pages {
				layoutInvalidated[p] = true
			}
		}

		if untrackedPartial {
			pagesToRender = allPages
		} else {
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
	}

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

	// Reload site data when data directory files have changed (issue #717).
	// The full Build() path loads data anew each invocation; the incremental
	// path reuses PipelineState from server startup, so data edits are
	// invisible to processPagination unless we reload here.
	dataDir := cfg.Structure.Data
	if dataDir == "" {
		dataDir = "data"
	}
	dataPrefix := filepath.ToSlash(dataDir) + "/"
	hasDataChange := false
	for _, f := range changedFiles {
		if strings.HasPrefix(filepath.ToSlash(f), dataPrefix) {
			hasDataChange = true
			break
		}
	}
	if hasDataChange {
		freshData, err := loadSiteData(cfg)
		if err != nil {
			log.Printf("warning: reloading site data: %v", err)
		} else if freshData != nil {
			ps.SiteData = freshData
			if ps.Hooks != nil && ps.Hooks.HasHooks(plugin.OnDataFetched) {
				dataResult, hookErr := ps.Hooks.RunWithTimeout(plugin.OnDataFetched, ps.SiteData)
				if hookErr != nil {
					log.Printf("warning: plugin hook onDataFetched after data reload: %v", hookErr)
				} else {
					switch modified := dataResult.(type) {
					case map[string]interface{}:
						for k, v := range modified {
							ps.SiteData[k] = v
						}
					case *ordered.Map:
						for _, entry := range modified.Entries() {
							ps.SiteData[entry.Key] = entry.Value
						}
					}
				}
			}
			if ps.Registry != nil {
				for _, rt := range ps.Registry.Runtimes() {
					if err := rt.SetSiteData(ps.SiteData); err != nil {
						log.Printf("warning: updating plugin site data after reload: %v", err)
					}
				}
			}
		} else {
			// freshData is nil with no error: the data directory was deleted
			// or is empty. Check whether it still exists to disambiguate.
			dataDirAbs := resolveDir(cfg.ProjectRoot, dataDir)
			if _, statErr := os.Stat(dataDirAbs); os.IsNotExist(statErr) {
				ps.SiteData = nil
			}
			// Directory exists but contains no recognized data files;
			// keep stale data so pagination doesn't break.
		}
		// Invalidate paginated pages that reference site.data — their content
		// hash is unchanged but their data source has, so they must re-render.
		for _, page := range allPages {
			if paginationRaw, ok := page.FrontMatter["pagination"]; ok {
				if pm, ok := paginationRaw.(map[string]interface{}); ok {
					if dataRef, ok := pm["data"].(string); ok && strings.HasPrefix(dataRef, "site.data.") {
						pagesToRender = append(pagesToRender, page)
					}
				}
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

	permalinkCfg := buildPermalinkCfg(ps)

	plRenderer := newPermalinkRenderer(ps.Engine)

	for _, page := range allPages {
		url, err := resolvePagePermalink(page, ps, plRenderer)
		if err != nil {
			return nil, fmt.Errorf("permalink resolution: %s: %w", page.RelPath, err)
		}
		page.URL = url
	}

	// Save discovered page RelPaths before onPagesReady can inject virtual
	// pages. Used after applyBatchContext to identify which pages are virtual.
	discoveredPaths := make(map[string]bool, len(allPages))
	for _, p := range allPages {
		discoveredPaths[p.RelPath] = true
	}

	allPages, bc, err := applyBatchContext(allPages, cfg, ps, permalinkCfg)
	if err != nil {
		return nil, err
	}

	// Track which RelPaths need rendering before pagination expands them
	renderRelPaths := make(map[string]bool, len(pagesToRender))
	for _, p := range pagesToRender {
		renderRelPaths[p.RelPath] = true
	}

	// Add virtual pages to renderRelPaths with dependency awareness
	// (issue #1058). On initial builds (nil cache), render all virtual
	// pages. On incremental rebuilds, consult each virtual page's
	// Dependencies field:
	//   - nil Dependencies → always render (safe fallback; unknown deps)
	//   - empty Dependencies → skip (tracked, no file deps to invalidate)
	//   - non-empty Dependencies → render only if a dep is in changedFiles
	// Also re-render when a layout change invalidates the page, or when
	// the page is new (not in previous cache).
	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[filepath.ToSlash(f)] = true
	}
	previousVirtualSet := make(map[string]bool)
	if previousCache != nil {
		for _, relPath := range previousCache.VirtualPagePaths() {
			previousVirtualSet[relPath] = true
		}
	}
	for _, p := range allPages {
		if discoveredPaths[p.RelPath] {
			continue
		}
		if previousCache == nil {
			renderRelPaths[p.RelPath] = true
			continue
		}
		if !previousVirtualSet[p.RelPath] {
			renderRelPaths[p.RelPath] = true
			continue
		}
		if untrackedPartial || layoutInvalidated[p.RelPath] {
			renderRelPaths[p.RelPath] = true
			continue
		}
		if p.Dependencies == nil {
			renderRelPaths[p.RelPath] = true
			continue
		}
		for _, dep := range p.Dependencies {
			if changedSet[dep] {
				renderRelPaths[p.RelPath] = true
				break
			}
		}
	}

	// Capture content hashes before pagination — virtual pages have nil
	// Content, so the cache must use the original discovered page's bytes.
	prePageContentHash := make(map[string]string, len(allPages))
	for _, p := range allPages {
		if len(p.Content) > 0 {
			prePageContentHash[p.RelPath] = cache.HashContent(p.Content)
		}
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

	templateUsage := make(map[string][]string)
	rc := &RenderContext{
		Cfg:            cfg,
		SiteData:       ps.SiteData,
		CollectionsCtx: bc.Collections,
		TaxonomiesCtx:  bc.TaxonomiesCtx,
		Pages:          allPages,
		Engine:         ps.Engine,
		TemplateUsage:  templateUsage,
		LayoutCache:    make(map[string]tmpl.Template),
		PermalinkCfg:   permalinkCfg,
		Registry:       ps.Registry,
	}
	rendered, renderErr := renderPages(pagesToRender, rc, nil)
	if renderErr != nil {
		return nil, renderErr
	}

	// Lifecycle hooks between passes — mirrors Build() flow (issue #731).
	// Warning-only errors: dev server resilience (Build() returns fatal errors).
	if ps.Hooks != nil {
		if ps.Hooks.HasHooks(plugin.OnContentLoaded) {
			contentScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnContentLoaded))
			contentPayload := serializePagesForHook(pagesToRender, contentScope)
			contentResult, hookErr := ps.Hooks.RunWithTimeout(plugin.OnContentLoaded, contentPayload)
			if hookErr != nil {
				log.Printf("warning: plugin hook onContentLoaded: %v", hookErr)
			} else if returnedPages, ok := contentResult.([]interface{}); ok {
				if len(returnedPages) != len(contentPayload) {
					log.Printf("warning: plugin hook onContentLoaded: returned %d pages but input had %d; ignoring mutations", len(returnedPages), len(contentPayload))
				} else {
					scopedPaths := make(map[string]bool, len(contentPayload))
					for _, cp := range contentPayload {
						scopedPaths[cp.Path] = true
					}
					pathToIdx := make(map[string]int, len(pagesToRender))
					for i, page := range pagesToRender {
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
									pagesToRender[origIdx].FrontMatter[k] = v
								}
							}
							if htmlVal, ok := pageMap["html"]; ok {
								if htmlStr, ok := htmlVal.(string); ok {
									pagesToRender[origIdx].SetRenderedBody([]byte(htmlStr))
								}
							}
						}
					}
				}
			}
		}
		if ps.Hooks.HasHooks(plugin.OnDataCascadeReady) {
			cascadeScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnDataCascadeReady))
			cascadePayload := serializePagesForCascadeHook(pagesToRender, cascadeScope)
			cascadeResult, hookErr := ps.Hooks.RunWithTimeout(plugin.OnDataCascadeReady, cascadePayload)
			if hookErr != nil {
				log.Printf("warning: plugin hook onDataCascadeReady: %v", hookErr)
			} else if returnedPages, ok := cascadeResult.([]interface{}); ok {
				if len(returnedPages) != len(cascadePayload) {
					log.Printf("warning: plugin hook onDataCascadeReady: returned %d pages but input had %d; ignoring mutations", len(returnedPages), len(cascadePayload))
				} else {
					scopedPaths := make(map[string]bool, len(cascadePayload))
					for _, cp := range cascadePayload {
						scopedPaths[cp.Path] = true
					}
					pathToIdx := make(map[string]int, len(pagesToRender))
					for i, page := range pagesToRender {
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
										pagesToRender[origIdx].FrontMatter[k] = v
									}
								}
							}
						}
					}
				}
			}
		}
		if err := fireContentTransformedHooks(pagesToRender, ps.Hooks); err != nil {
			log.Printf("warning: plugin hook onContentTransformed: %v", err)
		}
	}

	// Pass 2: layout resolution + rendering (issue #628)
	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	engineName := cfg.Templates.Engine
	for _, page := range pagesToRender {
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

	if ps.Hooks != nil && ps.Hooks.HasHooks(plugin.OnPageRendered) {
		payloads := make([]interface{}, len(pagesToRender))
		for i, page := range pagesToRender {
			payloads[i] = buildPageRenderedPayload(page)
		}
		results, hookErr := ps.Hooks.RunBatchWithProgress(plugin.OnPageRendered, payloads, nil)
		if hookErr != nil {
			log.Printf("warning: plugin hook onPageRendered: %v", hookErr)
		} else {
			for i, result := range results {
				if html, ok := extractPageRenderedHTML(result); ok {
					pagesToRender[i].SetRenderedBody([]byte(html))
				} else if result != nil {
					log.Printf("warning: onPageRendered result for %s: expected page object with html key, got %T — plugin may need migration to the object API", pagesToRender[i].RelPath, result)
				}
			}
		}
	}

	needsRenderedMap := options.CaptureRenderedContent || (cfg.SSR != nil && !options.SkipSSR)
	var renderedContent map[string]string
	if needsRenderedMap {
		renderedContent = make(map[string]string, len(pagesToRender))
		for _, page := range pagesToRender {
			if len(page.RenderedBody) > 0 {
				renderedContent[renderedContentKey(page)] = page.HTML()
			}
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
		componentsDir := cfg.Structure.Components
		if componentsDir == "" {
			componentsDir = "components"
		}
		componentsDirPrefix := filepath.ToSlash(componentsDir) + "/"
		for _, f := range changedFiles {
			normalized := filepath.ToSlash(f)
			if strings.HasPrefix(normalized, componentsDirPrefix) {
				parts := strings.SplitN(strings.TrimPrefix(normalized, componentsDirPrefix), "/", 2)
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
										onDemand, renderErr := renderPages([]*content.Page{p}, rc, nil)
										if renderErr == nil && len(onDemand) > 0 && len(p.RenderedBody) > 0 {
											ssrHTML[pKey] = p.HTML()
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
											onDemand, renderErr := renderPages([]*content.Page{p}, rc, nil)
											if renderErr == nil && len(onDemand) > 0 && len(p.RenderedBody) > 0 {
												ssrHTML[pKey] = p.HTML()
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
				for _, page := range pagesToRender {
					if transformed, ok := ssrResult[renderedContentKey(page)]; ok {
						page.SetRenderedBody([]byte(transformed))
					}
				}
				for relPath, html := range ssrResult {
					renderedContent[relPath] = html
				}
				ssrPagesRendered = len(ssrResult)
			}
		}
	}

	if err := validateOutputDir(cfg); err != nil {
		return nil, err
	}
	outputDir := resolveDir(cfg.ProjectRoot, cfg.Build.Output)
	dirCache := output.NewDirectoryCache()
	for _, page := range pagesToRender {
		if !output.ShouldWrite(page.URL) {
			continue
		}
		outPath := output.ComputeOutputPath(page.URL)
		if err := output.WriteFileCached(outputDir, outPath, page.RenderedBody, dirCache); err != nil {
			return nil, fmt.Errorf("writing output %s: %w", outPath, err)
		}
		for format, fmtBody := range page.FormatBodies {
			fmtPath := formatOutputPath(outPath, format)
			if err := output.WriteFileCached(outputDir, fmtPath, fmtBody, dirCache); err != nil {
				return nil, fmt.Errorf("writing %s output %s: %w", format, fmtPath, err)
			}
		}
		aliases, aliasErr := permalink.ResolveAliases(page)
		if aliasErr != nil {
			return nil, fmt.Errorf("resolving aliases for %s: %w", page.RelPath, aliasErr)
		}
		if len(aliases) > 0 {
			if err := output.WriteAliasesCached(outputDir, aliases, page.RenderedBody, dirCache); err != nil {
				return nil, fmt.Errorf("writing aliases for %s: %w", page.RelPath, err)
			}
		}
		page.ReleaseRenderedBody()
	}

	// Build in-memory cache: clone previous (preserves skipped page hashes),
	// then update only rendered pages + carry forward template tracking.
	var buildCache *cache.Cache
	if previousCache != nil {
		buildCache = previousCache.Clone()
	} else {
		buildCache = cache.New()
	}
	for _, page := range pagesToRender {
		if h, ok := prePageContentHash[page.RelPath]; ok {
			buildCache.SetHash(page.RelPath, h)
		} else if len(page.Content) > 0 {
			buildCache.SetHash(page.RelPath, cache.HashContent(page.Content))
		}
	}
	for pagePath, layoutPaths := range templateUsage {
		for _, layoutPath := range layoutPaths {
			buildCache.TrackTemplateUsage(pagePath, layoutPath)
		}
	}

	// Track virtual page RelPaths and dependencies in the cache so the
	// next incremental rebuild can do selective rendering (issues #970, #1058).
	// Clear stale tracking from the cloned previous cache first.
	buildCache.ClearVirtualPages()
	trackVirtualPages(buildCache, allPages, discoveredPaths)

	var capturedContent map[string]string
	if options.CaptureRenderedContent {
		capturedContent = renderedContent
	}

	result := &BuildResult{
		OutputDir:        cfg.Build.Output,
		PageCount:        len(pagesToRender),
		PagesSkipped:     skipped,
		SSRPagesRendered: ssrPagesRendered,
		Duration:         time.Since(start),
		SSRSkipped:       ssrSkipped,
		PagesRendered:    rendered,
		RenderedContent:  capturedContent,
		Cache:            buildCache,
		SiteData:         ps.SiteData,
	}

	reportSummary(reporter, result.PageCount, result.Duration, result.PagesSkipped)

	if ps.Hooks != nil && ps.Hooks.HasHooks(plugin.OnBuildComplete) {
		if _, hookErr := ps.Hooks.RunWithTimeout(plugin.OnBuildComplete, buildCompletePayload(result)); hookErr != nil {
			log.Printf("warning: plugin hook onBuildComplete: %v", hookErr)
		}
	}
	if ps.Hooks != nil {
		for _, w := range ps.Hooks.Warnings() {
			log.Printf("warning: %s", w)
		}
	}

	return result, nil
}
