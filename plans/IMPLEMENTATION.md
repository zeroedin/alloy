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
- `LoadDirectory`: Walk dir, `LoadFile` each, key by filename without extension
- `LoadCSV`: `encoding/csv`, first row = headers, subsequent rows = `[]map[string]string`

### 1C: `internal/cascade` — 22 tests
**Files**: `internal/cascade/merge.go`, `internal/cascade/context.go`

- `DeepMerge`: Recursive map merge, arrays replaced (not concatenated)
- `LoadDirectoryCascade`: Walk content dir for `_data.yaml` files, merge parent->child
- `BuildContext`/`BuildContextFull`: Allocate `PageContext` with shared pointers
- `Get`: Lookup order: Computed > FrontMatter > Directory > Global. May need `PluginData` field added.

### 1D: `internal/validation` — 15 tests
**File**: `internal/validation/conflicts.go`

- `DetectConflicts`: Group `OutputPathEntry` by path, return conflicts where count > 1
- `ValidatePermalinkAliases`: Error if page has `permalink: false` AND aliases

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

### 2B: `internal/content` — 70 tests
**Files**: `frontmatter.go`, `discovery.go`, `markdown.go`, `lifecycle.go`

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

## Phase 3: Mid-Level Packages (~75 tests)

### 3A: `internal/permalink` — 22 tests
**File**: `internal/permalink/permalink.go`

- `ResolveTokens`: Replace `:year`, `:month`, `:day`, `:slug`, `:section`, `:filename`, `:title`
- `Resolve`: Front matter permalink > pattern with tokens. Handle `permalink: false`.
- `ContainsLiquidTags`: Check for `{{` in string
- `DefaultFromPath`: `blog/my-post.md` -> `/blog/my-post/`
- `ResolveForSection`: Front matter > section pattern > default pattern > file path
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

### 3C: `internal/template` — context, layout, shortcodes (~23 tests)
**Files**: `context.go`, `layout.go`, `shortcodes.go`

- `BuildTemplateContext`: Populate `TemplateContext` from page + site data. All pages live under `site.pages` (not top-level `pages`) for consistency with `site.title`, `site.data`, etc.
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
- `ResolveOutputFormat`/`ResolveFormatLayout`

### 3E: `internal/assets` — 11 tests
**File**: `internal/assets/assets.go`

- `CopyAssets`/`ProcessAssets`/`ResolveURL`

**Testdata exists**: `internal/assets/testdata/site-assets/`

**Verify**: `go test ./internal/permalink/... ./internal/collection/... ./internal/template/... ./internal/output/... ./internal/assets/...`

---

## Phase 4: Template Engines + Pipeline = Walking Skeleton (~56 tests)

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

### 4D: `internal/pipeline` — 16 tests
**File**: `internal/pipeline/build.go`

