# Alloy SSG: Red-to-Green Implementation Plan

## Context

Alloy is a Go-based static site generator with a comprehensive spec (PLAN.md) and 605 failing TDD tests across 21 packages. All implementation files are stubs returning `ErrNotImplemented`. Types, interfaces, and function signatures are already defined. Tests are spec-derived and **must not be modified** — the implementation must conform to the tests exactly.

The goal is to systematically turn tests green by implementing packages in dependency-graph order, maximizing passing tests at each milestone.

---

## Phase 0: Add Missing Go Dependencies

**Goal**: Install parsing libraries the implementation needs. No tests pass yet, but this unblocks everything.

**Actions**:
```
go get gopkg.in/yaml.v3
go get github.com/BurntSushi/toml
go get github.com/yuin/goldmark
go get github.com/osteele/liquid
```

**Risk**: The Liquid engine choice is the biggest risk. Tests expect `forloop.parentloop`, `{% render %}` with isolated scope, `{%- -%}` whitespace control, `{% tablerow %}`. Need to evaluate `osteele/liquid` capabilities against test expectations early. If insufficient, may need to use `Notifuse/liquidgo` (spec's recommendation) or a hybrid approach.

**Verify**: `go build ./...` compiles.

---

## Phase 1: Leaf Packages — No Internal Dependencies (~109 tests)

These packages depend only on stdlib or already-defined types.

### 1A: `internal/cache` — 20 tests
**File**: `internal/cache/cache.go`

- Initialize `hashes` and `templates` maps in `New()`
- Add unexported fields: `directoryData map[string][]string`, `configHash string`
- `SetHash`/`GetHash`/`HasChanged`/`Entries`/`Clear`: Direct map operations
- `HashContent`: `crypto/sha256` + `hex.EncodeToString`
- `SaveTo`/`LoadFrom`: JSON marshal/unmarshal via helper struct (fields are unexported). `LoadFrom` returns empty cache when file missing.
- `TrackTemplateUsage`/`InvalidatedPages`: Template-to-pages reverse index
- `ShouldSkipFile`: Hash content and compare with stored hash
- `IsConfigChanged`: Compare against stored `__config__` hash
- `InvalidatedByGlobalData`: Return all page paths from hashes map
- `TrackDirectoryData`/`InvalidatedByDirectoryData`: Directory-to-pages tracking

### 1B: `internal/data` — 8 tests
**File**: `internal/data/loader.go`

- `LoadFile`: Detect format by extension (.yaml/.yml, .toml, .json), parse with appropriate library
- `LoadDirectory`: Walk dir, `LoadFile` each, key by filename without extension. **Stem collision detection**: Track seen stem names. If two files share a stem (e.g., `team.csv` and `team.yaml`), return an error listing both files. No silent overwrites — consistent with output path conflict philosophy (§2).
- `LoadCSV`: `encoding/csv`, first row = headers, subsequent rows = `[]map[string]string`

### 1C: `internal/cascade` — 28 tests
**Files**: `internal/cascade/merge.go`, `internal/cascade/context.go`

- `DeepMerge`: Recursive map merge, arrays replaced (not concatenated)
- `LoadDirectoryCascade`: Walk content dir for `_data.yaml` files, merge parent->child. Only creates entries for directories that contain `_data.yaml`.
- `FindCascadeData(cascadeData, contentBase, relPath)`: Ancestor-walking lookup. Given a page's `relPath`, computes the directory key and walks up the directory tree to find the nearest ancestor with cascade data. Returns `nil` when no ancestor has data. The pipeline must use this instead of exact key lookup — otherwise pages in directories without `_data.yaml` miss ancestor inheritance (spec §3 requires cascade to flow to all descendants).
- `BuildContext`/`BuildContextFull`: Allocate `PageContext` with shared pointers
- `Get`: Lookup order: Computed > FrontMatter > Directory > Global. May need `PluginData` field added.

### 1D: `internal/validation` — 16 tests
**File**: `internal/validation/conflicts.go`

- `DetectConflicts`: Group `OutputPathEntry` by path, return conflicts where count > 1
- `ValidatePermalinkAliases`: Error if page has no output URL (`p.URL == ""`) AND aliases. Must check the computed `URL` field (populated by `ResolveForSection`), not the front-matter `Permalink` field — pages without an explicit `permalink:` in front matter still have valid URLs from config-level `permalinks:` patterns (issue #110).

### 1E: `internal/pagination` — 22 tests
**File**: `internal/pagination/pagination.go`

- `Paginate`: Chunk data by `perPage`, build `PaginationContext` for each. First page = basePath, subsequent = `basePath + pathSegment + "/N/"`
- `ResolveDataSource`: Parse dot-path ref (`site.data.X`, `collections.X`), lookup in provided maps
- `PaginateWithLiquidPermalink`: Per-item pages with Liquid URL rendering

### 1F: `internal/template/filters.go` — ~22 tests (standalone filter functions)
**File**: `internal/template/filters.go`

Implement all 50+ filter functions and `ApplyFilter` dispatch table. Key implementations:
- **String**: Slugify, Upcase, Downcase, Capitalize, Truncate, TruncateWords, StripHTML, Escape, Replace, ReplaceFirst, Split, Join, Strip, Append, Prepend, NewlineToBr, Contains
- **Date**: DateFormat (strftime-style: `%B %d, %Y`)
- **Array**: Sort, Reverse, First, Last, Where, GroupBy, Size, Map, Uniq, Compact, Concat
- **Set**: Intersect, Union, Complement
- **URL**: URLEncode, URLDecode, AbsoluteURL
- **Math**: Plus, Minus, Times, DividedBy (guard div-by-zero), Modulo, Ceil, Floor, Round, Abs
- **Content**: Markdownify (needs goldmark)
- **Regex**: FindRE, ReplaceRE
- **Data**: JSONFilter, Default
- **Assets**: Fingerprint, SafeHTML
- `RegisterBuiltinFilters`: Register all filters on engine via `AddFilter`

**Verify**: `go test ./internal/cache/... ./internal/data/... ./internal/cascade/... ./internal/validation/... ./internal/pagination/...`

---

## Phase 2: Config + Content (~119 tests)

### 2A: `internal/config` — 49 tests
**File**: `internal/config/config.go`

- `Load`: Read file, detect format by extension, unmarshal into `Config` struct. Return parse errors (not `ErrNotImplemented`) for invalid files.
- `LoadWithDefaults`: Call `Load`, then apply defaults: `Build.Output="_site"`, `Templates.Engine="liquid"`, `Content.Markdown.Goldmark.TemplateTags=true`, `Pagination.Path="page"`, `Plugins.Timeout=5000`, `Content.Formats=["md","html"]`, `Language="en"`, `Build.Clean=true`, `Structure.Content="content"`, `Structure.Layouts="layouts"`, `Structure.Assets="assets"`, `Structure.Static="static"`, `Structure.Data="data"`
- **Note**: `FeedConfig` type still exists but is no longer a field on `Config`. Feed generation is now opt-in via template placement (see Phase 3D). Do not apply feed defaults here.
- `DetectConfigFile`: Check for `alloy.config.{yaml,yml,toml,json}` in order. Error if none found.
- `MergeFlags`: Map keys `"output"` -> `cfg.Build.Output`, `"verbose"` -> `cfg.Verbose`, `"quiet"` -> `cfg.Quiet`
- `Validate`: Check baseURL not empty and looks like a URL. Check timeout >= 0. Check title not empty (errors_test.go expects this). Error messages must contain field names.

**Testdata exists**: `internal/config/testdata/` with valid.yaml, valid.toml, valid.json, minimal.yaml, invalid.yaml

### 2B: `internal/content` — 75 tests
**Files**: `frontmatter.go`, `discovery.go`, `markdown.go`, `lifecycle.go`, `page.go`

**page.go**:
- `ToTemplateMap()`: Converts a `Page` to `map[string]interface{}` for Liquid template access (issues #66, #67). Merges FrontMatter keys first, then overlays struct fields (`url`, `date`, `summary`, `collection`, `slug`) with lowercase keys. Struct fields take precedence over same-named FrontMatter keys. Zero-value struct fields are skipped (don't override FrontMatter). Used by `buildCollectionsContext` and `TaxonomyPageContext.ToMap()` to convert `[]*Page` slices to template-accessible maps — raw `*Page` struct pointers are not reliably accessible from osteele/liquid due to reflection/acronym-casing issues.

**frontmatter.go**:
- `ParseFrontMatter`: Detect delimiter (`---`/`+++`/`{`), extract and parse. **Front matter is required on content files only** — files without front matter delimiters are a build error (not "return empty map"). Error message must contain "front matter" and suggest adding empty front matter (`---\n---`). Empty front matter is valid (returns empty map, body follows). Handle edge cases: no body, malformed YAML (error must contain "yaml"). Layouts, partials, and data files do not require front matter.
- `BuildPage`: Call `ParseFrontMatter`, populate `Page` struct fields. Include source path in error messages.

**discovery.go**:
- `Discover`: Walk `contentDir`, create `Page` per .md/.html/.txt. Set `Section` from first path segment. Handle page bundles. Ignore `_data.yaml`.
- `DiscoverWithFormats`: Same but filter by allowed extensions.

**markdown.go**:
- `RenderMarkdown`: Configure goldmark with extensions (tables, task lists, typographer, footnotes). Handle `Unsafe` (raw HTML passthrough). Handle `TemplateTags` (preserve `{{ }}`/`{% %}` through rendering via placeholder substitution).
- `RenderText`: Wrap in `<pre>` tags.

**lifecycle.go**:
- `FilterByLifecycle`: Exclude drafts (unless includeDrafts), future publishDate, past expiryDate.

**Testdata exists**: `internal/content/testdata/site1/` with full content directory

**Verify**: `go test ./internal/config/... ./internal/content/...`

---

## Phase 3: Mid-Level Packages (~79 tests)

### 3A: `internal/permalink` — 22 tests
**File**: `internal/permalink/permalink.go`

- `ResolveTokens`: Replace `:year`, `:month`, `:day`, `:slug`, `:section`, `:filename`, `:title`
- `Resolve`: Front matter permalink > pattern with tokens. Handle `permalink: false`.
- `ContainsLiquidTags`: Check for `{{` in string
- `DefaultFromPath`: `blog/my-post.md` -> `/blog/my-post/`. Handles index files: `index.md` → `/`, `blog/index.md` → `/blog/`, `blog/post/index.md` → `/blog/post/` (strips `/index` suffix).
- `ResolveForSection`: Front matter > section pattern > default pattern > file path. **Index file override (issue #39)**: Before applying section or default patterns (steps 2-3), check if the page is an index file (`isIndexFile(page.RelPath)`). If so, skip directly to `DefaultFromPath` (step 4). This prevents `default: "/:slug/"` from turning `index.md` (title: "Home") into `/home/` instead of `/`. Front matter `permalink:` (step 1) still overrides — allows configuring index path for subdirectory deployments.
- `ResolveAliases`: Return page's Aliases slice

### 3B: `internal/collection` — 21 tests
**Files**: `collection.go`, `taxonomy.go`

- `BuildCollections`: Group pages by section with date-based permalink patterns
- `BuildCollectionsWithMode(pages, permalinkCfg, devMode bool)`: Lifecycle-aware wrapper — filters drafts (via `content.FilterByLifecycle`) before calling `BuildCollections`. `devMode=true` includes drafts; `devMode=false` excludes them.
- `SortPages`/`SortByFrontMatter`: Stable sort, dateless pages sort after dated ones
- `Freeze`/`IsFrozen`/`AddPage`: Add `frozen bool` field, error if frozen
- `BuildTaxonomies`: Group pages by declared taxonomy keys from front matter
- `GenerateTaxonomyPages`: Index + per-term pages with configured permalinks
- `BuildTaxonomyPageContext`/`DetectDuplicateTermSlugs`

### 3C: `internal/template` — context, layout, shortcodes (~27 tests)
**Files**: `context.go`, `layout.go`, `shortcodes.go`

- `BuildTemplateContext(page, siteData, allPages, collections, paginationCtx *pagination.PaginationContext, asName string)`: Populate `TemplateContext` from page + site data. All pages live under `site.pages` (not top-level `pages`) for consistency with `site.title`, `site.data`, etc. When `paginationCtx` is non-nil, set `ctx.Pagination` and inject `ctx.Custom[asName]` as a top-level alias for `pagination.items`. Non-paginated callers pass `nil, ""`.
- `TemplateContext` struct additions: `Pagination *pagination.PaginationContext` field, `Custom map[string]interface{}` field for dynamic top-level variables (the `as` alias).
- `ResolveLayout`: Lookup chain per spec (front matter > section name > filename > default). Handle `layout: false`.
- `ResolveLayoutWithCascade(page, layoutsDir, engine, permalinkCfg, cascadeData)`: Same lookup chain as `ResolveLayout`, but also considers `_data.yaml` cascade data for layout resolution. Front matter takes priority over cascade data.
- `ResolveTaxonomyLayout`: `layouts/taxonomies/<name>.liquid` > `layouts/<name>.liquid`
- `RegisterShortcode`/`RenderShortcodes`: Registry + inline expansion

### 3D: `internal/output` — 21 tests
**Files**: `writer.go`, `feed.go`, `sitemap.go`, `formats.go`

- `WriteFile`/`CleanOutputDir`/`ComputeOutputPath`/`WriteAliases`/`ShouldWrite`
- `ResolveFeedTemplates(layoutsDir)`: Scan `layouts/` for `feed.xml` templates. Template placement determines scope: `layouts/feed.xml` → site-wide (`/feed.xml`), `layouts/blog/feed.xml` → section (`/blog/feed.xml`), `layouts/taxonomies/tags/feed.xml` → per-term (`/tags/:slug/feed.xml`). Returns `[]FeedTemplate` with computed output paths and scope metadata.
- `RenderFeedTemplate(tmpl FeedTemplate, context)`: Render a discovered feed template with the appropriate filtered context (all pages for site-wide, section pages for section feeds, tagged pages for per-term feeds).
- **Note**: Feeds are now opt-in via template placement, not auto-generated. The old `GenerateFeed` function has been removed.
- `GenerateSitemap`: XML sitemap with baseURL prefix, per-page exclusions
- `ResolveOutputFormat(page) string`: Returns first entry from `page.Outputs`, defaulting to `"html"` when unset.
- `ResolveFormatLayout(page, format, layoutsDir, engine) (string, error)`: Finds layout for a specific format: `<layout>.<format>.<engine-ext>` (e.g., `single.json.liquid`).

#### Multi-format pipeline wiring (issue #71)

The pipeline currently renders each page once (HTML only). Pages with `outputs: ["html", "json"]` need to render once per format. The wiring change is in Stage 5 (layout resolution and rendering, `build.go` ~line 141):

```
// Current: single render per page
for _, page := range pages {
    layoutPath := tmpl.ResolveLayout(page, layoutsDir, engineName, cfg.Permalinks)
    // ... render and write
}

// Required: render per format per page
for _, page := range pages {
    formats := page.Outputs
    if len(formats) == 0 {
        formats = []string{"html"}
    }
    for _, format := range formats {
        if format == "html" {
            // Existing HTML path: template.ResolveLayout → render → ComputeOutputPath
        } else {
            // Format-specific: template.ResolveLayoutForFormat(page, layoutsDir, engine, format)
            // This is the canonical resolver — it checks disk for the layout file and
            // returns an error if not found, consistent with ResolveLayout for HTML.
            // (output.ResolveFormatLayout computes the path but does not validate existence.)
            // Output path: replace extension — /my-post/index.json instead of index.html
        }
    }
}
```

Key points:
- `page.Outputs` is populated from `outputs` front matter during `content.BuildPage` (the field already exists on `content.Page`)
- HTML format uses existing `template.ResolveLayout` (no change). Non-HTML formats use `template.ResolveLayoutForFormat` — the canonical resolver that validates the layout file exists on disk. `output.ResolveFormatLayout` computes the expected path (useful for testing) but should not be used as the pipeline resolver.
- Output path for non-HTML: `output.ComputeOutputPath(page.URL)` returns `slug/index.html` — for JSON it should produce `slug/index.json`. Either extend `ComputeOutputPath` to accept a format parameter, or compute manually.
- The rendered body for each format is independent — a page's JSON output uses a different layout than its HTML output.
- Content rendering (markdown → HTML) happens once. Layout rendering happens per format.

### 3E: `internal/assets` — 11 tests
**File**: `internal/assets/assets.go`

- `CopyAssets`/`ProcessAssets`/`ResolveURL`

**Testdata exists**: `internal/assets/testdata/site-assets/`

**Verify**: `go test ./internal/permalink/... ./internal/collection/... ./internal/template/... ./internal/output/... ./internal/assets/...`

---

## Phase 4: Template Engines + Pipeline = Walking Skeleton (~57 tests)

### 4A: Liquid Engine (~20 tests)
**File**: `internal/template/liquid.go`

- Adapt chosen Liquid library to `TemplateEngine` interface
- `Parse`, `Render`, `AddFilter`, `AddTag`, `ParseWithIncludes`
- `RenderTemplate`: Parse + render, wrap errors with source path
- Must support: `{{ var }}`, `{{ page.title }}`, `{% if %}`, `{% for %}`, `{% assign %}`, `{{ content }}` injection, `{% include %}`, filter pipelines

### 4B: Go Template Engine (~12 tests)
**File**: `internal/template/gotemplate.go`

- Adapt `html/template` to `TemplateEngine` interface via `FuncMap`

### 4C: `internal/static` — 6 tests
**File**: `internal/static/copy.go`

- `CopyStatic`/`CopyPassthrough`: Walk and copy preserving structure
- `CopyPassthroughWithValidation(mappings, projectRoot, outputDir, managedDirs)`: Same as `CopyPassthrough`, but silently skips any mapping where the `from` path resolves to a managed directory (content, layouts, assets, static, data). Prevents passthrough configs from accidentally overwriting managed content.

### 4D: `internal/pipeline` — 19 tests
**File**: `internal/pipeline/build.go`

- `BuildWithContent`: Accept injected content, render through pipeline. Error messages must contain source file path + "template rendering" stage.
- `BuildPhase1`/`BuildPhase2`: Phase separation. Phase 2 operates entirely in memory:
  1. For each page, scan intermediate HTML for custom element tags (anything with a hyphen) and record in ComponentMap for cache invalidation
  2. For each page with custom elements, extract the inner content of `<body>` (everything between `<body>` and `</body>`, not the tags themselves). Invoke the SSR command based on `config.SSRConfig.Mode`:
     - **`exec`** (default): spawn a new process per page via `os/exec`, pipe body content to stdin, read transformed body content from stdout
     - **`stream`**: use a persistent process (started once for the build), write body content + `\0` to stdin, read until `\0` from stdout
  3. Re-insert the SSR'd body content into the original document skeleton (preserve `<!DOCTYPE>`, `<html>`, `<head>`, `<body>` tags from the intermediate HTML)
  - The command is invoked once per page — the SSR engine handles component discovery, deduplication, and DSD injection internally.
  - Pages without custom elements skip the SSR command invocation (pass through unchanged).
  - If the command is not found, `BuildPhase2` must return an error (no silent fallback).
  - **Timeout**: Each page render is subject to `config.SSRConfig.Timeout` (default 30s). Use `context.WithTimeout` on exec, or a read deadline on stream. On timeout, kill the process.
  - **Exec error isolation**: A failed page (timeout or non-zero exit) does not abort the build. Continue with remaining pages, collect all failures, report at the end.
  - **Stream recovery**: On process crash, timeout, or malformed output — restart the process, retry the failed page once. If it fails again, skip the page and continue. Report all skipped pages at the end.
- **`validateOutputDir`** (issue #9): Uses path equality + parent/child overlap detection (not substring matching). Only rejects exact matches (`output == content`) and nesting (`output = content/build` or `content` inside `output`). Names like `my_content_site` are valid output directories.
- **Render ordering** (issue #10): Markdown renders first, then template tags — per spec §6 steps 3-4. Goldmark's TemplateTags extension preserves `{{ }}`/`{% %}` through markdown rendering. After markdown rendering and before Liquid processing, `escapeTemplateTagsInCode` converts template tags inside `<code>` elements to HTML entities so Liquid ignores them (issue #46). Markdown errors use stage name `"content transformation"`, template errors use `"template rendering"`.

#### `Build()` full orchestration (issue #30)

`Build()` must orchestrate all pipeline stages from §2. Currently it stops after markdown+template rendering. The individual packages for each stage are implemented and pass tests — they need to be called in order:

```
 1. config.ApplyDefaults(cfg)                           ✅ done
 2. validateOutputDir(cfg)                               ✅ done
 3. content.DiscoverWithFormats(contentDir, formats)      ✅ done
 4. content.FilterByLifecycle(pages, now, includeDrafts)  ✅ done (issue #108: must pass includeDrafts from server mode, not hardcode false)
 5. permalink.ResolveForSection(page, cfg.Permalinks)     ✅ done
 6. cascade.LoadDirectoryCascade + FindCascadeData + PageContext ✅ done
 7. data.LoadDirectory(dataDir) → siteData                ✅ done
 8. collection.BuildCollections(pages, permalinks)        ✅ done
 9. collection.BuildTaxonomies(pages, taxonomies)         ✅ done
10. template.RegisterBuiltinFilters(engine)               ✅ done
11. renderPages (markdown → template tags)                ✅ done
12. template.ResolveLayout(page, layoutsDir, engine)      ✅ done
13. Render page through layout ({{ content }} injection)  ✅ done
14. output.ComputeOutputPath(page) → output path          ✅ done
15. output.WriteFile(outputPath, html)                    ✅ done
16. static.CopyStatic(staticDir, outputDir)               ✅ done
17. assets.CopyAssets(assetsDir, outputDir)                ✅ done
18. static.CopyPassthroughWithValidation(...)             ✅ done
19. output.GenerateSitemap(pages, baseURL, outputDir)     ✅ done
20. cache.SaveTo(cacheFile)                               ✅ done
```

**Key implementation notes:**
- Steps 5-9 happen before rendering (step 11) so templates can access `page.url`, `collections.*`, filters, etc.
- **Multi-format output (issue #71)**: Steps 12-15 (layout resolution → render → compute path → write) must loop over `page.Outputs` when present. Content rendering (step 11) happens once; layout rendering happens per format. See Phase 3D wiring guidance.
- Step 12-13 happen after step 11: content is rendered first, then injected into the layout via `{{ content }}`.
- Steps 15-20 are post-render: write files, copy assets, generate sitemap, persist cache.
- If content directory doesn't exist or is empty, `Build()` should return a successful zero-page result (not error). This is required for `alloy init && alloy build` to work and for cmd tests to pass.
- **i18n (issue #70)**: When `cfg.Languages` is present, the pipeline uses a two-pass per-language loop (see Phase 5C wiring). Pass 1 runs steps 3-11 (discovery through content rendering) per language. Then `LinkTranslations` runs once across all languages. Pass 2 runs steps 12-15 (layout resolution through output writing) per language — this ensures `page.Translations` is populated before templates render. Steps 1-2 (config/validation) and 16-20 (static/assets/sitemap/cache) run once outside the loop.
- **Plugin filter bridging (issue #93)**: After `registry.LoadPlugins(hooks)` (step 0) and engine creation (step 10), bridge plugin-discovered filters to the template engine. For each filter name from `LoadPlugins()`, call `engine.AddFilter(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallFilter()`. This must happen before content rendering (step 11) so templates can use plugin filters. Similarly, `alloy.hook()`/`alloy.on()` registrations discovered by `EvalFile()` must be wired into the `HookRegistry` during `LoadPlugins()`.
- **Plugin shortcode bridging (issue #139)**: Same pattern as filter bridging. After engine creation, iterate `rt.RegisteredShortcodes()` and call `engine.AddTag(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallShortcode(name, args, innerContent)`. Both inline and block shortcodes must be supported. `CallShortcode()` is currently a stub (returns input unchanged) and must be implemented to actually invoke the JS shortcode function.
- **Plugin filter shadowing (issue #140)**: When a plugin registers a filter with the same name as a built-in liquidgo filter (e.g., `reverse`), the plugin's version must take precedence. Per spec §4: "the last one loaded wins." The current implementation fails because `knownLiquidFilters` prevents plugin filters from being treated as dynamic filters, so liquidgo's native implementation intercepts the call. The fix must ensure plugin-registered filters override built-in filters in the template engine's dispatch chain.
- **Hook payload contract (issue #182)**: All hook payloads must be JSON-serializable for JS/WASM plugins. Four categories:
  - **Per-page HTML hooks** (`onContentTransformed`, `onPageRendered`): fire once per page with the rendered HTML string as payload. The pipeline iterates pages and calls `hooks.RunWithTimeout(event, string(page.RenderedBody))` for each page. The returned string is converted back to `[]byte` and stored in `page.RenderedBody`. Current code passes the entire `pages` slice — must change to per-page iteration.
  - **Per-page JSON hooks** (`onContentLoaded`, `onDataCascadeReady`): fire once per page with a `map[string]interface{}` payload containing page-scoped data (`path`, `frontMatter`, `body` or `data`). Return value is deserialized and applied back to the page.
  - **Per-asset hook** (`onAssetProcess`): fire once per asset with `map[string]interface{}{"path": relPath, "content": fileContent}`. Return value's `"content"` key replaces the asset content.
  - **Per-build hooks** (`onConfig`, `onBeforeValidation`, `onAfterValidation`, `onDataFetched`): convert Go structs to `map[string]interface{}` before passing to `CallHook`. Deserialize returned map and apply changes back. Requires enhancing the QuickJS/WASM hook bridge so structured Go values are marshaled as real JSON objects (not stringified) — encode the map to JSON, parse in JS, then deserialize returned JS objects back into Go maps.
  - **Read-only hooks** (`onBuildComplete`, `onDevServerStart`, `onFileChanged`): serialize to JSON for observation, return value ignored. The runtime bridge must still preserve object payloads at the JS boundary for observation hooks.
- **WASM runtime (issue #181)**: `WASMRuntime.LoadModule()` is a stub. Must use wazero to compile and instantiate the WASM binary, discover exported functions (`filter`, `shortcode`, `hook`), and register them. `CallExport(name, args...)` must invoke the actual WASM function and return the result. `RegisteredFilters()` must return filter names from the module's exports. `LoadModule` must return an error for invalid WASM binaries.
- **Incremental build via cache (issue #105)**: Before step 3 (content discovery), load the previous build cache via `cache.LoadFrom(cacheDir)`. After discovering pages (step 3) and computing hashes, use `previousCache.ShouldSkipFile(relPath, content)` to skip unchanged pages — no re-parse, no re-render. Template changes override content-hash skipping: if a layout file changed, `previousCache.InvalidatedPages(layoutPath)` returns the affected pages, which must be rebuilt even if their content hash is unchanged. Config changes (`previousCache.IsConfigChanged(currentHash)`) trigger a full rebuild of all pages. The `BuildResult` should report the skip count (e.g., "Built 5 pages, 27 skipped (cached)").

#### Cascade wiring (PR #55)

The pipeline uses `cascade.PageContext` per spec §3 for proper 5-level cascade:

1. `cascade.LoadDirectoryCascade(contentDir)` — loads all `_data.yaml` files with parent→child merge
2. `cascade.FindCascadeData(cascadeData, contentBase, page.RelPath)` — walks up the directory tree to find the nearest ancestor with cascade data (handles directories without their own `_data.yaml`)
3. `cascade.BuildContext(siteData, dirData, page.FrontMatter)` — creates a `PageContext` with shared pointers for Global (level 1) and Directory (level 2), per-page FrontMatter (level 3)
4. `pctx.ToMap()` — flattens via `PageContext.Get()` (lazy deep-merge only for conflicting nested keys) into `page.FrontMatter` so downstream consumers (taxonomy building, collection sorting) see effective values

Levels 4/5 (Computed/PluginData) are nil until plugin hooks populate them — `PageContext` is ready for plugins with no pipeline changes needed.

### WALKING SKELETON MILESTONE
At this point, `alloy build` works end-to-end on test fixtures.

**Verify**: `go test ./internal/template/... ./internal/static/... ./internal/pipeline/...`

---

## Phase 5: Plugin + Fetch + I18n + CLI (~111 tests)

### 5A: `internal/plugin` — 63 tests
**Files**: `hooks.go`, `registry.go`, `node.go`, `wasm.go`

- **hooks.go**: Hook registry with timeout, chained execution, warnings. `HookFunc` signature is `func(ctx context.Context, payload interface{}) (interface{}, error)` — context carries timeout deadline for cooperative cancellation (issue #13). `Run()` passes `context.Background()`. `RunWithTimeout()` uses `context.WithTimeout()` and passes the derived context to each hook.
- **registry.go**: Plugin classification by file type, discovery, filter registration, conflict warnings
- **node.go**: LSP-style message encoding/decoding, bridge state management
- **wasm.go**: QuickJS/WASM runtime with filter/shortcode/hook registration and execution.
  - `EvalFile()` parses `alloy.filter()`, `alloy.shortcode()`, and `alloy.hook()`/`alloy.on()` registrations
  - `CallFilter()` must execute the actual JS filter function and return the transformed value — not passthrough, not pattern-matching. The current `simulateJSFilter` approach only handles known patterns (word count); arbitrary JS like `toUpperCase()` returns input unchanged. Real QuickJS execution via wazero is required (issue #103).
  - `RegisteredHooks()` returns hook names discovered during `EvalFile()`
  - `LoadPlugins()` returns discovered filter names + hook registrations so the pipeline can bridge them to the template engine and HookRegistry

### 5B: `internal/fetch` — 16 tests
**File**: `internal/fetch/fetch.go`

- REST/GraphQL fetching, file-based caching, XML/CSV parsing, GraphQL data unwrapping

#### Source pipeline wiring (issue #107)

After `loadSiteData()` loads local data files, the pipeline must iterate `cfg.Sources` and fetch/cache each source before template rendering:

1. For each `SourceConfig` in `cfg.Sources`:
   - Check cache via `fetch.GetCached(name, cacheDir, source.Cache)` — if found and TTL valid, use cached data
   - If not cached (or `--refetch` flag set), call the appropriate fetcher based on `source.Type`:
     - `"rest"` → `fetch.FetchREST(source.URL)`
     - `"graphql"` → `fetch.FetchGraphQL(source.Endpoint, source.Query)`
     - `"plugin"` → `fetch.FetchPluginSource(source.Plugin, configMap)`
   - Save fetched data to cache via `fetch.SaveCache(name, cacheDir, data)`
   - Merge result into `siteData` under the `source.As` key so templates access it as `site.data.<as>`
2. Fire `onDataFetched` hook after all sources are merged (existing hook call stays in place)
3. On fetch failure: abort build with clear error identifying the source name and URL

### 5C: `internal/i18n` — 18 tests
**File**: `internal/i18n/i18n.go`

- `BuildLanguageContexts(cfg map[string]*config.LanguageConfig) ([]LanguageContext, error)`: Returns contexts sorted by weight (lowest first = default). Errors when nil/empty.
- `OutputPrefix(langCode string, isRoot bool) string`: Returns `""` for root language, `"lang/"` for non-root.
- `ContentTreeRoute(langCode string) string`: Returns `"content/<lang>/"` for language-specific content discovery.
- `LanguageData(ctx LanguageContext) map[string]interface{}`: Returns `site.language` data cascade entry (`code`, `title`, `root`, `strings`).
- `LanguageSiteTitle(globalTitle string, langCfg *config.LanguageConfig) string`: Returns language-specific title override, falling back to global.
- `FilterByLanguage(pages []*content.Page, langCode string) []*content.Page`: Filters pages by `lang` front matter field.
- `LinkTranslations(pages []*content.Page, languages []string) error`: Groups pages by relative path (stripping language prefix), links matching pages as translations.
- `GetTranslations(page *content.Page) []TranslationInfo`: Returns URL + language code for each linked translation.
- `BuildTaxonomiesForLanguage(langCode string, pages []*content.Page) map[string]interface{}`: Generates per-language taxonomy maps from language-scoped pages.

#### i18n pipeline wiring (issue #70)

The pipeline needs a language-aware outer loop. When `cfg.Languages` is nil/empty, the pipeline runs once (current behavior — single-language site). When `cfg.Languages` is present, the pipeline iterates over each language:

```
if cfg.Languages != nil {
    langContexts := i18n.BuildLanguageContexts(cfg.Languages)
    allLangPages := []*content.Page{}  // collect across languages for translation linking

    // ── Pass 1: discover + content-render per language (steps 3-11) ──
    // Each language's pages are discovered, filtered, collected, and
    // content-rendered (Markdown → HTML). Layout resolution and output
    // writing are deferred so that page.Translations is populated first.
    type langBatch struct {
        ctx         LanguageContext
        pages       []*content.Page
        collections map[string]interface{}
        siteData    map[string]interface{} // per-language copy
    }
    var batches []langBatch

    for _, langCtx := range langContexts {
        // Scope content discovery to content/<lang>/
        contentDir := filepath.Join(projectRoot, "content", langCtx.Code)
        pages := content.DiscoverWithFormats(contentDir, formats)

        // Set lang on each page's front matter
        for _, page := range pages {
            page.FrontMatter["lang"] = langCtx.Code
        }

        // Apply output prefix to permalinks
        prefix := i18n.OutputPrefix(langCtx.Code, langCtx.Root)
        // prefix permalinks: page.URL = prefix + page.URL
        //
        // IMPORTANT (issue #113): permalink resolution must use the
        // ORIGINAL relPath (without language prefix), not the prefixed
        // one. The language prefix is added for translation linking,
        // but DefaultFromPath("en/index.md") returns "/en/" instead of
        // "/", causing double-prefixing for index pages. Resolve the
        // permalink first, then prefix RelPath for translation linking.

        // Inject site.language into per-language site data copy
        langSiteData := copyMap(siteData)
        langSiteData["language"] = i18n.LanguageData(langCtx)
        langSiteData["title"] = i18n.LanguageSiteTitle(cfg.Title, cfg.Languages[langCtx.Code])

        // Run steps 4-11 (lifecycle filter, cascade, permalinks,
        // collections, taxonomies, content rendering) per language
        // Collections and taxonomies are per-language (scoped to langPages)

        allLangPages = append(allLangPages, pages...)
        batches = append(batches, langBatch{ctx: langCtx, pages: pages, collections: langCollections, siteData: langSiteData})
    }

    // ── Link translations across all language trees ──
    langCodes := make([]string, len(langContexts))
    for i, ctx := range langContexts { langCodes[i] = ctx.Code }
    i18n.LinkTranslations(allLangPages, langCodes)

    // ── Pass 2: layout resolution + output writing (steps 12-15) ──
    // page.Translations is now populated, so templates can render
    // {% for trans in page.translations %} correctly.
    for _, batch := range batches {
        // Run steps 12-15 (layout resolution, template context build,
        // layout rendering, output writing) with batch.pages, batch.siteData
    }

    // Steps 16-20 (static copy, assets, sitemap, cache) run once after all languages
} else {
    // Single-language build (current behavior, no changes)
}
```

Key points:
- **Two-pass per-language loop**: Pass 1 (steps 3-11) discovers and content-renders each language. `LinkTranslations` runs between passes. Pass 2 (steps 12-15) resolves layouts and writes output. This ensures `page.Translations` is populated before templates render, enabling `{% for trans in page.translations %}` for `<link rel="alternate" hreflang="...">` tags.
- `layouts/` is shared across all languages — never scoped
- `data/` globals are shared, but `site.language` and `site.title` are overridden per-language iteration via a shallow copy
- Collections and taxonomies are per-language: `collections.blog` for English only contains English posts
- Languages can build in parallel (independent content trees) but initial implementation should be sequential

### 5D: `cmd/` + `main.go` — 15 tests
**Files**: `main.go`, `root.go`, `build.go`, `serve.go`, `init.go`, `version.go`

- **`main.go` exit code handling (issue #28)**: `main()` must check the error return from `cmd.Execute()` and call `os.Exit(1)` on failure. Without this, all CLI errors exit 0, breaking scripts and CI. Current code discards the error.
- Register Cobra flags (--config, --output, --root, --verbose, --quiet, --port, --preview, --no-drafts, --refetch) ✅ done (except --root)
- `Version`: Set to non-empty string ✅ done

#### `cmd/init.go` (issue #26)

`RunInit` needs 4 fixes:
1. **Create target directory** if it doesn't exist (`os.MkdirAll(dir, 0755)` before writing).
2. **Generated config must include `baseURL`** — current config is just `title: My Alloy Site` which fails `config.Validate`. Minimum: `title` + `baseURL: "http://localhost:3000"`.
3. **Print success message** after writing: `fmt.Println("Created alloy.config.yaml")`.
4. **Don't swallow "already exists" error** — the `RunE` wrapper catches the error, prints it, then returns `nil`. It must return the error so Cobra exits non-zero.

#### `cmd/build.go` (issue #27)

`RunE` is an empty stub. Must wire to pipeline:
1. Read `--config` flag, call `config.DetectConfigFile` or `config.Load`. If no config file found, use `config.ApplyDefaults` on an empty `Config` (zero-page build, not an error).
2. Read `--output`, `--verbose`, `--quiet`, `--root` flags, call `config.MergeFlags`.
3. Call `pipeline.Build(cfg)`.
4. Print summary: `fmt.Printf("Built %d pages in %s\n", result.PageCount, result.Duration)`.
5. Return any error from Build — Cobra will handle exit code.

**Test note**: The existing cmd test `"build command executes the build pipeline successfully"` runs without a project fixture. Build must handle missing content directory gracefully (return zero-page success, not error) for this test to pass.

#### `cmd/serve.go` (issue #29)

`RunE` is an empty stub. Must wire to server:
1. Load config (same as build).
2. Run initial build via `pipeline.Build(cfg)`.
3. Read `--port`, `--preview`, `--no-drafts`, `--refetch` flags.
4. Call `server.NewWithMode(cfg, mode)` and `server.Start()`.
5. Start file watcher.
6. Block until interrupt.

**Test note**: The cmd test `"serve command is registered and callable"` verifies command registration via `root.Find()` without calling `Execute()`, since the serve command blocks on `Wait()`. Server startup behavior is tested directly in `internal/server/server_test.go`.

#### `--root` flag (issue #75)

Add `--root` / `-r` as a persistent flag on the root command (like `--config`). Default: empty string (use config file directory). When set, overrides `cfg.ProjectRoot` after config loading.

**`config.MergeFlags`** must handle the `"root"` key:
```go
if v, ok := flags["root"]; ok {
    if s, ok := v.(string); ok && s != "" {
        absRoot, err := filepath.Abs(s)
        if err == nil {
            cfg.ProjectRoot = absRoot
        }
    }
}
```

**`cmd/root.go`**: Register `--root` as persistent flag:
```go
rootCmd.PersistentFlags().StringP("root", "r", "", "Project root directory (default: config file directory)")
```

**`cmd/build.go` and `cmd/serve.go`**: Read `--root` flag and pass to `MergeFlags`:
```go
if cmd.Flags().Changed("root") {
    v, _ := cmd.Flags().GetString("root")
    flags["root"] = v
}
```

The flag must be applied **after** config loading but **before** pipeline execution, since all `structure:` paths resolve against `ProjectRoot`.

**Verify**: `go test ./internal/plugin/... ./internal/fetch/... ./internal/i18n/... ./cmd/...`

---

## Phase 6: Server + SSR (~65 tests)

### 6A: `internal/server` — 51 tests
**Files**: `server.go`, `watcher.go`, `overlay.go`

- HTTP server with mode-aware behavior (dev/preview)
- File watcher with debouncing and change classification
- Error overlay injection
- `WebSocketReloadMessage()`: Return `{"type": "reload"}` JSON string for connected browser reload
- `DebounceInterval()`: Return configurable debounce interval in milliseconds for file watcher
- `DetermineRebuildAction(changedFiles []string) RebuildScope`: Classify file changes as incremental or full rebuild. Many simultaneous changes trigger a full rebuild.
- `StartWithPortFallback(preferredPort, maxAttempts int) (int, error)`: Try `net.Listen("tcp", ":port")` starting at `preferredPort`. On `EADDRINUSE`, increment port and retry up to `maxAttempts` times. Return the actual port on success. After exhausting all attempts, return error containing `"no available port"` and the range tried. Log a warning when skipping an occupied port. Store the actual port on the Server struct.
- `Port() int`: Return the actual port the server is listening on. Returns 0 before the server has started.
- `Serve404Page(outputDir string) ([]byte, error)`: Check for `404.html` at the output root. If found, return its contents (for the HTTP handler to serve with a 404 status code). If not found, return an error so the caller can fall back to Go's default `http.NotFound()`. In dev mode, the 404 page must receive the WebSocket reload script injection like any other served page (issue #109).

**Test hygiene (issue #59)**: All server tests that call `Start()` must use port 0 (OS-assigned) to avoid collisions when `go test ./...` runs packages in parallel. Every successful `Start()` or `StartWithPortFallback()` must be paired with `defer srv.Stop()` to release the port promptly.

#### Port auto-increment (issue #60)

`cmd/serve.go` should call `srv.StartWithPortFallback(port, 10)` instead of `srv.Start(port)`. The returned actual port is used in the startup message (`Serving at http://localhost:<actual-port>`). The `--port` flag remains "preferred" — it's the starting point for the search, not a hard requirement. No `--strict-port` flag needed; `alloy serve` is a dev tool and auto-increment is always the right UX.

### 6B: `internal/ssr` — 23 tests
**Files**: `scanner.go`, `depgraph.go`, `persistence.go`

- `ScanComponents(html string) []string`: Parse HTML for custom element tags (anything with a hyphen), return unique tag names. Used for component tracking, not for per-instance SSR.
- `ExtractBody(html string) (body string, before string, after string)`: Extract the inner content of `<body>` from a full HTML document. Returns the body content, the document prefix (everything up to and including `<body>`), and the suffix (from `</body>` onward). Used by `BuildPhase2` to split the document before piping to the SSR command.
- `ReassembleDocument(before string, ssrBody string, after string) string`: Re-insert SSR'd body content into the original document skeleton.
- `RenderPage(command string, html string) (string, error)`: Exec mode — spawn process, pipe body content via stdin, read transformed body content from stdout. Errors when the command is not found or returns non-zero exit.
- `RenderPageWithTimeout(ctx context.Context, command string, html string) (string, error)`: Exec mode with timeout — same as `RenderPage` but respects context deadline. Kills process on timeout.
- `NewStreamRenderer(command string) (*StreamRenderer, error)`: Start a persistent process for stream mode. Returns a handle for sending NUL-delimited messages.
- `(*StreamRenderer) RenderPage(html string) (string, error)`: Stream mode — write HTML + `\0` to the persistent process's stdin, read until `\0` from stdout. Errors if the process has exited or returns malformed output.
- `(*StreamRenderer) Restart() error`: Kill and restart the persistent process. Used for recovery after crash, timeout, or malformed output.
- `(*StreamRenderer) Close() error`: Shut down the persistent process (close stdin, wait for exit).
- `HashOutput(html string) string`: Content hash for Phase 2 output comparison (skip SSR when unchanged).
- Component dependency graph (`depgraph.go`): Tracks parent-child component relationships for future nested component invalidation. Keep as-is.
- Component map persistence (`persistence.go`): Save/load `pageToComponents`, `componentToPages`, `definitionHashes` to `.alloy/components.json`. `ShouldSkipSSR` checks definition hash for cache invalidation.
- **Removed**: `SSREngine` interface (`engine.go`), `DeduplicateInstances`, `InsertMarkers`, `StampBack`, `ParseSSRConfig`, `RenderViaHTTP`, `RenderViaStdio`, `ComponentCacheKey`, `ComponentInstance` struct with attrs/hash fields

**Verify**: `go test ./internal/server/... ./internal/ssr/...`

---

## Phase 7: Integration Tests + Final (~16 tests)

### 7A: `test/integration/` — 32 tests
**Files**: `build_test.go`, `crosscutting_test.go`, `plugin_template_test.go`

Cross-package integration paths that should mostly pass once pipeline works:
- Minimal/cascade/collections fixture builds
- Data file -> template rendering
- Front matter -> permalink -> output
- Collection -> pagination -> output
- Taxonomy -> layout -> template context
- Plugin hook -> content transform
- i18n -> data cascade -> template
- Filter integration with both Liquid and Go engines
- Plugin → template engine filter bridging (issue #93)
- External data source → fetch → site.data → template context (issue #107)
- Plugin → HookRegistry hook bridging (issue #93)
- Build cache → incremental build skip detection (issue #105)
- Draft visibility → server mode → lifecycle filtering (issue #108)
- i18n index page URL resolution without prefix doubling (issue #113)

**Verify**: `go test ./... 2>&1 | grep -E "Passed|Failed"`

---

## Summary

| Phase | Packages | Est. Tests | Cumulative |
|-------|----------|-----------|------------|
| 0 | Dependencies only | 0 | 0 |
| 1 | cache, data, cascade, validation, pagination, filters | ~109 | ~109 |
| 2 | config, content | ~119 | ~228 |
| 3 | permalink, collection, template (context/layout/shortcodes), output, assets | ~79 | ~307 |
| 4 | template (liquid/go engines), static, pipeline **[WALKING SKELETON]** | ~56 | ~363 |
| 5 | plugin, fetch, i18n, cmd | ~111 | ~474 |
| 6 | server, ssr | ~70 | ~544 |
| 7 | integration tests + remaining | ~86 | ~630 |

## Key Risks

1. **Liquid engine compatibility** — Biggest unknown. Evaluate osteele/liquid vs Notifuse/liquidgo against test expectations in Phase 0.
2. **Template tag preservation in Markdown** — goldmark must preserve `{{ }}`/`{% %}`. Requires custom extension or placeholder substitution.
3. **WASM/QuickJS runtime** — May need `wazero` dependency. Most infrastructure-heavy feature. Defer if needed.
4. **Pipeline test expectations** — `pipeline.Build` tests call `Build(cfg)` without specifying content directory. Need to handle defaults or infer from config.
