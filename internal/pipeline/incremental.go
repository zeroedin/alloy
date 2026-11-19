package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/cascade"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/ssr"
	tmpl "github.com/zeroedin/alloy/internal/template"
	"github.com/zeroedin/alloy/internal/validation"
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
	var incrementalPassthroughs []string
	var fsMode bool

	if contentMap == nil {
		// Filesystem mode: discover content from the real project directory
		fsMode = true
		contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
		var err error
		// Colocated files are discovered here, not just pages: an incremental
		// rebuild must re-check the claims it changes (PLAN.md §Pre-Build
		// Validation, dev mode), and a colocated file claiming a page's output
		// path is the case that silently loses a page (issue #1238).
		allPages, incrementalPassthroughs, err = content.DiscoverWithPassthrough(contentDir, cfg.Content.Formats)
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

	// Front matter hashes of every discovered page, taken before lifecycle
	// filtering and pagination change the page list (issue #1320).
	frontMatterHashes := make(map[string]string, len(allPages))
	for _, page := range allPages {
		frontMatterHashes[page.RelPath] = frontMatterHash(page)
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

		// Find pages invalidated by dependency changes (issue #1100).
		// For each changed file, check if any content pages declared it
		// as a dependency via addDependencies. Dependency-invalidated
		// pages are rebuilt even if their own content hash is unchanged.
		depInvalidated := make(map[string]bool)
		for _, f := range changedFiles {
			depPages := previousCache.PagesForDependency(filepath.ToSlash(f))
			for _, p := range depPages {
				depInvalidated[p] = true
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
				// Page invalidated by dependency change? (issue #1100)
				if depInvalidated[page.RelPath] {
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

	// Pick up _data.yaml edits (issue #1306). alloy dev keeps one
	// PipelineState for the whole session, and InitPipelineState reads the
	// cascade only once. Reading it again on every rebuild means a page always
	// renders with the folder data on disk, also after a full Build in
	// between, which doesn't touch ps. Then re-render every page in the folder
	// of a changed _data.yaml, and below it, since their own content hash is
	// unchanged. After a build that couldn't load the cascade, the pages it
	// rendered had no folder data, so the first rebuild that loads it again
	// renders every page. That build's own result travels in its cache
	// (issue #1324), so a file fixed while it ran still counts as broken.
	cascadeRecovered := false
	loadFailed := ps.cascadeLoadFailed // a fresh state's own read
	pendingInState := false
	if fsMode && options.PipelineState != nil {
		pendingInState = ps.cascadeLoadFailed
		loadFailed = !reloadCascadeData(cfg, ps)
		if loadFailed {
			ps.cascadeLoadFailed = true
		}
	}
	if fsMode {
		prevFailed := previousCache != nil && previousCache.CascadeLoadFailed()
		cascadeRecovered = !loadFailed && (pendingInState || prevFailed)
		if previousCache != nil && !untrackedPartial {
			if cascadeRecovered {
				pagesToRender = allPages
				skipped = 0
			} else {
				var added int
				pagesToRender, added = addPagesUnderDirs(pagesToRender, allPages, changedCascadeDirs(cfg, changedFiles))
				skipped -= added
			}
		}
	}

	// List and tag pages stay current (issue #1320). A page that lists other
	// pages reads their front matter, and nothing records which pages read
	// what. So on a site with a collection or a taxonomy, a front matter
	// change or an added or removed page re-renders every page and
	// regenerates the taxonomy pages, as a full build does.
	listsStale := false
	if fsMode && previousCache != nil && listInputsChanged(previousCache, frontMatterHashes) &&
		(len(cfg.Collections) > 0 || len(cfg.Taxonomies) > 0 ||
			collection.HasCollection(buildPermalinkCfg(ps), collectionNames(cfg))) {
		listsStale = true
		pagesToRender = allPages
		skipped = 0
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
	// Taxonomies per language, as Build groups them (issue #1320). They
	// come from the same pages as bc.Taxonomies, before pagination.
	langTaxonomies, err := taxonomiesByLanguage(cfg, allPages, bc.Taxonomies)
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
		// Cascade recovery renders every page (issue #1306): a plugin may
		// have built this one from pages that had no folder data.
		if cascadeRecovered || listsStale || untrackedPartial || layoutInvalidated[p.RelPath] {
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

	allPages, err = processPagination(allPages, cfg, ps.SiteData, bc.Collections, ps.Engine)
	if err != nil {
		return nil, err
	}

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
		SitePages:      tmpl.NewSitePages(),
	}
	rendered, renderErr := renderPages(pagesToRender, rc, nil)
	if renderErr != nil {
		return nil, renderErr
	}
	// onContentLoaded gets every page's rendered body, as in a full build
	// (issue #1377), so render the bodies of the other pages too. They don't
	// count as rendered: only pagesToRender go on to their layout.
	if ps.Hooks != nil && ps.Hooks.HasHooks(plugin.OnContentLoaded) {
		if _, err := renderPages(pagesNotIn(allPages, pagesToRender), rc, nil); err != nil {
			return nil, err
		}
	}

	// Build in-memory cache early so dependency tracking hooks can populate
	// it. Clone previous cache (preserves skipped page hashes). Hashes and
	// template tracking are added after rendering.
	var buildCache *cache.Cache
	if previousCache != nil {
		buildCache = previousCache.Clone()
	} else {
		buildCache = cache.New()
	}
	// The pages this rebuild renders have folder data only if the cascade
	// loaded (issue #1324). A full render after recovery leaves no page
	// without it, and a failed load keeps the recovery pending.
	buildCache.SetCascadeLoadFailed(loadFailed)
	// Record exactly the pages discovered now, so the next rebuild sees a
	// page added or removed since (issue #1320).
	buildCache.ClearFrontMatterHashes()
	for relPath, h := range frontMatterHashes {
		buildCache.SetFrontMatterHash(relPath, h)
	}

	// Clear content page dependencies only for pages being re-rendered.
	// Hooks will re-track deps for these pages; skipped pages retain their
	// deps from the cloned previous cache (issue #1100).
	for _, page := range pagesToRender {
		buildCache.UntrackPageDependencies(page.RelPath)
	}

	// Lifecycle hooks between passes — mirrors Build() flow (issue #731).
	// Warning-only errors: dev server resilience (Build() returns fatal errors).
	// onContentLoaded and onDataCascadeReady run on every page, as in a full
	// build: each page is read from disk again, so a page they skipped would
	// lose what they add (issue #1377). onContentTransformed changes only a
	// page's own output, so it runs on the pages this rebuild re-renders.
	hookPages := allPages
	if ps.Hooks != nil {
		if ps.Hooks.HasHooks(plugin.OnContentLoaded) {
			contentScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnContentLoaded))
			contentPayload := serializePagesForHook(hookPages, contentScope)
			contentResult, hookErr := ps.Hooks.RunWithTimeout(plugin.OnContentLoaded, contentPayload)
			if hookErr != nil {
				log.Printf("warning: plugin hook onContentLoaded: %v", hookErr)
			} else if returnedPages, ok := contentResult.([]interface{}); ok {
				if len(returnedPages) != len(contentPayload) {
					log.Printf("warning: plugin hook onContentLoaded: returned %d pages but input had %d; ignoring mutations", len(returnedPages), len(contentPayload))
				} else {
					// Each entry goes to the page with its path and url
					// (issue #1391). An entry that can't be placed drops
					// all of this hook's changes, as a count mismatch does.
					matches, matchErr := matchHookEntries(hookPages, pagePayloadKeys(contentPayload), returnedPages)
					if matchErr != nil {
						log.Printf("warning: plugin hook onContentLoaded: %v; ignoring mutations", matchErr)
					} else {
						for n, rp := range returnedPages {
							if matches[n] >= 0 {
								applyContentLoadedEntry(hookPages[matches[n]], rp)
							}
						}
					}
				}
			}
		}
		if ps.Hooks.HasHooks(plugin.OnDataCascadeReady) {
			cascadeScope := computeUnionScope(ps.Hooks.ScopeFor(plugin.OnDataCascadeReady))
			cascadePayload := serializePagesForCascadeHook(hookPages, cascadeScope)
			cascadeResult, hookErr := ps.Hooks.RunWithTimeout(plugin.OnDataCascadeReady, cascadePayload)
			if hookErr != nil {
				log.Printf("warning: plugin hook onDataCascadeReady: %v", hookErr)
			} else if returnedPages, ok := cascadeResult.([]interface{}); ok {
				if len(returnedPages) != len(cascadePayload) {
					log.Printf("warning: plugin hook onDataCascadeReady: returned %d pages but input had %d; ignoring mutations", len(returnedPages), len(cascadePayload))
				} else {
					// Each entry goes to the page with its path and url
					// (issue #1391). An entry that can't be placed drops
					// all of this hook's changes, as a count mismatch does.
					matches, matchErr := matchHookEntries(hookPages, cascadePayloadKeys(cascadePayload), returnedPages)
					if matchErr != nil {
						log.Printf("warning: plugin hook onDataCascadeReady: %v; ignoring mutations", matchErr)
					} else {
						for n, rp := range returnedPages {
							if matches[n] >= 0 {
								applyCascadeEntry(hookPages[matches[n]], rp)
							}
						}
					}
				}
			}
		}
		if err := fireContentTransformedHooks(pagesToRender, ps.Hooks, buildCache); err != nil {
			log.Printf("warning: plugin hook onContentTransformed: %v", err)
		}
	}

	// Pass 2: layout resolution + rendering (issue #628). A new site.pages
	// list for this pass: the hooks above may have changed pages
	// (issue #1360).
	rc.SitePages = tmpl.NewSitePages()
	layoutsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts)
	for _, page := range pagesToRender {
		layoutPath, err := tmpl.ResolveLayout(page, layoutsDir, permalinkCfg)
		if err != nil {
			// A layout the page names (front matter or _data.yaml cascade)
			// that doesn't exist fails the build (issue #1296).
			if namesLayout(page) {
				return nil, err
			}
			continue
		}
		if layoutPath == "" {
			continue
		}
		if err := renderPageThroughLayouts(page, layoutPath, layoutsDir, rc); err != nil {
			return nil, err
		}
		if err := renderPageFormats(page, layoutsDir, rc); err != nil {
			return nil, err
		}
	}

	// Regenerate the taxonomy pages when list pages are stale (issue #1320),
	// or when every page re-renders (cascade recovery, no cache), as Build does
	// after rendering: per language, with that language's URL prefix. They
	// are written with the other pages.
	// With no previous cache every page renders, so the taxonomy pages are
	// generated too, as in a full build.
	noCache := fsMode && previousCache == nil
	var regeneratedTaxonomies []*content.Page
	taxonomiesRegenerated := false
	if (listsStale || cascadeRecovered || noCache) && bc.Taxonomies != nil && ps.Engine != nil {
		taxonomiesRegenerated = true
		for _, lt := range langTaxonomies {
			taxPages, err := generateTaxonomyPages(lt.taxonomies, layoutsDir, rc, lt.ctx)
			if err != nil {
				return nil, err
			}
			regeneratedTaxonomies = append(regeneratedTaxonomies, taxPages...)
		}
		pagesToRender = append(pagesToRender, regeneratedTaxonomies...)
	}

	// Dispatch onPageRendered, onFormatRendered, and single-format writeback.
	// Incremental builds log warnings instead of returning fatal errors.
	_ = dispatchPostRenderHooks(pagesToRender, ps.Hooks, buildCache, nil, func(err error) error {
		log.Printf("warning: %v", err)
		return nil
	})

	needsRenderedMap := options.CaptureRenderedContent || (cfg.SSR != nil && !options.SkipSSR)
	var renderedContent map[string]string
	var formatContent map[string]map[string]string
	if needsRenderedMap {
		renderedContent = make(map[string]string, len(pagesToRender))
		for _, page := range pagesToRender {
			if len(page.RenderedBody) > 0 {
				renderedContent[renderedContentKey(page)] = page.HTML()
			}
		}
	}
	if options.CaptureRenderedContent {
		formatContent = make(map[string]map[string]string)
		for _, page := range pagesToRender {
			if len(page.FormatBodies) > 0 {
				key := renderedContentKey(page)
				formatContent[key] = make(map[string]string, len(page.FormatBodies))
				for format, body := range page.FormatBodies {
					formatContent[key][format] = string(body)
				}
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
			switch {
			case page.RelPath == "" && len(page.Content) == 0:
				// A generated taxonomy page has no source, so scan what its
				// layout rendered (issue #1320).
				rawContent = renderedContent[renderedContentKey(page)]
			case fsMode:
				rawContent = string(page.Content)
			default:
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
	// Taken before writing, which releases the format bodies (issue #1347).
	var taxOutputs []string
	if taxonomiesRegenerated {
		taxOutputs = taxonomyOutputPaths(regeneratedTaxonomies)
	}
	dirCache := output.NewDirectoryCache()
	for _, page := range pagesToRender {
		if !output.ShouldWrite(page.URL) {
			page.ReleaseRenderedBody()
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

	// Populate cache hashes and template tracking for rendered pages.
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

	// Re-check output path claims (issue #1238). A conflict here is reported,
	// not fatal — the dev server surfaces it in the overlay and keeps serving,
	// per PLAN.md's dev-mode rules.
	claimEntries, claimOrigins, claimErr := incrementalClaims(cfg, allPages, incrementalPassthroughs)
	// Taxonomy pages claim their paths too, as in Build: a content page on a
	// term's URL is a conflict, whether or not this rebuild wrote the term.
	for _, lt := range langTaxonomies {
		claimEntries = append(claimEntries, taxonomyClaims(cfg, lt.taxonomies, lt.urlPrefix)...)
	}
	var claimConflicts []error
	// A term that lost its last page gets no page from the regeneration,
	// but its old output is still there, listing the page that left. A full
	// build empties the output folder first, so delete it here (issue #1347),
	// unless something this build claims now writes that path.
	if taxonomiesRegenerated {
		// Without this build's claims (claim collection failed), what is
		// still claimed is unknown, so nothing is deleted. A stale path that
		// isn't deleted stays recorded, so a later rebuild retries it.
		var pending []string
		if previousCache != nil {
			stale := staleTaxonomyOutputs(previousCache.TaxonomyOutputs(), taxOutputs)
			if claimErr == nil {
				pending = removeStaleTaxonomyOutputs(outputDir, stale, claimEntries)
			} else {
				pending = stale
			}
		}
		buildCache.SetTaxonomyOutputs(append(taxOutputs, pending...))
	}
	if claimErr != nil {
		claimConflicts = append(claimConflicts, claimErr)
	} else if conflicts, _ := validation.DetectConflicts(claimEntries); len(conflicts) > 0 {
		claimConflicts = append(claimConflicts, formatConflicts(conflicts))
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
		FormatContent:    formatContent,
		Cache:            buildCache,
		SiteData:         ps.SiteData,
		OutputClaims:     claimEntries,
		CopyOrigins:      claimOrigins,
		Errors:           claimConflicts,
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

	if cascadeRecovered {
		ps.cascadeLoadFailed = false
	}
	return result, nil
}

// incrementalClaims recomputes the full output path claim set for an
// incremental rebuild — pages, their alternate formats and aliases, plus every
// copy source. BuildIncremental copies none of the copy sources itself (the dev
// watcher does), so the claim set it hands back has to describe the whole
// output directory for the watcher's own checks to stay accurate.
func incrementalClaims(cfg *config.Config, pages []*content.Page, contentPassthroughs []string) ([]validation.OutputPathEntry, map[string]string, error) {
	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	contentLabel := validation.SourceDirLabel(cfg.ProjectRoot, contentDir)
	pageSource := func(page *content.Page, suffix string) string {
		label := page.RelPath
		if contentLabel != "" {
			label = contentLabel + "/" + filepath.ToSlash(page.RelPath)
		}
		return label + suffix
	}

	var entries []validation.OutputPathEntry
	for _, page := range pages {
		if !output.ShouldWrite(page.URL) {
			continue
		}
		outPath := output.ComputeOutputPath(page.URL)
		entries = append(entries, validation.OutputPathEntry{
			Path: outPath, Source: pageSource(page, ""),
		})
		for _, format := range page.Outputs {
			if format == "html" {
				continue
			}
			entries = append(entries, validation.OutputPathEntry{
				Path:   formatOutputPath(outPath, format),
				Source: pageSource(page, " ("+format+")"),
			})
		}
		aliases, _ := permalink.ResolveAliases(page)
		for _, alias := range aliases {
			entries = append(entries, validation.OutputPathEntry{
				Path: output.ComputeOutputPath(alias), Source: pageSource(page, " (alias)"),
			})
		}
	}

	if cfg.Sitemap.Enabled && len(pages) > 0 {
		entries = append(entries, validation.OutputPathEntry{
			Path: "sitemap.xml", Source: "sitemap.xml (generated)",
		})
	}

	copyClaims, err := validation.CollectCopyClaims(validation.CopySources{
		ProjectRoot:         cfg.ProjectRoot,
		StaticDir:           resolveDir(cfg.ProjectRoot, cfg.Structure.Static),
		AssetsDir:           resolveDir(cfg.ProjectRoot, cfg.Structure.Assets),
		ContentDir:          contentDir,
		Passthrough:         cfg.Passthrough,
		ManagedDirs:         managedPassthroughDirs(cfg),
		ContentPassthroughs: contentPassthroughs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("collecting output paths: %w", err)
	}
	origins := make(map[string]string, len(copyClaims))
	for _, c := range copyClaims {
		entries = append(entries, c.Entry())
		origins[c.Path] = c.Origin
	}
	return entries, origins, nil
}

// AfterFullBuild brings a reused PipelineState in line with a full Build
// that succeeded (issue #1306). That Build rendered every page, so a
// recovery still pending from an earlier failed incremental reload is no
// longer needed; the state reloads the folder data and keeps a recovery
// pending only if it doesn't load. Whether the Build itself rendered pages
// without folder data travels in its cache (issue #1324), so a file fixed
// while the Build ran still counts as broken. alloy dev calls it after each
// full rebuild, which builds with its own state.
func (ps *PipelineState) AfterFullBuild(cfg *config.Config) {
	ps.cascadeLoadFailed = !reloadCascadeData(cfg, ps)
}

// reloadCascadeData reads every _data.yaml in the content directory again,
// the same way InitPipelineState does, so a rebuild renders with the folder
// data on disk instead of the copy read at startup (issue #1306). A read
// error is a warning and leaves no folder data, as in a fresh build. It
// reports whether the cascade loaded.
func reloadCascadeData(cfg *config.Config, ps *PipelineState) bool {
	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	cascadeData, err := cascade.LoadDirectoryCascade(contentDir)
	if err != nil {
		log.Printf("warning: loading cascade data: %v", err)
	}
	ps.CascadeData = cascadeData
	return err == nil
}

// changedCascadeDirs returns the folders, relative to the content directory
// in slash form, of every _data.yaml or _data.yml in changedFiles. The root
// of the content directory is ".". A deleted file is a changed path too.
// Paths are matched against structure.content, so a content directory with
// any name works (issue #1306).
func changedCascadeDirs(cfg *config.Config, changedFiles []string) []string {
	contentDir := filepath.Clean(resolveDir(cfg.ProjectRoot, cfg.Structure.Content))
	var dirs []string
	for _, f := range changedFiles {
		path := filepath.FromSlash(f)
		if name := filepath.Base(path); name != "_data.yaml" && name != "_data.yml" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(cfg.ProjectRoot, path)
		}
		rel, err := filepath.Rel(contentDir, filepath.Dir(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(rel))
	}
	return dirs
}

// addPagesUnderDirs appends to pagesToRender every page in allPages that sits
// in one of dirs or below it, unless it is already queued. It returns the new
// list and how many pages it added.
func addPagesUnderDirs(pagesToRender, allPages []*content.Page, dirs []string) ([]*content.Page, int) {
	if len(dirs) == 0 {
		return pagesToRender, 0
	}
	queued := make(map[*content.Page]bool, len(pagesToRender))
	for _, p := range pagesToRender {
		queued[p] = true
	}
	added := 0
	for _, page := range allPages {
		if queued[page] {
			continue
		}
		rel := filepath.ToSlash(page.RelPath)
		for _, dir := range dirs {
			if dir == "." || strings.HasPrefix(rel, dir+"/") {
				pagesToRender = append(pagesToRender, page)
				queued[page] = true
				added++
				break
			}
		}
	}
	return pagesToRender, added
}

// frontMatterHash hashes a discovered page's front matter block: the source
// bytes before its body (issue #1320). It reads the source, not the parsed
// map, and runs before pagination, which changes a paginated source page.
func frontMatterHash(page *content.Page) string {
	raw := page.Content
	if len(page.Body) <= len(raw) && bytes.HasSuffix(raw, page.Body) {
		raw = raw[:len(raw)-len(page.Body)]
	}
	return cache.HashContent(raw)
}

// listInputsChanged reports whether what a list page reads has changed since
// the previous build (issue #1320): a page's front matter differs, a page has
// no recorded hash (it is new, or the cache predates front matter hashes), or
// a page with a recorded hash is gone.
func listInputsChanged(previous *cache.Cache, current map[string]string) bool {
	for relPath, h := range current {
		prev, ok := previous.FrontMatterHash(relPath)
		if !ok || prev != h {
			return true
		}
	}
	for _, relPath := range previous.FrontMatterHashPaths() {
		if _, ok := current[relPath]; !ok {
			return true
		}
	}
	return false
}

// languageTaxonomies is one language's taxonomies, with the context and URL
// prefix Build uses for them.
type languageTaxonomies struct {
	ctx        *i18n.LanguageContext
	urlPrefix  string
	taxonomies map[string]*collection.TaxonomyCollection
}

// taxonomiesByLanguage splits the taxonomies by language, as Build's
// per-language batches do (issue #1320): one batch per language whose content
// directory exists. A site with one root language keeps the combined set, with
// no language context, as Build does.
func taxonomiesByLanguage(cfg *config.Config, pages []*content.Page, combined map[string]*collection.TaxonomyCollection) ([]languageTaxonomies, error) {
	if combined == nil {
		return nil, nil
	}
	if len(cfg.Languages) == 0 {
		return []languageTaxonomies{{taxonomies: combined}}, nil
	}
	langContexts, err := i18n.BuildLanguageContexts(cfg.Languages)
	if err != nil {
		return nil, fmt.Errorf("i18n setup: %w", err)
	}
	if len(langContexts) == 1 && langContexts[0].Root {
		return []languageTaxonomies{{taxonomies: combined}}, nil
	}
	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	result := make([]languageTaxonomies, 0, len(langContexts))
	for i := range langContexts {
		lc := &langContexts[i]
		// Build skips a language with no content directory, so it gets no
		// taxonomy pages or claims. An existing empty directory still does.
		if _, statErr := os.Stat(filepath.Join(contentDir, lc.Code)); errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		prefix := lc.Code + "/"
		var langPages []*content.Page
		for _, p := range pages {
			if strings.HasPrefix(p.RelPath, prefix) {
				langPages = append(langPages, p)
			}
		}
		result = append(result, languageTaxonomies{
			ctx:        lc,
			urlPrefix:  i18n.OutputPrefix(lc.Code, lc.Root),
			taxonomies: collection.BuildTaxonomies(langPages, cfg.Taxonomies),
		})
	}
	return result, nil
}

// staleTaxonomyOutputs returns the taxonomy output paths the previous build
// wrote that this rebuild didn't generate (issue #1347).
func staleTaxonomyOutputs(previous, current []string) []string {
	generated := make(map[string]bool, len(current))
	for _, p := range current {
		generated[p] = true
	}
	var stale []string
	for _, p := range previous {
		if !generated[p] {
			stale = append(stale, p)
		}
	}
	return stale
}

// removeStaleTaxonomyOutputs deletes each stale taxonomy output path, and its
// folder if that leaves it empty (issue #1347). A path another claim now holds
// is left alone and forgotten: it isn't a taxonomy page's output any more. It
// returns the paths it couldn't delete, to retry on a later rebuild.
func removeStaleTaxonomyOutputs(outputDir string, stale []string, claims []validation.OutputPathEntry) []string {
	claimed := make(map[string]bool, len(claims))
	for _, c := range claims {
		claimed[c.Path] = true
	}
	var pending []string
	for _, rel := range stale {
		if claimed[rel] {
			continue
		}
		path := filepath.Join(outputDir, filepath.FromSlash(rel))
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("warning: removing stale taxonomy page %s: %v", rel, err)
			pending = append(pending, rel)
			continue
		}
		if dir := filepath.Dir(path); dir != filepath.Clean(outputDir) {
			_ = os.Remove(dir) // only succeeds when the folder is now empty
		}
	}
	return pending
}

// pagesNotIn returns the pages of all that aren't in some, in order.
func pagesNotIn(all, some []*content.Page) []*content.Page {
	skip := make(map[*content.Page]bool, len(some))
	for _, p := range some {
		skip[p] = true
	}
	var rest []*content.Page
	for _, p := range all {
		if !skip[p] {
			rest = append(rest, p)
		}
	}
	return rest
}