- `Build`: Orchestrate full pipeline. Set `SSRSkipped=true` when no SSR config. Handle empty content (PageCount=0). Return `OutputDir`, `PageCount`, `Duration`.
- `BuildWithContent`: Accept injected content, render through pipeline. Error messages must contain source file path + "template rendering" stage.
- `BuildPhase1`/`BuildPhase2`: Phase separation. Phase 2 inserts `<template shadowrootmode="open">` markers for custom elements (minimal SSR simulation).
- **`validateOutputDir`** (issue #9): Uses path equality + parent/child overlap detection (not substring matching). Only rejects exact matches (`output == content`) and nesting (`output = content/build` or `content` inside `output`). Names like `my_content_site` are valid output directories.
- **Render ordering** (issue #10): Markdown renders first, then template tags — per spec §6 steps 3-4. Goldmark's TemplateTags extension preserves `{{ }}`/`{% %}` through markdown rendering. Code fences protect their contents automatically (goldmark parsers take precedence). Markdown errors use stage name `"content transformation"`, template errors use `"template rendering"`.

### WALKING SKELETON MILESTONE
At this point, `alloy build` works end-to-end on test fixtures.

**Verify**: `go test ./internal/template/... ./internal/static/... ./internal/pipeline/...`

---

## Phase 5: Plugin + Fetch + I18n + CLI (~107 tests)

### 5A: `internal/plugin` — 58 tests
**Files**: `hooks.go`, `registry.go`, `node.go`, `wasm.go`

- **hooks.go**: Hook registry with timeout, chained execution, warnings. `HookFunc` signature is `func(ctx context.Context, payload interface{}) (interface{}, error)` — context carries timeout deadline for cooperative cancellation (issue #13). `Run()` passes `context.Background()`. `RunWithTimeout()` uses `context.WithTimeout()` and passes the derived context to each hook.
- **registry.go**: Plugin classification by file type, discovery, filter registration, conflict warnings
- **node.go**: LSP-style message encoding/decoding, bridge state management
- **wasm.go**: QuickJS/WASM runtime stubs (may need `wazero` dep — defer if complex)

### 5B: `internal/fetch` — 16 tests
**File**: `internal/fetch/fetch.go`

- REST/GraphQL fetching, file-based caching, XML/CSV parsing, GraphQL data unwrapping

### 5C: `internal/i18n` — 18 tests
**File**: `internal/i18n/i18n.go`

- Language context building, translation linking, output prefixes, per-language filtering

### 5D: `cmd/` — 15 tests
**Files**: `root.go`, `build.go`, `serve.go`, `init.go`, `version.go`

- Register Cobra flags (--config, --output, --verbose, --quiet, --port, --preview, --no-drafts, --refetch)
- `RunInit`: Create default config, error if exists
- `Version`: Set to non-empty string

**Verify**: `go test ./internal/plugin/... ./internal/fetch/... ./internal/i18n/... ./cmd/...`

---

## Phase 6: Server + SSR (~65 tests)

### 6A: `internal/server` — 40 tests
**Files**: `server.go`, `watcher.go`, `overlay.go`

- HTTP server with mode-aware behavior (dev/preview)
- File watcher with debouncing and change classification
- Error overlay injection
- `WebSocketReloadMessage()`: Return `{"type": "reload"}` JSON string for connected browser reload
- `DebounceInterval()`: Return configurable debounce interval in milliseconds for file watcher
- `DetermineRebuildAction(changedFiles []string) RebuildScope`: Classify file changes as incremental or full rebuild. Many simultaneous changes trigger a full rebuild.

### 6B: `internal/ssr` — 25 tests
**Files**: `scanner.go`, `depgraph.go`, `persistence.go`

- Custom element scanning and deduplication
- Component dependency graph
- SSR marker insertion and stamp-back
- Component map persistence

**Verify**: `go test ./internal/server/... ./internal/ssr/...`

---

## Phase 7: Integration Tests + Final (~16 tests)

### 7A: `test/integration/` — 16 tests
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

**Verify**: `go test ./... 2>&1 | grep -E "Passed|Failed"`

---

## Summary

| Phase | Packages | Est. Tests | Cumulative |
|-------|----------|-----------|------------|
| 0 | Dependencies only | 0 | 0 |
| 1 | cache, data, cascade, validation, pagination, filters | ~109 | ~109 |
| 2 | config, content | ~119 | ~228 |
| 3 | permalink, collection, template (context/layout/shortcodes), output, assets | ~75 | ~303 |
| 4 | template (liquid/go engines), static, pipeline **[WALKING SKELETON]** | ~56 | ~359 |
| 5 | plugin, fetch, i18n, cmd | ~107 | ~466 |
| 6 | server, ssr | ~65 | ~531 |
| 7 | integration tests + remaining | ~74 | ~605 |

## Key Risks

1. **Liquid engine compatibility** — Biggest unknown. Evaluate osteele/liquid vs Notifuse/liquidgo against test expectations in Phase 0.
2. **Template tag preservation in Markdown** — goldmark must preserve `{{ }}`/`{% %}`. Requires custom extension or placeholder substitution.
3. **WASM/QuickJS runtime** — May need `wazero` dependency. Most infrastructure-heavy feature. Defer if needed.
4. **Pipeline test expectations** — `pipeline.Build` tests call `Build(cfg)` without specifying content directory. Need to handle defaults or infer from config.
