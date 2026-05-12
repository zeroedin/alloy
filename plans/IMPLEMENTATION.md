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

### 1B-0: `internal/ordered` — ordered map for JSON key preservation (issue #453)
**File**: `internal/ordered/map.go`

In-house ordered map type that preserves JSON key insertion order. ~100 lines, zero dependencies. Aligned to [c0b/go-ordered-json](https://pkg.go.dev/gitlab.com/c0b/go-ordered-json) API.

- `New() *Map` — constructor
- `Get(key) interface{}` / `GetValue(key) (interface{}, bool)` — O(1) lookup
- `Set(key, value)` — O(1) insert/update, preserves insertion order for new keys
- `Delete(key)` — remove
- `Has(key) bool` — existence check
- `Keys() []string` — keys in insertion order
- `Len() int` — key count
- `Entries() []KVPair` — key-value pairs in insertion order
- `UnmarshalJSON(data) error` — uses `json.Decoder` with `Token()` to stream keys in document order. Nested JSON objects recursively create nested `*Map` instances.
- `MarshalJSON() ([]byte, error)` — emits keys in insertion order

Used by `LoadFile` and `LoadExternalFiles` for `.json` files only. Front matter, config, cascade, and collections do not use this type.

### 1B: `internal/data` — 8 tests
**File**: `internal/data/loader.go`

- `LoadFile`: Detect format by extension (.yaml/.yml, .toml, .json), parse with appropriate library. **JSON files return `*ordered.Map`** (issue #453) — the value inside `siteData["filename"]` is an `*ordered.Map` preserving key insertion order. The `LoadFile` return type stays `map[string]interface{}` for YAML/TOML; for JSON, the top-level object is an `*ordered.Map` stored as `interface{}` in the result.
- `LoadDirectory`: Walk dir, `LoadFile` each, key by filename without extension. **Stem collision detection**: Track seen stem names. If two files share a stem (e.g., `team.csv` and `team.yaml`), return an error listing both files. No silent overwrites — consistent with output path conflict philosophy (§2).
- `LoadCSV`: `encoding/csv`, first row = headers, subsequent rows = `[]map[string]string`
- **External data files (issue #271)**: `loadSiteData` in `build.go` must also load files from `cfg.Data.Files` (a `map[string]string` of key → path). For each entry, resolve the path relative to `cfg.ProjectRoot`, call `data.LoadFile`, and add the result to `siteData[key]`. Check for collisions with `data/` directory keys. File not found is a build error. Add `DataConfig` struct to `config.go` with `Files map[string]string` field.

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
- `PaginateWithTemplatePermalink(data, permalinkTemplate, as, renderer)`: Per-item pages with template URL rendering. Renamed from `PaginateWithLiquidPermalink` (issue #315). Accepts a `renderer func(templateSrc string, ctx map[string]interface{}) (string, error)` callback instead of hardcoding Liquid. The pipeline provides the callback from the configured engine (`liquid` or `gotemplate`). The `useLiquidPermalink` check in `build.go` should be renamed to `useTemplatePermalink` — detection still uses `strings.Contains(permalinkStr, "{{")` since both engines use `{{` syntax.
- **Front matter interpolation for virtual pages (issue #378)**: After `processPagination` generates virtual pages with `perPage: 1`, interpolate string-valued front matter fields that contain `{{ }}` or `{% %}` markers. In `processPagination`, after the virtual page is created (with permalink already resolved), iterate the virtual page's `FrontMatter` map. For each key-value pair where the value is a `string` containing `{{` or `{%`: render it through the same `TemplateRenderer` callback used for permalink, with `ctx = map[string]interface{}{asVar: item}`. Store the rendered result back in `vp.FrontMatter[key]`. Skip keys: `"permalink"` (already processed), `"layout"`, `"pagination"`, and any key starting with `"_"` (internal transport keys). Only applies when `perPage == 1` — paginated list pages (`perPage > 1`) do not interpolate front matter because the `as:` variable is a slice, not a single item.

### 1F: `internal/template/filters.go` — ~22 tests (standalone filter functions)
**File**: `internal/template/filters.go`

Implement all 50+ filter functions and `ApplyFilter` dispatch table. **Package-level maps (issue #363)**: `ApplyFilter` and `isBuiltinName` currently construct map literals on every call. Promote both to package-level `var` declarations — Go maps are safe for concurrent reads with no writes after init. Derive `builtinFilterNames` from the filter map at init time to eliminate sync risk. Key implementations:
- **String**: Slugify, Upcase, Downcase, Capitalize, Truncate, TruncateWords, StripHTML, Escape, Replace, ReplaceFirst, Split, Join, Strip, Append, Prepend, NewlineToBr, Contains
- **Date**: DateFormat (strftime-style via `github.com/lestrrat-go/strftime`, issue #367). Replace the hand-rolled `strftimeFormat` function with lestrrat-go/strftime for full POSIX compliance (36+ directives), pre-compiled patterns, and zero per-call allocation. `DateFormat` keeps its input parsing logic (`time.Time` vs string, no-args passthrough) but delegates format conversion to lestrrat-go. Registered in `RegisterBuiltinFilters` for both Liquid and Go template engines — overrides liquidgo's native `date` filter (which has gaps: week numbers stubbed, map-per-call allocation). Delete `strftimeFormat` function entirely.
- **Array**: Sort (numeric-aware, issue #348), Reverse, First, Last, Where, GroupBy, Size, Map, Uniq, Compact, Concat
- **`Sort` numeric awareness (issue #348)**: Update `Sort` comparison to use `isWholeNumber(v)` helper: type-switch on `int` (direct, negatives included), `float64` (no fractional part → int, negatives included), `string` (all digits → `strconv.Atoi`, negative strings NOT parsed). Both values whole numbers → compare as `int`. Either not → fall back to `toString` comparison. Nil/missing values sort to end. No new filter — `sort` itself becomes numeric-aware.
- **Set**: Intersect, Union, Complement
- **URL**: URLEncode, URLDecode, AbsoluteURL
- **Math**: Plus, Minus, Times, DividedBy (guard div-by-zero), Modulo, Ceil, Floor, Round, Abs
- **Content**: Markdownify (issue #366) — must use the same config-driven goldmark extensions and parser options as the main content renderer. `createEngine(cfg)` should initialize the shared goldmark instance from `cfg.Content.Markdown` (same `Unsafe`, `Typographer`, `AutoHeadingID` settings). Use a shared instance, not per-call allocation. Does NOT run template tag protection or code escape — processes already-rendered values. If the site sets `autoHeadingID: false`, `markdownify` also skips heading IDs.
- **Regex**: FindRE, ReplaceRE
- **Data**: JSONFilter, Default
- **Assets**: Fingerprint, SafeHTML
- **Asset fingerprinting (issue #559)**: Two new built-in filters requiring filesystem access. Both resolve paths against source directories in order: `config.Structure.Static` → `config.Structure.Assets` → `config.Structure.Content` (for co-located assets like `content/blog/post-1/hero.svg`). They cannot be plugin-provided because all plugin tiers lack filesystem access. Passthrough mappings are not searched (v1 limitation — consistent with Hugo/Zola precedent). liquidgo only supports positional args, so keyword-arg syntax from Zola (`cachebust: true`) is not available.
  - **`cachebust`**: Reads file at resolved source path, computes SHA-256 of file contents, returns `/<input>?h=<hex12>` (12-char truncated hex). Prepends `/` like the `url` filter. File not found → returns `/<input>` without hash (graceful degradation, no error). Query-string style, not filename rewriting.
  - **`get_hash`**: Reads file at resolved source path, computes digest. Positional args: `sha_type` (int: 256/384/512, default 256), `base64` (bool, default true). Returns digest string (base64 or hex). File not found → returns empty string.
  - **Registration**: These filters need the project root path and source directory names at registration time. Add a `RegisterAssetFilters(rootDir, staticDir, assetsDir, contentDir string)` function to the `template` package that creates closures capturing the path configuration, inserts them into `builtinFilters` (so `IsBuiltinFilter`/`ApplyFilter` discover them), and is callable from `createEngine(cfg)` in the pipeline package. This follows the same pattern as `RegisterBuiltinFilters` but accepts filesystem context. In tests, call `RegisterAssetFilters` with the testdata directory as root.
  - **Documented limitation**: Hash reflects pre-processed source files. If `onAssetProcess` transforms the asset after template rendering, the hash is stale.
- `RegisterBuiltinFilters`: Register all filters on engine via `AddFilter`

**Verify**: `go test ./internal/cache/... ./internal/data/... ./internal/cascade/... ./internal/validation/... ./internal/pagination/...`

---

## Phase 2: Config + Content (~119 tests)

### 2A: `internal/config` — 49 tests
**File**: `internal/config/config.go`

- `Load`: Read file, detect format by extension, unmarshal into `Config` struct. Return parse errors (not `ErrNotImplemented`) for invalid files.
- `LoadWithDefaults`: Call `Load`, then apply defaults: `Build.Output="_site"`, `Templates.Engine="liquid"`, `Content.Markdown.Goldmark.TemplateTags=true`, `Pagination.Path="page"`, `Plugins.Timeout=5000`, `Content.Formats=["md","html"]`, `Language="en"`, `Build.Clean=true`, `Structure.Content="content"`, `Structure.Layouts="layouts"`, `Structure.Assets="assets"`, `Structure.Static="static"`, `Structure.Data="data"`
- **Note**: `FeedConfig` type was removed in PR #426 (dead code). Feed generation is opt-in via template placement (see Phase 3D).
- `DetectConfigFile`: Check for `alloy.config.{yaml,yml,toml,json}` in order. Error if none found.
- `MergeFlags`: Map keys `"output"` -> `cfg.Build.Output`, `"verbose"` -> `cfg.Verbose`, `"quiet"` -> `cfg.Quiet`
- `Validate`: Check baseURL not empty and looks like a URL. Check timeout >= 0. Check title not empty (errors_test.go expects this). Error messages must contain field names.

**Testdata exists**: `internal/config/testdata/` with valid.yaml, valid.toml, valid.json, minimal.yaml, invalid.yaml

### 2B: `internal/content` — 75 tests
**Files**: `frontmatter.go`, `discovery.go`, `markdown.go`, `lifecycle.go`, `page.go`

**page.go**:
- `ToTemplateMap()`: Converts a `Page` to `map[string]interface{}` for Liquid template access (issues #66, #67). Merges FrontMatter keys first, then overlays struct fields (`url`, `date`, `summary`, `collection`, `slug`) with lowercase keys. Struct fields take precedence over same-named FrontMatter keys. Zero-value struct fields are skipped (don't override FrontMatter). Used by `buildCollectionsContext` and `TaxonomyPageContext.ToMap()` to convert `[]*Page` slices to template-accessible maps — raw `*Page` struct pointers are not reliably accessible from liquidgo due to reflection/acronym-casing issues.
- **Taxonomy collection items must use `ToTemplateMap()` (issue #328)**: In `build.go`, `buildCollectionsContext` already converts section collection pages via `ToTemplateMap()`, but taxonomy term entries are still added as raw `[]*content.Page`. Fix: iterate `tc.Terms` and convert each page slice via `ToTemplateMap()` before adding to the context. Without this, `{{ p.title }}` and `{{ p.url }}` are empty in taxonomy collection loops.

**frontmatter.go**:
- `ParseFrontMatter`: Detect delimiter (`---`/`+++`/`{`), extract and parse. **Front matter is required on content files only** — files without front matter delimiters are a build error (not "return empty map"). Error message must contain "front matter" and suggest adding empty front matter (`---\n---`). Empty front matter is valid (returns empty map, body follows). Handle edge cases: no body, malformed YAML (error must contain "yaml"). Layouts, partials, and data files do not require front matter.
- `BuildPage`: Call `ParseFrontMatter`, populate `Page` struct fields. Include source path in error messages.

**discovery.go**:
- `Discover`: Walk `contentDir`, create `Page` per .md/.html/.txt. Set `Section` from first path segment. Handle page bundles. Ignore `_data.yaml`.
- `DiscoverWithFormats`: Same but filter by allowed extensions. Returns content pages only — unchanged signature.
- `DiscoverWithPassthrough(contentDir string, formats []string) ([]*Page, []string, error)`: New function (issue #287). Same discovery as `DiscoverWithFormats` but also collects non-format files as passthrough paths. Excludes `_data.yaml`/`_data.yml`, directories, and dot-prefixed files (`.DS_Store`, `.gitkeep`, etc.). Returns content pages + passthrough relative paths. The passthrough files are copied to the output directory during Phase 3, preserving their path relative to `content/`. Non-format files must NOT be passed to `BuildPage` — they have no front matter and would error.
- **HTML front matter detection (issue #337)**: For `.html` files matching `content.formats`, three-way classification based on file content:
  1. **Has front matter** (`---`, `+++`, `{`) → call `BuildPage` normally
  2. **No front matter + has `<!DOCTYPE` or `<html`** → full document, add to passthroughs (copy as-is)
  3. **No front matter + no DOCTYPE/html** → fragment, create `Page` with empty `FrontMatter` and entire file as `Body`. Inherits layout from `_data.yaml` cascade.
  
  Detection: read first bytes, check for front matter delimiters first. If no delimiters, check for `<!DOCTYPE` or `<html` followed by `>`, ` `, or newline (case-insensitive, may have leading whitespace). Bare `<html` prefix without boundary must NOT match. `BuildPage` is bypassed for fragments — construct `Page` directly with empty `FrontMatter`. `.md` files always require front matter.
  
  This classification must apply consistently across both `DiscoverWithPassthrough` and `DiscoverWithFormats` — extract shared classification logic so all discovery entry points behave identically.
  
  Default `content.formats`: `["md", "html"]`. `.liquid` is a valid content format only when engine is `liquid` — when engine is `gotemplate`, `.liquid` files are always passthrough even if in formats list.

**markdown.go**:
- `RenderMarkdown`: Configure goldmark with extensions (tables, task lists, typographer, footnotes). Handle `Unsafe` (raw HTML passthrough). Handle `TemplateTags` via a custom goldmark extension (issue #564) — see **Template tag goldmark extension** below. Handle `TemplateBlocks` (issue #202) via the same extension's block parser. NO placeholder substitution — the `protectTemplateTags`/`restoreTemplateTags` functions, `blockShortcodeLineRe` regex, and all placeholder logic must be removed and replaced by proper goldmark parser extensions.

  **Template tag goldmark extension** (issue #564):

  **Custom AST node types** (in `markdown.go`):
  - `TemplateTagInline` — embeds `ast.BaseInline`, stores `TagText []byte`. `Kind()` returns `KindTemplateTagInline`. `IsRaw()` returns `true`.
  - `TemplateTagBlock` — embeds `ast.BaseBlock`, stores `TagText []byte`. `Kind()` returns `KindTemplateTagBlock`. `IsRaw()` returns `true`.

  Both use custom AST node kinds (NOT `ast.RawHTML`) so template tags are preserved regardless of the `unsafe` setting.

  **Inline parser** (`templateTagInlineParser`, implements `parser.InlineParser`):
  - `Trigger()` returns `[]byte{'{'}`.
  - `Parse()` checks for `{{` or `{%` at position, scans for matching `}}` or `%}` (including `{%-`/`-%}` whitespace-trimming variants). On match: creates `TemplateTagInline` with tag text, advances reader. Handles multiline tags via line advancement (for `{{ "hello\nworld" | filter }}`).
  - Priority: 90.

  **Block parser** (`templateTagBlockParser`, implements `parser.BlockParser`):
  - `Trigger()` returns `[]byte{'{'}`.
  - `Open()` matches a line containing EXACTLY ONE `{% ... %}` tag with only whitespace before/after. Must NOT match lines with multiple tags or mixed content (e.g., `{% if %}Visible{% endif %}` is NOT a block match — it contains text between tags). Creates `TemplateTagBlock`, returns `(node, NoChildren)`.
  - `Continue()` returns `parser.Close` (single-line block).
  - `CanInterruptParagraph()` returns `true`.
  - `CanAcceptIndentedLine()` returns `false`.
  - Priority: 50.

  **Custom renderer** (`templateTagRenderer`, implements `renderer.NodeRenderer`):
  - Registers for `KindTemplateTagInline` and `KindTemplateTagBlock`.
  - Inline: writes `node.TagText` verbatim. No HTML escaping, no `unsafe` check.
  - Block: writes `node.TagText` + `\n` verbatim. No `<p>` wrapping.
  - Priority: 100. Follows pattern in `render_hooks.go`.

  **Extension** (`templateTagsExtension`, implements `goldmark.Extender`):
  - `Extend()` adds inline parser, block parser, and renderer.
  - Registered in `CreateGoldmark()` when `opts.TemplateTags` is `true`.

  **Code to remove**:
  - `protectTemplateTags()` function
  - `restoreTemplateTags()` function
  - `blockShortcodeLineRe` regex
  - `preprocessSource()` reduced to only the escape path (or removed, with escape logic inlined)
  - All placeholder logic in `RenderMarkdown()` and `RenderMarkdownWithTOC()`

  **What remains unchanged**:
  - `escapeTemplateTags()` and `templateTagPattern` (used for `templateTags: false` path)
  - `CreateGoldmark()` signature and `MarkdownOptions` struct
  - All render hook logic in `render_hooks.go`
  - `escapeTemplateTagsInCode()` in `build.go` (runs after markdown rendering, independent)

  **TOC integration**: `extractText()` in `markdown.go` only collects `ast.Text` nodes for TOC entries. With the new extension, `TemplateTagInline` inside headings (e.g., `## {{ page.section_title }}`) must also contribute to TOC text. Update `extractText` to write `node.TagText` for `TemplateTagInline` nodes.
- `RenderText`: Wrap in `<pre>` tags.
- **Auto heading IDs + heading attributes (issue #274, #306)**: Add `AutoHeadingID bool` to `MarkdownOptions` (default true from `cfg.Content.Markdown.AutoHeadingID`). When true, enable `parser.WithAutoHeadingID()` and `parser.WithAttribute()` in goldmark parser options. When false, skip both — headings render without `id` attributes. These are goldmark core options, not extensions. Heading attributes (`{#custom-id .class}`) only work when `AutoHeadingID` is true.
- **TOC extraction (issue #274)**: Add a `TOC` field to `MarkdownOptions` (bool, default true from `cfg.Content.Markdown.TOC`). When true, `RenderMarkdown` walks the goldmark AST after parsing (before HTML rendering) and collects heading nodes (h2-h6, excluding h1) into a nested `[]TOCEntry` structure. `TOCEntry` has `ID`, `Text`, `Level`, `Children`. Returns the TOC alongside the rendered HTML — change return to `([]byte, []TOCEntry, error)` or add a `TOCResult` wrapper. The pipeline stores the result on `page.TOC` so templates can access `page.toc`. The `onContentTransformed` hook receives the page with `TOC` populated — plugins can mutate it.
- **Render hooks (issues #273, #310, #311)**:
  
  **MarkdownOptions changes**: Add `Hooks map[string]string` (hook name → template source) and `HookRenderer func(templateSrc string, ctx map[string]interface{}) (string, error)` callback. The `content` package cannot import `template` (circular dependency), so the pipeline provides the renderer callback. When `HookRenderer` is nil, hooks in the `Hooks` map are ignored (no fallback to a bare Liquid environment).
  
  **Discovery (`internal/template/hooks.go`, issue #311)**: `DiscoverRenderHooks(layoutsDir string, engine string) (map[string]string, error)` scans `layouts/_markup/` for `render-{type}.{ext}` files. Extension matches the configured engine (`.liquid` for liquid, `.html` for gotemplate). Returns a map: `"codeblock" → templateSource`, `"codeblock-mermaid" → templateSource`, `"link" → templateSource`, etc. Missing `_markup/` directory is not an error (returns empty map). Unrecognized filenames are silently ignored. Valid hook names: `blockquote`, `codeblock`, `codeblock-{language}`, `heading`, `image`, `link`, `table`.
  
  **Pipeline wiring (issue #311)**: In `renderPages()` and the standalone `mdOpts` construction in `Build()`, call `tmpl.DiscoverRenderHooks(layoutsDir, engineName)` once at the start. Set `mdOpts.Hooks` to the returned map. Set `mdOpts.HookRenderer` to a closure that wraps `engine.Parse()` + `tpl.Render()`. Parsed templates should be cached (parse once, render per node).
  
  **Goldmark integration**: For each hook type in `opts.Hooks`, register a custom `renderer.NodeRenderer` that builds the `markup.*` context from the AST node and calls `opts.HookRenderer(templateSrc, ctx)`. Context: `inner` from rendered children, `language` from fenced code block, `level` from heading, `id` from slugified heading text, `src`/`alt`/`title` from image, `destination`/`text`/`title`/`is_external` from link.

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
- `GenerateTaxonomyPages`: Index + per-term pages with configured permalinks. **Skip when `TaxonomyConfig.Render == false` (issue #319)**. Also skip `DetectDuplicateTermSlugs` for non-rendered taxonomies.
- `BuildTaxonomyPageContext`/`DetectDuplicateTermSlugs`
- **Taxonomy namespace separation (issue #333)**: `buildCollectionsContext` must NOT add taxonomies to the `collections` map. Taxonomies get their own top-level template variable `taxonomies`. In `BuildTemplateContext`, add `taxonomies` alongside `collections`, `page`, `site`. Templates access `taxonomies.tags.about` not `collections.tags.about`. This prevents collisions between section names and taxonomy names (e.g., a `content/taxonomies/` directory).
- **`TaxonomyConfig.Render bool` (issue #319)**: Add `Render bool` field to `TaxonomyConfig` with `yaml:"render"`. Default must be `true` (set in `ApplyDefaults`). When `false`, `generateTaxonomyPages` in `build.go` skips that taxonomy entirely — no layout resolution, no page generation, no duplicate slug check. The taxonomy data is still built by `BuildTaxonomies` and available in `taxonomies.*`.

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
- `RenderTemplate`: Parse + render, wrap errors with source path. **Must use a cached environment** (issue #364) — currently creates a new `liquid.Environment` and registers standard tags per call. Use `sync.Once` to initialize a package-level default environment, or eliminate `RenderTemplate` entirely by passing the engine to all callers. The `rc.Engine != nil` guard in `renderPages` (build.go:1272) should always be true — engine is created by `createEngine(cfg)` which never returns nil. The `else` branch using `RenderTemplate` is dead code and should be removed. The `Server.RenderPage` path (server.go:401) should use the engine instance held by the server, not a bare `RenderTemplate` call — otherwise plugin filters and custom tags are unavailable in dev mode on-demand renders.
- Must support: `{{ var }}`, `{{ page.title }}`, `{% if %}`, `{% for %}`, `{% assign %}`, `{{ content }}` injection, `{% include %}`, filter pipelines

### 4B: Go Template Engine (~12 tests)
**File**: `internal/template/gotemplate.go`

- Adapt `html/template` to `TemplateEngine` interface via `FuncMap`
- **`*ordered.Map` compatibility (issue #458)**: Go templates use reflection — `*ordered.Map` is a struct, not a map or slice, so neither `{{ index }}` nor `{{ range }}` work natively. Converting to `map[string]interface{}` enables property access but loses iteration order; converting to `[]KVPair` enables ordered iteration but breaks key-based lookup. These are mutually exclusive on the same value. Fix: keep `*ordered.Map` as the context value and register FuncMap helpers in the Go template engine:
  - **`oget`** (`{{ oget .site.data.tokens "white" }}`): calls `m.Get(key)` on `*ordered.Map`, returns the value. Falls back to `index` for regular maps.
  - **`orange`** (`{{ range orange .site.data.tokens }}`): calls `m.Entries()` on `*ordered.Map`, returns `[]KVPair` for ordered iteration. Each entry has `.Key` and `.Value`.
  
  Register both in `goEngine.AddFilter` or directly in the FuncMap during engine creation. The `*ordered.Map` in `siteData` is never mutated. Liquid uses it directly via `Each` and `LiquidMethodMissing`.

### 4C: `internal/static` — 10 tests + 12 passthrough filtering tests (issue #547)
**File**: `internal/static/copy.go`

- `CopyStatic`/`CopyPassthrough`: Walk and copy preserving structure
- `CopyPassthroughWithValidation(mappings, projectRoot, outputDir, managedDirs)`: Same as `CopyPassthrough`, but silently skips any mapping where the `from` path resolves to a managed directory (content, layouts, assets, static, data). Prevents passthrough configs from accidentally overwriting managed content.

**Passthrough filtering (issue #547):**

- `PassthroughMapping.Exclude []string` — optional, gitignore-style exclude patterns added to `internal/config/config.go`
- New dependency: `github.com/bmatcuk/doublestar/v4` — glob matching with `**`, `{a,b}`, `[chars]` support
- `normalizeExcludePattern(pattern string) string` — gitignore-style normalization: patterns without `/` get `**/` prepended (match at any depth); patterns ending with `/` get `**` appended (match directory tree); patterns with `/` pass through as-is
- `matchExclude(patterns []string, relPath string) bool` — returns true if `relPath` matches any normalized exclude pattern via `doublestar.Match`. Called during file walk or glob iteration to skip excluded files.
- `globRoot(pattern string) string` — returns the longest directory prefix of a glob pattern before any metacharacter (`*`, `?`, `[`, `{`). `elements/**/*.js` → `elements`. Used to compute relative output paths.
- `containsGlobChars(path string) bool` — returns true if path contains any glob metacharacter. Used to branch between glob and directory-walk modes.
- `CopyPassthrough` updated: if `from` contains glob chars → resolve `from` relative to `projectRoot`, use `doublestar.Glob` to find matching files, compute each file's relPath from `globRoot`, filter by `matchExclude`, copy to `outputDir/to/relPath`. If `from` is a plain directory → existing `filepath.Walk` behavior with `matchExclude` filter added during walk.
- `copyDirConcurrent(src, dst string, excludes []string)` — add `excludes` parameter; during `filepath.Walk`, compute each file's relative path from `src` and skip if `matchExclude(excludes, relPath)` returns true. Callers without excludes pass `nil`.
- `RecopyPassthroughFile` in `internal/server/watcher.go` — check exclude patterns before recopying; if `from` is a glob, also verify the changed file matches the `from` glob. Return error (or skip indicator) for excluded/non-matching files.

### 4D: `internal/pipeline` — 19 tests
**File**: `internal/pipeline/build.go`

- `BuildWithContent(cfg, contentMap, opts ...BuildOptions)`: Thin wrapper around `Build()`. Writes `contentMap` entries to a temp directory preserving path structure (e.g., `"content/index.md"` → `tmpDir/content/index.md`, `"layouts/default.liquid"` → `tmpDir/layouts/default.liquid`), sets `cfg.ProjectRoot = tmpDir`, and calls `Build(cfg, opts...)`. This ensures every pipeline stage runs — plugins, hooks, data cascade, collections, lifecycle filtering, layout chaining, SSR, validation, output. Zero divergence from `Build()`. The temp directory is cleaned up after `Build()` returns. `BuildWithContent` must NOT duplicate any pipeline logic — it is purely file setup + delegation (issue #283).
- `BuildIncremental(cfg, contentMap, previousCache, changedFiles)`: Dev-mode incremental rebuild. Accepts a previous `*cache.Cache` (loaded by the caller, not by this function) and list of changed file paths. Discovers all pages, skips pages where `cache.ShouldSkipFile` returns true (unchanged), and renders pages where it returns false (changed) or that were invalidated via `cache.InvalidatedPages` for layout changes. Returns `BuildResult` with `PagesSkipped` count. When `previousCache` is nil, renders all pages (equivalent to full build).
- `BuildResult.PagesSkipped int`: Number of pages skipped via cache comparison during incremental rebuild. Always 0 for `Build` and `BuildWithContent` (full rebuild).
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
- **Render ordering** (issue #10): Markdown renders first, then template tags — per spec §6 steps 3-4. Goldmark's TemplateTags extension preserves `{{ }}`/`{% %}` through markdown rendering. After markdown rendering and before Liquid processing, `escapeTemplateTagsInCode` converts template tags inside `<code>` elements to HTML entities so Liquid ignores them (issue #46). **Only run on `.md` files (issue #352)** — `.html` and `.liquid` content may have Liquid expressions inside `<code>` that should be interpolated, not escaped. Move the `escapeTemplateTagsInCode` call inside the `.md` case, not after the switch. Markdown errors use stage name `"content transformation"`, template errors use `"template rendering"`.

#### `BuildOptions` (issue #264)

`Build()` accepts an optional `BuildOptions` to control pipeline behavior without changing config:

```go
type BuildOptions struct {
    SkipSSR bool // true = skip Phase 2 entirely, regardless of cfg.SSR
}

func Build(cfg *config.Config, opts ...BuildOptions) (*BuildResult, error)
```

The variadic pattern keeps existing callers working (`Build(cfg)` still compiles). When `opts` is provided and `SkipSSR` is true, the pipeline skips Phase 2 entirely and sets `result.SSRSkipped = true`. The SSR check becomes:

```go
ssrSkipped := cfg.SSR == nil || (len(opts) > 0 && opts[0].SkipSSR)
```

`cmd/dev.go` always passes `pipeline.BuildOptions{SkipSSR: true}` — for both the initial `Build()` and watcher `BuildIncremental()` calls. `cmd/build.go` and `cmd/serve.go` call `Build(cfg)` with no options (SSR runs if configured). `BuildIncremental` gets the same variadic parameter.

#### `Build()` full orchestration (issue #30)

`Build()` must orchestrate all pipeline stages from §2. Currently it stops after markdown+template rendering. The individual packages for each stage are implemented and pass tests — they need to be called in order:

```
 1. config.ApplyDefaults(cfg)                           ✅ done
 2. validateOutputDir(cfg)                               ✅ done
 3. content.DiscoverWithFormats(contentDir, formats)      ✅ done
 4. content.FilterByLifecycle(pages, now, includeDrafts)  ✅ done (issue #108: must pass includeDrafts from server mode, not hardcode false)
 5. cascade.LoadDirectoryCascade + FindCascadeData + PageContext ✅ done (was step 6, reordered for #302)
 6. permalink.ResolveFromCascade(page, cascadeData)       ← CHANGED (issue #302)
    Permalink pattern comes from _data.yaml cascade (step 5), not cfg.Permalinks.
    Remove `Permalinks map[string]string` from Config struct.
    Remove `ResolveForSection` — replaced by `ResolveFromCascade`.
    Update all callers in build.go (batch loop, i18n prefix logic).
 7. data.LoadDirectory(dataDir) → siteData                ✅ done
 8. collection.BuildCollections(pages, cascadeData)       ← CHANGED (issue #302)
    Date-based section detection reads permalink from each section's
    _data.yaml cascade instead of cfg.Permalinks map.
    `isDateBasedSection` in layout.go also needs cascade data.
 9. collection.BuildTaxonomies(pages, taxonomies)         ✅ done
10. template.RegisterBuiltinFilters(engine)               ✅ done
11. renderPages (markdown → template tags)                ✅ done
12. template.ResolveLayout(page, layoutsDir, engine)      ✅ done
12a. Layout chaining (issue #276)                          ← MISSING
     After resolving the initial layout, render page content through it,
     then check the layout file for front matter `layout:` directive.
     If present, resolve the parent layout and render again with the
     previous result as `{{ content }}`. Repeat until a root layout
     (no `layout:` front matter) is reached. Max depth: 10 levels.
     Strip layout front matter before rendering (must not appear in output).
     Call `DetectCircularLayouts(layoutsDir)` once during Phase 0.
12b. cache.TrackTemplateUsage(page.RelPath, normalizedLayoutPath)   ← MISSING (issue #229)
     layoutPath must be relative to project root and slash-normalized (filepath.ToSlash),
     e.g. "layouts/default.liquid" — not an absolute filesystem path.
     For chained layouts, track ALL layouts in the chain (not just the innermost).
13. Render page through layout chain ({{ content }} injection)  ✅ done (single level only, chaining missing)
14. output.ComputeOutputPath(page) → output path          ✅ done
15. output.WriteFile(outputPath, html)                    ✅ done
16. static.CopyStatic(staticDir, outputDir)               ✅ done — **concurrent file copies (issue #511)**
17. assets.CopyAssets(assetsDir, outputDir)                ✅ done — **concurrent file copies (issue #511)**
18. static.CopyPassthroughWithValidation(...)             ✅ done — **concurrent file copies (issue #511)**
18b. Copy content-colocated passthrough files (issue #300)  ← MISSING

**Static/asset copy runs as its own pipeline stage (issue #507, #511)**: Steps 16-18b run during Phase 3 (output), after all content rendering and hooks complete. The copy stage does NOT overlap with rendering or hook execution (#507). File copies within the stage use a bounded worker pool of `runtime.NumCPU()` goroutines (#511) — walk the source directory, create directories synchronously, dispatch file copies to the pool via a semaphore channel. First error cancels remaining work. Benchmark: 60% reduction in copy time (4.8s → 1.9s on 7,947 files).
     `Build()` step 3 must switch from `DiscoverWithFormats` to `DiscoverWithPassthrough`.
     The returned passthrough paths are carried through the pipeline and copied
     to outputDir preserving their relative path: `content/about/diagram.svg`
     → `_site/about/diagram.svg`. Use `static.CopyFile(src, dst)` or equivalent.
     Add `ContentPassthroughs []string` to `BuildResult` — relative paths of
     non-content files copied from `content/` to output.
     In dev mode (alloy dev), skip the copy — files are served from source.
19. output.GenerateSitemap(pages, baseURL, outputDir)     ✅ done
20. cache.SaveTo(cacheFile)                               ✅ done
```

**Key implementation notes:**
- Steps 5-9 happen before rendering (step 11) so templates can access `page.url`, `collections.*`, filters, etc.
- **Multi-format output (issue #71)**: Steps 12-15 (layout resolution → render → compute path → write) must loop over `page.Outputs` when present. Content rendering (step 11) happens once; layout rendering happens per format. See Phase 3D wiring guidance.
- Step 12-13 happen after step 11: content is rendered first, then injected into the layout via `{{ content }}`.
- **Layout chaining (issue #276)**: After rendering content through the initial layout, check the layout file for front matter with a `layout:` directive using `extractLayoutParent()`. If a parent is found, resolve it via `ResolveLayout` (using the parent name), render the current result as `{{ content }}` into the parent, and repeat. Loop until a root layout (no `layout:` in front matter) is reached, or max depth (10) is exceeded. Strip front matter from layout content before parsing/rendering. Call `DetectCircularLayouts(layoutsDir)` once after layout discovery (Phase 0) to fail fast on cycles. Track all layouts in the chain for cache invalidation (`cache.TrackTemplateUsage` for each level).
- Steps 15-20 are post-render: write files, copy assets, generate sitemap, persist cache.
- If content directory doesn't exist or is empty, `Build()` should return a successful zero-page result (not error). This is required for `alloy init && alloy build` to work and for cmd tests to pass.
- **Unified pipeline (issue #280)**: `Build()` must have ONE code path, not separate multi-language and single-language forks. A site without `languages:` config produces a single language batch with defaults (`{code: cfg.Language, root: true}`). The pipeline always:
  1. Builds language batches (1 for single-language, N for multi-language)
  2. Pass 1: discover + render content per batch (steps 3-11)
  3. `LinkTranslations` between passes (no-op for single batch)
  4. Pass 2: layout resolution + rendering per batch (steps 12-15)
  5. SSR, output, static copy (shared, after all batches)
  
  This eliminates the current `if len(cfg.Languages) > 0 / else` fork that duplicates content discovery, lifecycle filtering, permalink resolution, cascade, collections, taxonomy, layout rendering, and layout chaining logic. Every feature (layout chaining #276, progress reporting #255, BuildOptions #264) is wired once.
  
  `BuildWithContent()` delegates to `Build()` entirely (issue #283) — no separate engine, no duplicate logic.
  
  Helper functions to extract: `renderPageThroughLayouts(page, layoutChain, engine, ctx)`, `generateTaxonomyPages(taxonomies, engine, cfg, ...)`.
  
  The two-pass design ensures `page.Translations` is populated before layout templates render, enabling `{% for trans in page.translations %}` for hreflang tags.
- **Hook priority (issue #464)**: `alloy.hook(name, options, fn)` — options is required, second argument. The `priority` field in options controls execution order (default 50, lower runs first). Changes needed:
  - **JS bridge**: `alloy.hook()` in bridge.js captures the second argument as options (object with `priority`, `data`, `pages`, `pageFields`). `__registerHook` callback sends `{name, priority, scope}` where scope is the JSON-serialized options (minus priority). Default priority to 50 if omitted.
  - **QuickJS/Node runtime**: `RegisteredHooks()` returns `[]HookRegistration{Name, Priority, Scope}` instead of `[]string`. Scope is the `HookScope` derived from the JS options object.
  - **HookRegistry**: Add `RegisterWithPriority(event, fn, priority int)`. `Register(event, fn)` delegates to `RegisterWithPriority(event, fn, 50)`. `hooks` field changes from `map[HookName][]HookFunc` to `map[HookName][]priorityHook` where `priorityHook` has `fn HookFunc`, `batchFn BatchHookFunc`, `priority int`, `index int`, and `scope *HookScope`. Use insertion sort at registration time to maintain sorted order — do NOT sort per-call in `Run`/`RunWithTimeout` (those fire per-page, sorting per-call is wasteful). Stable insertion preserves registration order for same-priority hooks.
  - **registry.go**: `loadPluginRuntime` passes priority and scope from `RegisteredHooks()` into `hooks.RegisterWithOptions(event, fn, scope, priority)`.
- **Declarative hook payload scoping (issue #528)**: Plugins must declare what data they need at registration time via the required options object. The pipeline uses these declarations to serialize only the requested subset — eliminating redundant full-site serialization passes.
  - **Go types** (all in `internal/plugin/hooks.go`):
    - `PagesScopeMode` — `int` type with iota constants: `PagesScopeNone` (zero value — skip pages), `PagesScopeAll` (all pages), `PagesScopeGlob` (path glob filter), `PagesScopeTaxonomy` (taxonomy term filter).
    - `PagesScope` — struct with `Mode PagesScopeMode`, `Glob string` (only when Mode == PagesScopeGlob), `Taxonomies map[string][]string` (only when Mode == PagesScopeTaxonomy). Multiple terms within same taxonomy are OR'd (union). Multiple taxonomies are AND'd (intersection). Matches `TaxonomyCollection.Terms map[string][]*content.Page` in `collection/taxonomy.go`.
    - `HookScope` — struct with `Data []string` (siteData keys; nil = skip; `["*"]` = all), `Pages PagesScope` (page filtering mode), `PageFields []string` (per-page fields; nil = all; `["*"]` = all).
  - **`RegisterWithOptions(event HookName, fn HookFunc, scope HookScope, priority int)`**: Same insertion sort as `RegisterWithPriority` but stores `&scope` on the `priorityHook`. Existing `Register`/`RegisterWithPriority`/`RegisterBatchWithPriority` remain unchanged — they set `scope` to `nil` (unscoped = serialize everything, backward compatible).
  - **`RegisterBatchWithOptions(event HookName, fn HookFunc, batchFn BatchHookFunc, scope HookScope, priority int)`**: Same as `RegisterBatchWithPriority` but stores `&scope` on the `priorityHook`.
  - **`ScopeFor(event HookName) []*HookScope`**: Returns scopes for all hooks registered on an event, in priority order (matching execution order). Nil entries for hooks registered without scope (via `Register`/`RegisterWithPriority`). Returns `nil` (not empty slice) if no hooks registered for the event. The pipeline calls `ScopeFor` at each serialization site to compute the union of requested data and serialize only that subset.
  - **`ValidateScope(event HookName, scope HookScope) error`**: Validates that the scope is compatible with the hook's position in the pipeline. Two validation rules:
    1. **Pageless hooks** (`OnConfig`, `OnBeforeValidation`, `OnAfterValidation`, `OnDataFetched`) do not receive pages. Any `Pages.Mode` other than `PagesScopeNone` is rejected with an error mentioning "pages". These hooks receive per-build payloads (config, paths, data) — page filtering is meaningless.
    2. **Pre-taxonomy hooks** that receive pages (`OnPagesReady`) reject `PagesScopeTaxonomy` with an error mentioning "taxonomy" — taxonomy indices are not built yet at step 9.
    
    Post-taxonomy hooks that accept all page scope modes: `OnContentLoaded`, `OnDataCascadeReady`, `OnContentTransformed`, `OnPageRendered`. `PagesScopeNone` is valid on all hooks. Called at plugin load time — errors surface before the build starts.
  - **`addPages` return shape**: When a hook declares `pages: false` and injects virtual pages (e.g., `onPagesReady` generating pages from site data), the return value uses `{ addPages: [...] }` instead of returning a full pages array. The pipeline detects the `addPages` key and appends the new pages without requiring the plugin to round-trip all existing pages. Same validation as the existing virtual page injection path: `path` and `url` required, output-path collision check.
  - **Scope-aware serialization**: Each serialization site in the pipeline (`runOnPagesReady`, `serializePagesForHook`, `fireContentTransformedHooks`, etc.) calls `registry.ScopeFor(event)` to get all scopes, computes the union of requested fields across all hooks for that event, and serializes only the union. If any scope requests `Data: ["*"]`, all site data is serialized. If any scope requests `Pages.Mode == PagesScopeAll`, all pages are serialized. If all scopes have `Pages.Mode == PagesScopeNone`, no pages are serialized. Glob and taxonomy filters are applied after the union computation — pages matching ANY hook's filter are included. `PageFields` union determines which fields are populated on each `HookPagePayload`. **Union scope is not an isolation boundary (issue #543)**: The union means all hooks on the same event receive the same superset payload — if hook A requests `["html"]` and hook B requests `["toc"]`, both see `["html", "toc"]`. This is a performance optimization (one serialization per event, not per hook), not a security or privacy mechanism. Plugin authors must not rely on scoping to hide fields from other plugins. **Taxonomy term dedup (issue #546)**: When merging `PagesScopeTaxonomy` scopes, `computeUnionScope` must deduplicate terms per taxonomy key. Two hooks scoping `tags: ["go"]` must produce `tags: ["go"]`, not `tags: ["go", "go"]`. Use a set during merge.
  - **Bridge updates**:
    - **QuickJS** (`wasm.go`): `alloy.hook(name, opts, fn)` at line 96-99 — swap second and third arguments. `__registerHook` Go callback at line 71-79 — receives `(name, priority, scopeJSON)` instead of `(name, priority)`. Parse `scopeJSON` into `HookScope`. Scope validation happens later in `registry.go` during `registerRuntime()`, not in the `__registerHook` callback itself. **QuickJS duplicate hook detection (issue #558)**: `__registerHook` callback must detect duplicate hook names (checked against `r.hooks` map) and append a warning to `evalWarnings`. Warning format matches Node bridge: `"duplicate hook registration: \"<name>\" registered multiple times, last registration wins"`. `EvalWarnings() []string` accessor returns accumulated warnings. Last-wins semantics preserved (map key overwrites). **EvalWarner forwarding (issue #562)**: `registerRuntime` in `registry.go` must check if the runtime implements `EvalWarner` and, if so, forward accumulated `EvalWarnings()` to `HookRegistry.warnings` with a `"plugin <name>: "` prefix. This surfaces QuickJS duplicate warnings through the same `HookRegistry.Warnings()` API that surfaces timeout warnings. **Both `__registerHook` code paths (issue #562)**: The duplicate detection check (`r.hooks[name]` existence) must fire before the `len(args) >= 2` branch, so it applies to both the 3-arg path (priority + scope from `alloy.hook()`) and the 1-arg fallback path (default priority 50, no scope). Test fixture `duplicate-hook-no-scope.js` exercises the 1-arg path directly via `__registerHook("hookName")` calls.
    - **Node** (`node.go`): Hook registration in eval response includes scope JSON alongside hook name and priority. `loadPluginRuntime` parses scope and calls `RegisterWithOptions`. **Eliminate double JSON round-trip (issue #545)**: `EvalFile` currently marshals the Go `map[string]interface{}` scope to JSON via `json.Marshal`, then immediately parses it back via `parseScopeJSON`. Refactor: extract the type-switch logic from `parseScopeJSON` into `parseScopeMap(m map[string]interface{}) (*HookScope, error)` which builds `HookScope` directly from a Go map via type assertions. `parseScopeJSON` becomes a thin wrapper: unmarshal JSON → call `parseScopeMap`. `EvalFile` calls `parseScopeMap` directly, eliminating the round-trip. One code path for the polymorphic `pages` handling. **Warn on duplicate hookScope clobber (issue #544)**: Detection must happen in `bridge.js` because JS objects deduplicate keys before serialization — Go's `EvalFile` receives pre-deduplicated `hookScopes` and cannot detect within-plugin duplicates. In `bridge.js`, the `hook()` method must check if `hooks[name]` already exists before overwriting. If so, push a warning string (e.g., `"duplicate hook registration: <name> — last registration wins"`) to a module-level `warnings` array. The eval response must include `warnings: warnings` alongside `filters`, `shortcodes`, `hooks`, and `hookScopes`. In `node.go`, add `evalWarnings []string` field to `NodeRuntime` and `EvalWarnings() []string` accessor. `EvalFile` parses the `warnings` array from the eval response and appends each string to `r.evalWarnings`. Last-wins semantics are preserved — the warning is informational, not an error. **Defensive hooks dedup (issue #555)**: `r.hooks` must not contain within-plugin duplicates. bridge.js already prevents this via `Object.keys(hooks)` (JS objects have unique keys), but Go must defensively deduplicate as a protocol invariant guard. `RegisteredHookDetails()` must deduplicate by hook name when building the output slice (use a seen-set; skip names already emitted). This ensures one `HookRegistration` per unique hook name regardless of how duplicates enter `r.hooks`. Cross-plugin duplicates (different plugins registering the same hook) are intentional and handled by `HookRegistry` — this dedup applies only within a single runtime's accumulated hooks.
- **Plugin filter bridging (issue #93)**: After `registry.LoadPlugins(hooks)` (step 0) and engine creation (step 10), bridge plugin-discovered filters to the template engine. For each filter name from `LoadPlugins()`, call `engine.AddFilter(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallFilter()`. This must happen before content rendering (step 11) so templates can use plugin filters. Similarly, `alloy.hook()`/`alloy.on()` registrations discovered by `EvalFile()` must be wired into the `HookRegistry` during `LoadPlugins()`.
- **CallFilter must pass args to JS (issue #318)**: `QuickJSRuntime.CallFilter` currently sets `__callInput` from `input` but ignores `args`. Fix: serialize `args` as a JS array global (`__callArgs`), then invoke `__filters[__callFilterName](__callInput, ...__callArgs)` using spread syntax. Each arg must be type-switched (string/int/float64/bool/map/slice) and converted to a QuickJS value. Maps and slices should be JSON-serialized and parsed in JS.
- **Plugin site data access (issue #317, wiring #339)**: `QuickJSRuntime` already implements `SetSiteData(data map[string]interface{})` which JSON-serializes `data` and sets it as `alloy.data` in the JS context. **Pipeline wiring (issue #339)**: After `loadSiteData(cfg)` AND external source fetching complete, inject `siteData` into each runtime. Either add `SetSiteData` to the `Runtime` interface, or use a type assertion (`if sdr, ok := rt.(SiteDataReceiver); ok { sdr.SetSiteData(siteData) }`). This must happen before content rendering (step 11) and after ALL data sources (local + external) are merged. Without this call, `alloy.data` is `undefined` in plugins. For Node (Tier 3), send site data as a `{type: "siteData", payload: data}` message via the bridge. `alloy.data` is read-only — changes in JS do not propagate back to Go.
- **`{% inline %}` tag (issue #288, wiring #295)**: `createEngine()` in `build.go` must call `tmpl.RegisterInlineTag(engine)` after `RegisterBuiltinFilters(engine)`. Without this, `{% inline %}` works in unit tests but fails with "unknown tag" in actual builds. Register `inline` as a custom tag via `engine.AddTag("inline", inlineTagFunc)`. The tag function:
  1. Extracts the path argument (first positional arg, string)
  2. Validates path starts with `./` or `../` — error if absolute
  3. Checks file extension against an allowlist (`.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`) — error with guidance for binary types
  4. Resolves the path relative to the current content file's directory (passed via render context as `_contentDir`)
  5. **Sandboxes** the resolved path: `filepath.Rel(contentRoot, resolved)` must not start with `..`. Rejects paths that escape the content directory (e.g., `../../../../etc/passwd`). The content root is passed via `_contentRoot` in the render context.
  6. Reads the file and returns raw contents (no template processing)
  
  The template context must include both keys before rendering each page: `ctx["_contentDir"] = filepath.Dir(filepath.Join(contentDir, page.RelPath))` and `ctx["_contentRoot"] = contentDir`. These are internal context keys (prefixed with `_`) — accessible in templates but unsupported/unstable.
- **Plugin shortcode bridging (issue #139)**: Same pattern as filter bridging. After engine creation, iterate `rt.RegisteredShortcodes()` and call `engine.AddTag(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallShortcode(name, args, innerContent)`. Both inline and block shortcodes must be supported. `CallShortcode()` is currently a stub (returns input unchanged) and must be implemented to actually invoke the JS shortcode function.
- **Plugin filters in `{% include %}` partials (issue #376)**: Plugin-registered filters fail with "undefined filter" when used inside `{% include %}` partials. Root cause: alloy's `Parse()` method rewrites novel filter names (e.g., `{{ x | tokenType }}` → `{{ x | plugin_filter: "tokenType" }}`) before liquidgo parses the template. But when liquidgo encounters `{% include %}` at render time, it calls `alloyFileSystem.ReadTemplateFile()` to read the partial source, then parses it internally — bypassing alloy's `rewriteFilterToPlugin` step. The partial source is parsed as-is, so plugin filter names are unknown to liquidgo. **Fix**: `alloyFileSystem` must have access to the `dynamicFilters` map. `ReadTemplateFile` must apply `rewriteFilterToPlugin` for each dynamic filter before returning the source string. This ensures partials go through the same pre-processing as content templates and layouts.
- **Plugin filter shadowing (issue #140)**: When a plugin registers a filter with the same name as a built-in liquidgo filter (e.g., `reverse`), the plugin's version must take precedence. Per spec §4: "the last one loaded wins." The current implementation fails because `knownLiquidFilters` prevents plugin filters from being treated as dynamic filters, so liquidgo's native implementation intercepts the call. The fix must ensure plugin-registered filters override built-in filters in the template engine's dispatch chain.
- **Hook payload contract (issue #182)**: All hook payloads must be JSON-serializable for JS/WASM plugins. Seven categories:
  - **`onContentTransformed`** (issue #448): fires once per page with a typed `HookTransformPayload` struct: `HookTransformPayload{HTML: string(page.RenderedBody), TOC: convertTOCEntries(page.TOC), Path: page.RelPath, URL: page.URL, FrontMatter: convertOrderedMaps(page.FrontMatter)}` where `convertTOCEntries` copies `[]content.TOCEntry` → `[]plugin.TOCEntry` (shallow field copy, recursive for children). The returned object (inbound as `map[string]interface{}`) is applied back: `html` → `page.RenderedBody`, `toc` → `page.TOC`, `frontMatter` → `page.FrontMatter`. This allows plugins to build TOC for non-markdown pages, modify front matter post-render, or transform HTML. `fireContentTransformedHooks` must build a typed `HookTransformPayload`, not a `map[string]interface{}`. **Performance (issue #463)**: Before building the payload, recursively convert any `*ordered.Map` values in `page.FrontMatter` to plain `map[string]interface{}` via `ToGoMap()`. `ordered.Map.MarshalJSON` is expensive (13.7% of CPU in profiling). The conversion is read-only — `page.FrontMatter` is not mutated, only the serialization copy is converted. Templates still see the original `*ordered.Map` values after hooks complete. **Performance (issue #529)**: Use `sonic.ConfigStd` for JSON serialization instead of `encoding/json`. Typed payload structs allow sonic's JIT compiler to generate optimized encoders for the fixed fields (`Path`, `URL`, `HTML`, `TOC`) — only `FrontMatter` (user-defined keys) requires reflection. See §Typed outbound payload structs below.
  - **`onPageRendered`**: fires once per page with HTML string payload (post-processing only — page is already rendered, no page data mutation). The returned string replaces `page.RenderedBody`.
  - **Pre-taxonomy hook** (`onPagesReady`, issue #525): fires once per language batch inside `applyBatchContext()`, after cascade data application (line ~2238) but before `BuildTaxonomies()` (line ~2245). Payload: typed `HookPagesReadyPayload{Pages: serializePagesForPagesReady(pages), SiteData: siteData}` where each page is `HookPagePayload{Path: page.RelPath, URL: page.URL, FrontMatter: convertOrderedMaps(page.FrontMatter), Content: string(page.Body)}`. No `HTML` field — content has not been rendered yet. Return value: inbound as `map[string]interface{}` with same JSON shape. Pages at indices `>= len(originalPages)` are virtual pages — construct `*content.Page` from returned map: `path` → `RelPath`, `url` → `URL`, `frontMatter` → `FrontMatter` (including taxonomy terms like `tags`), `content` → `Body`. Validate: `path` and `url` required (error if missing). Check for output-path collisions with existing pages (error on collision — different URLs can map to the same output file, e.g. `/` and `/index.html`). Appended virtual pages are added to the `pages` slice **before** `BuildTaxonomies()` and `buildCollectionsContext()` — so they participate in taxonomy collection and appear in `taxonomies.*` in templates. Virtual pages with raw `content` flow through the content rendering pipeline (markdown → HTML). Per-batch firing means each invocation only sees pages from one language batch, avoiding the language routing problem in #521. `serializePagesForPagesReady` returns `[]HookPagePayload` (omits `HTML`, includes `Content`).
  - **Content hooks** (`onContentLoaded`, `onDataCascadeReady`): `onContentLoaded` fires once with the full pages array as `[]HookPagePayload` (typed structs with fields `Path`, `URL`, `FrontMatter`, `Content`, `HTML`). Return value is inbound as `[]interface{}` of `map[string]interface{}` — applied back: existing pages at indices `0..len(pages)-1` have their `frontMatter` merged back to `page.FrontMatter` and `html` written back to `page.RenderedBody`. Only `frontMatter` and `html` are applied — `content`, `path`, and `url` in the return are informational (present in the payload for plugin inspection, but mutations to those fields are not written back to the page). **Virtual page injection is not supported** — if the returned array length exceeds the input length, produce a validation error. Virtual page injection belongs exclusively in `onPagesReady` (#525). This also resolves #521 (virtual pages appended to wrong language batch) — since `onContentLoaded` no longer accepts virtual pages, the batch routing problem is eliminated. `onDataCascadeReady` fires once with the full pages array as `[]HookCascadePayload` — each entry is `HookCascadePayload{Path: page.RelPath, Data: page.FrontMatter}` (cascade is already merged into FrontMatter at this pipeline stage). Return value is inbound as `[]interface{}` of `map[string]interface{}` — cascade data applied back per page. No virtual page support. Apply-back pattern mirrors the `onContentLoaded` apply-back block in `build.go`: type-assert return as `[]interface{}`, validate length matches input (reject injection and removal with specific error messages referencing `onPagesReady`), build `pathToIdx` map from `pages`, iterate returned entries, extract `data` field via `toGoMap`, merge each key back into `pages[origIdx].FrontMatter`. The serialization must use `[]HookCascadePayload` instead of `serializePagesForHook` — build the slice directly where each entry has `Path: page.RelPath` and `Data: convertOrderedMaps(page.FrontMatter)`.
  - **Per-asset hook** (`onAssetProcess`): fire once per asset with `HookAssetPayload{Path: relPath, Content: fileContent}`. Return value's `"content"` key replaces the asset content.
  - **Per-build hooks** (`onConfig`, `onBeforeValidation`, `onAfterValidation`, `onDataFetched`): convert Go structs to `map[string]interface{}` before passing to `CallHook`. Deserialize returned map and apply changes back. Requires enhancing the QuickJS/WASM hook bridge so structured Go values are marshaled as real JSON objects (not stringified) — encode the map to JSON, parse in JS, then deserialize returned JS objects back into Go maps.
  - **Read-only hooks** (`onBuildComplete`, `onDevServerStart`, `onFileChanged`): serialize to JSON for observation, return value ignored. The runtime bridge must still preserve object payloads at the JS boundary for observation hooks.
- **Typed outbound payload structs (issue #529)**: The outbound serialization path (Go → Plugin) must use typed structs with json tags instead of `map[string]interface{}`. This separates the plugin bridge serialization path from the template rendering path (which uses `map[string]interface{}` for liquidgo). Typed structs allow `sonic.ConfigStd` to JIT-compile optimized encoders for fixed fields — only `FrontMatter` and `SiteData` (user-defined keys) require reflection. The inbound path (Plugin → Go) remains `map[string]interface{}` since plugin return shapes are dynamic. Structs are defined in `internal/plugin/payload.go`:
  - `HookPagePayload` — `Path string`, `URL string`, `FrontMatter map[string]interface{}`, `Content string` (omitempty), `HTML string` (omitempty). Used by `serializePagesForHook` (replaces `[]interface{}` return) and `serializePagesForPagesReady`. `FrontMatter` must NOT use `omitempty` — serialization path must coerce nil to `map[string]interface{}{}` before building the struct to avoid `null` in JSON (which causes `TypeError` on JS property access).
  - `HookTransformPayload` — `Path string`, `URL string`, `FrontMatter map[string]interface{}`, `HTML string`, `TOC []TOCEntry` (omitempty). Used by `fireContentTransformedHooks`.
  - `TOCEntry` — `ID string`, `Text string`, `Level int`, `Children []TOCEntry` (omitempty). Replaces `serializeTOC()`/`deserializeTOC()` manual map construction. This is a **separate type** from `content.TOCEntry` (which has no json tags and is used by the template/rendering path). The serialization path must convert `[]content.TOCEntry` → `[]plugin.TOCEntry` when building `HookTransformPayload` — a shallow field copy per entry.
  - `HookPagesReadyPayload` — `Pages []HookPagePayload`, `SiteData map[string]interface{}`. Used by `onPagesReady` serialization.
  - `HookCascadePayload` — `Path string`, `Data map[string]interface{}`. Used by `onDataCascadeReady`.
  - `HookAssetPayload` — `Path string`, `Content string`. Used by `onAssetProcess`.
  - **Bridge type switch (issue #529)**: The QuickJS `CallHook` at `wasm.go:323` uses a type switch with explicit cases for `map[string]interface{}` and `[]interface{}`. Typed payload structs hit the `default` case which calls `fmt.Sprint(v)` — producing a Go struct literal string, not JSON. The `default` case must be changed to `json.Marshal(v)` → parse in JS, matching the `map[string]interface{}` path. This applies to both the QuickJS bridge and the Node bridge's `EncodeMessage` at `node.go:56` (which already uses `json.Marshal` and handles structs correctly).
- **JSON library replacement (issue #529)**: Replace `encoding/json` with `sonic.ConfigStd` in `internal/plugin`. Add `var json = sonic.ConfigStd` package-level alias — all existing `json.Marshal`/`json.Unmarshal` calls work unchanged. sonic v1.15.1 (Apache 2.0) auto-falls back to `encoding/json` on non-amd64/arm64 architectures via build constraints. `sonic.ConfigStd` matches `encoding/json` behavior: HTML escaping enabled, map keys sorted, strings copied on decode. One known difference: backspace encoded as `\b` vs `` — both valid JSON, no behavioral impact. Add `github.com/bytedance/sonic` to `go.mod`.
- **WASM runtime (issue #181)**: `WASMRuntime.LoadModule()` uses wazero to compile and instantiate the WASM binary, discover exported functions (`alloc`, `filter`, `shortcode`, `hook`, `hooks`), and register them. `alloc` export is required — used by the host to get a safe write offset in WASM linear memory (issue #186). `CallExport(name, args...)` must call `alloc(inputLen)`, write input bytes at the returned pointer, call the export with `(ptr, len)`, and read the result from the returned `(resultPtr, resultLen)` (issue #190). `RegisteredFilters()` must return filter names from the module's exports. `LoadModule` must return an error for invalid WASM binaries. See PLAN.md §5 WASM Calling Convention for full ABI.
- **WASM hook support (issue #444)**: `WASMRuntime` must support hook registration and execution via the `hooks()`/`hook()` export ABI:
  - **Hook discovery**: `LoadModule` must check for a `hooks` export after module instantiation. If present, call it (no arguments — zero-arg function returning `(ptr i32, len i32)`, same pattern as `last_error`), read the returned bytes as a JSON array of hook name strings, and store as `r.hooks []string` on the `WASMRuntime` struct. If no `hooks` export, `r.hooks` remains nil. Add `hooks` to `wasmRuntimeExports` (note: `hook` is already reserved but `hooks` is not — add it).
  - **`RegisteredHooks()`**: Return `r.hooks` instead of nil.
  - **`CallHook(name string, payload interface{}) (interface{}, error)`**: New method. JSON-marshal the payload: if payload is `map[string]interface{}`, merge `{"event": name}` into the map; otherwise wrap as `{"event": name, "payload": payload}`. Call the `hook` export via the alloc/write/call/read ABI (same as `callStringFilter`). Parse the returned JSON. Return the result.
  - **Hook error handling**: Three error paths:
    1. **`hook()` returns `(0, 0)`**: Same convention as `filter`/`shortcode`. `CallHook` must check `last_error()` if available and return an error containing the module's error message. No silent fallback to original payload.
    2. **`hooks()` returns invalid JSON**: If `LoadModule` calls `hooks()` and the returned bytes are not a valid JSON array of strings (parse error, non-array type, or array containing non-string elements), `LoadModule` must return an error. Fail loud — do not silently treat the module as hook-less.
    3. **`hook()` returns malformed JSON response**: If `hook()` returns valid `(ptr, len)` (non-zero) but the bytes are not valid JSON, `CallHook` must return an error. Do not silently return nil or fall back to the original payload.
  - **Priority**: WASM hooks get default priority 50 — no per-hook priority in the export ABI. `registerRuntime` handles this via the fallback path (uses `RegisteredHooks()` with default priority 50 when `HookDetailer` is not implemented).
  - **Integration test coverage (issue #478)**: `internal/plugin/wasm_test.go` contains integration tests exercising the full JS→Go bridge path for hook priority: `alloy.hook({priority})` → `__registerHook` → `RegisteredHookDetails` → `registerRuntime` → `RegisterWithOptions` (QuickJS always passes scope JSON, so `registerRuntime` takes the `reg.Scope != nil` → `RegisterWithOptions` path, not `RegisterWithPriority`). Tests cover: explicit priority ordering (priority 10 before 100), default priority (omitted → 50), priority 0 as valid value, same-priority preserving registration order, negative priority, and non-integer priority flooring (`Math.floor` in JS bridge).
  - **`PluginFilterRuntime` interface update**: Add `CallHook(name string, payload interface{}) (interface{}, error)` to the `PluginFilterRuntime` interface (registry.go:46-58). Remove the `CallHook` type assertion in `registerRuntime()` (registry.go:266-268) — all three runtimes (QuickJS, WASM, Node) now implement it directly on the interface. This is the unified `Runtime` interface already spec'd at line 549-557.
  - **Test fixtures**: The `compiled.wasm` fixture (or a new `hook-plugin.wasm` in `testdata/single-files/`) must export `hooks()` returning `["onContentTransformed"]` and `hook(ptr, len)` that processes the event and modifies the payload (e.g., appends `<!-- wasm-hook -->` to HTML). The fixture's `hook()` export must return `(0, 0)` with `last_error()` set for unrecognized event names (to support the error path test). Two additional error-testing fixtures are required: `bad-hooks-export.wasm` (exports `hooks()` returning invalid JSON — e.g., raw bytes `not json`) and `wasm-malformed-hook-response.wasm` (exports valid `hooks()` but `hook()` returns non-JSON bytes). The developer will need to produce all fixtures with a WASM toolchain (Rust, TinyGo, or AssemblyScript).
- **Hook return type preservation (issue #571)**: JSON round-trip through plugin runtimes converts `*ordered.Map` to `map[string]interface{}`, breaking liquid template iteration and key order. Fix at the deserialization boundary in each runtime:
  1. **`ordered.RewrapValue(v interface{}) interface{}`**: New function in `internal/ordered/map.go`. Recursively converts `map[string]interface{}` → `*ordered.Map`, recurses into `[]interface{}` arrays. Passes through `*ordered.Map`, primitives, and nil unchanged. Inverse of `convertOrderedValue` in build.go.
  2. **QuickJS `CallHook`** (`wasm.go:388`): Replace `json.Unmarshal([]byte(s), &obj)` with `ordered.UnmarshalJSONValue([]byte(s))`. This preserves insertion order from the JSON response string.
  3. **Node `CallHook`** (`node.go:318`): After `resp.Result` is obtained from `DecodeMessage`, the Result field is already `map[string]interface{}` (standard `json.Unmarshal` of the Message struct). Two options: (a) change `DecodeMessage` to use `json.RawMessage` for the Result field and parse separately with `ordered.UnmarshalJSONValue`, or (b) post-process with `ordered.RewrapValue(resp.Result)`. Option (a) preserves insertion order; option (b) is simpler but loses original key order.
  4. **`build.go:293` consumer update**: The `onDataFetched` result handler does `if modified, ok := dataResult.(map[string]interface{})`. After the runtime fix, the result may be `*ordered.Map`. Add a case: if `*ordered.Map`, iterate its entries and merge into siteData.
  5. **Scope**: This affects any hook whose return value is merged back into pipeline state. Currently: `onDataFetched` (build.go:289-296). `onContentLoaded` (build.go:516-530) also deserializes returned page maps — same fix applies at the runtime level. `onDataCascadeReady` discards its return value, so no fix needed there.
- **WASM compilation cache (issue #391)**: `WASMRuntime.LoadModule()` creates `wazero.NewRuntime(ctx)` with no compilation cache. Add `wazero.NewCompilationCacheWithDir(".alloy/wasm-cache")` to the runtime config so compiled native code persists to disk across builds. For QuickJS (`qjs.New()`), the upstream `fastschema/qjs` package creates wazero internally — either fork/contribute to accept a `wazero.RuntimeConfig`, or bypass `qjs.New()` and initialize wazero directly with cache config.
- **Parallel plugin runtime initialization (issue #401)**: `LoadPlugins()` currently initializes all runtimes sequentially: `qjs.New()` / `CompileModule()` / Node subprocess spawn, then `EvalFile()`, then registration. Runtime initialization (Phase A) is pure CPU work with no pipeline dependencies — it can run concurrently across plugins. Split `LoadPlugins` into two phases:
  - **Phase A — `InitRuntimes()`**: For each plugin, create the runtime (`NewQuickJSRuntime` + `Init()`, `NewWASMRuntime` + `LoadModule()`, `NewNodeRuntime`). Returns initialized runtimes. Each runtime init is independent — run concurrently with `sync.WaitGroup`. Errors are collected, not fatal (same as current warning behavior).
  - **Phase B — `EvalAndRegister()`**: For each initialized runtime, call `EvalFile()`, register filters/hooks with the registry and HookRegistry. This is sequential — registration order matters for conflict resolution ("last loaded wins").
  
  In `Build()`, Phase A can overlap with early pipeline work (cascade loading, data file reading) via a goroutine. Phase B must complete before `onConfig` fires (line 138). The dependency chain is: Phase A done → Phase B done → `onConfig` → `InitPipelineState`.
  
  **Note**: The current `Build()` fires `onConfig` immediately after `DiscoverPlugins()` (line 138). Parallelization requires restructuring: start Phase A in a goroutine, do cascade/data preloading concurrently, join Phase A, run Phase B, then fire hooks.
- **Unified plugin bridge (issues #189, #237)**: All plugin tiers (Tier 2 QuickJS, Tier 2 WASM, Tier 3 Node) must implement the same `Runtime` interface. `Registry.Runtimes()` returns `[]Runtime` and the bridging loop in `Registry.LoadPlugins()` works identically for all tiers — no tier-specific code.

  ```go
  type Runtime interface {
      RegisteredFilters() []string
      CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error)
      RegisteredShortcodes() []string
      CallShortcode(name string, args []string, content string) (string, error)
      RegisteredHooks() []string
      CallHook(name string, payload interface{}) (interface{}, error)
  }
  ```

  Three implementations:
  - **`QuickJSRuntime`** — in-process JS execution via embedded QuickJS. Already implements most methods.
  - **`WASMRuntime`** — in-process WASM execution via wazero with alloc/ptr/len ABI.
  - **`NodeRuntime`** — subprocess execution via JSON-RPC over stdin/stdout (length-prefixed, LSP-style framing). Stderr redirected to `.alloy/plugin.log`.
  - **Worker pool (issue #491)**: For per-page hooks (`onPageRendered`, `onContentTransformed`), spawn multiple subprocess workers and distribute pages across them. Each worker loads the same plugins via `EvalFile`. Implementation:
    - `WorkerPool` struct: holds `[]*NodeBridge`, worker count, and dispatch logic
    - `NewWorkerPool(count int, projectRoot string) *WorkerPool`: spawns N bridges, each loading the same plugin files
    - `DispatchHook(event string, pages []PagePayload) ([]PageResult, error)`: distributes pages in contiguous chunks across workers, collects results in order
    - Auto-scaling: `cfg.Plugins.Workers` — `"auto"` (default) uses `min(runtime.NumCPU()/2, 8)` with floor 2. Integer value overrides.
    - `config.PluginsConfig` gets a `Workers` field: `Workers interface{} \`yaml:"workers"\`` — accepts `"auto"` or int
    - Workers spawn async during `DiscoverPlugins` (overlap with pipeline init), shut down in `registry.Close()`
    - Only Tier 3 (subprocess) runtimes — Tier 2 (QuickJS/WASM) is in-process, no pooling needed
    - Per-page dispatch (not batched) — prototype showed individual IPC outperforms batched JSON arrays for large HTML payloads
    - `EvalFile(path)`: Send the **absolute file path** (not source code) to the Node subprocess. The bridge loads the plugin via `await import(absPath)`, calls `mod.default(alloy)`, and returns discovered registration names. **ESM import replaces eval() (issue #441)**: the previous approach read the file, regex-stripped `export` keywords, and `eval()`d the result in global scope. This caused side-effect collisions (`customElements.define` duplication) because `eval()` has no module semantics — re-evaluation re-triggers side effects that ES modules would deduplicate. With `import()`, Node's module cache ensures each module is loaded once. Go side sends `Payload: absPath` instead of `Payload: string(src)`. Bridge side: `case 'eval'` uses `await import(msg.payload)` instead of `eval(code)`. **Prerequisite**: projects must use ESM (`"type": "module"` in `package.json`). This is a requirement for Tier 3 Node plugins.
    - `CallFilter(name, input, args...)`: Send `Message{ID: n, Type: "filter", Name: name, Payload: input}` to subprocess via `EncodeMessage`. Wait for response `Message{ID: n, Result: "..."}` via `DecodeMessage`. Return result.
    - `CallHook(name, payload)`: Send `Message{ID: n, Type: "hook", Name: name, Payload: payload}`. Wait for response. Return modified payload from `Result` field.
    - `CallShortcode(name, args, content)`: Send `Message{ID: n, Type: "shortcode", Name: name, Payload: {args, content}}`. Wait for response. Return rendered HTML from `Result` field.
    - All messages use the existing `Message` type and `EncodeMessage`/`DecodeMessage` with LSP-style length-prefixed framing. The `ID` field correlates requests with responses.
    - **Bridge script**: `NodeBridge.Start()` spawns `node` with a built-in bridge script (embedded in the Go binary, written to `.alloy/bridge.js` in the project root) that implements the `alloy` API object and the JSON-RPC message loop. The bridge script is NOT a user file — it's Alloy's Node-side runtime. Writing to the project root (not a temp dir) is required for ESM `import()` resolution — see module resolution below.
    - **Module resolution (issue #248)**: The Node subprocess must resolve `import()` and `require()` from the project's `node_modules/`. Setting `cmd.Dir = projectRoot` alone is insufficient — ESM `import()` resolves relative to the importing module's URL, not the working directory. The bridge script must be written to the project directory (e.g., `.alloy/bridge.js`) instead of a temp dir, so both CJS `require()` and ESM `import()` resolve from the project root. `NewNodeRuntime` must accept the project root and pass it to `NewNodeBridge`. `LoadPlugins` derives the project root from `registry.pluginsDir` (parent directory).

  `LoadPlugins()` does the same bridging for all runtimes:
  ```
  for _, rt := range registry.Runtimes() {
      // Bridge filters
      for _, name := range rt.RegisteredFilters() {
          engine.AddFilter(name, wrap(rt.CallFilter))
      }
      // Bridge shortcodes
      for _, name := range rt.RegisteredShortcodes() {
          engine.AddTag(name, wrap(rt.CallShortcode))
      }
      // Bridge hooks
      for _, name := range rt.RegisteredHooks() {
          hooks.Register(HookName(name), wrap(rt.CallHook))
      }
  }
  ```

  The pipeline never knows or cares which tier a plugin is. The `Runtime` interface is the only integration point.

  **LoadPlugins Node wiring (issue #244)**: Node plugins must be evaluated and bridged the same way as QuickJS plugins. For each Node plugin: (1) evaluate the plugin JS to discover registrations, (2) register discovered filters, (3) bridge discovered hooks to HookRegistry. On evaluation failure, produce a warning and continue loading other plugins — do not abort.

  **NodeRuntime specifics:**
  - `Init()` spawns the Node subprocess, sends all Tier 3 plugin source files for evaluation via JSON-RPC `eval` messages
  - The subprocess reports back which filters/shortcodes/hooks were registered via JSON-RPC `registered` response
  - `CallFilter`/`CallShortcode`/`CallHook` send a JSON-RPC request with the payload, wait for response, return the result
  - Payload serialization: Go `interface{}` → JSON → Node subprocess → JS function → JSON → Go `interface{}`. Same JSON-serializable contract as the hook payload spec (per-page HTML strings, `{path, content}` objects, etc.)
  - Timeout: each call respects `cfg.Plugins.Timeout` (default 5s). Node subprocess crash → error, not silent failure
  - Process lifecycle: spawned once at startup, kept alive for the build duration (or serve session). Killed on shutdown.
- **Incremental build via cache (issue #105, #225)**: This applies to `BuildIncremental` only (dev mode). `Build()` and `BuildWithContent()` always do full rebuilds — they do not read the cache. **Cache ownership is caller-side**: the dev-mode watcher loop in `cmd/dev.go` loads the previous build cache from disk via `cache.LoadFrom(cacheDir)` and passes it into `BuildIncremental` as `previousCache`. `BuildIncremental` consumes `previousCache` for skip/invalidation decisions only — it does not mutate the cache or return an updated cache object. Cache persistence happens via `Build()` Stage 9 (full rebuilds write `.alloy/cache.json`); incremental rebuilds rely on the cache written by the most recent full build. After discovering pages, use `previousCache.ShouldSkipFile(relPath, content)` to skip unchanged pages — no re-parse, no re-render. Template changes override content-hash skipping: if a layout file changed, `previousCache.InvalidatedPages(layoutPath)` returns the affected pages, which must be rebuilt even if their content hash is unchanged. Config changes (`previousCache.IsConfigChanged(currentHash)`) trigger a full rebuild. The `BuildResult.PagesSkipped` field reports the skip count (e.g., "Rebuilt 5 pages, 27 skipped (cached)").

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
// ── Build language batches (issue #280) ──
// Single-language sites produce one batch. Multi-language produces N batches.
// No if/else fork — the pipeline always iterates batches.
var langContexts []i18n.LanguageContext
if cfg.Languages != nil {
    langContexts = i18n.BuildLanguageContexts(cfg.Languages)
} else {
    // Single-language default: one batch with cfg.Language (default "en")
    langContexts = []i18n.LanguageContext{{Code: cfg.Language, Root: true}}
}

allLangPages := []*content.Page{}

type langBatch struct {
    ctx         LanguageContext
    pages       []*content.Page
    collections map[string]interface{}
    siteData    map[string]interface{}
}
var batches []langBatch

// ── Pass 1: discover + content-render per batch (steps 3-11) ──
for _, langCtx := range langContexts {
    // For multi-language: content/<lang>/. For single: content/.
    contentDir := resolveContentDir(langCtx, cfg)
    pages := content.DiscoverWithFormats(contentDir, formats)

    // Set lang on each page's front matter
    for _, page := range pages {
        page.FrontMatter["lang"] = langCtx.Code
    }

    // Apply output prefix (empty for root language)
    prefix := i18n.OutputPrefix(langCtx.Code, langCtx.Root)
    //
    // IMPORTANT (issue #113): permalink resolution must use the
    // ORIGINAL relPath (without language prefix), not the prefixed
    // one. Resolve permalink first, then prefix RelPath.

    langSiteData := copyMap(siteData)
    langSiteData["language"] = i18n.LanguageData(langCtx)

    // Steps 4-11 per batch

    allLangPages = append(allLangPages, pages...)
    batches = append(batches, langBatch{...})
}

// ── Link translations (no-op for single batch) ──
langCodes := make([]string, len(langContexts))
for i, ctx := range langContexts { langCodes[i] = ctx.Code }
i18n.LinkTranslations(allLangPages, langCodes)

// ── Pass 2: layout resolution + output (steps 12-15) per batch ──
for _, batch := range batches {
    renderPageThroughLayouts(page, chain, engine, batch.siteData, ...)
    generateTaxonomyPages(batch.taxonomies, engine, cfg, ...)
}

// Steps 16-20 (static copy, assets, sitemap, cache) run once
```

Key points:
- **Unified two-pass pipeline (issue #280)**: Always operates on language batches. Single-language sites produce one batch. Pass 1 (steps 3-11) discovers and content-renders each batch. `LinkTranslations` runs between passes (no-op for single batch). Pass 2 (steps 12-15) resolves layouts and writes output. No `if/else` fork.
- `layouts/` is shared across all languages — never scoped
- `data/` globals are shared, but `site.language` and `site.title` are overridden per-language iteration via a shallow copy
- Collections and taxonomies are per-language: `collections.blog` for English only contains English posts
- Languages can build in parallel (independent content trees) but initial implementation should be sequential

### 5D: `cmd/` + `main.go` — 15 tests
**Files**: `main.go`, `root.go`, `build.go`, `serve.go`, `init.go`, `version.go`

- **`main.go` exit code handling (issue #28)**: `main()` must check the error return from `cmd.Execute()` and call `os.Exit(1)` on failure. Without this, all CLI errors exit 0, breaking scripts and CI. Current code discards the error.
- Register Cobra flags (--config, --output, --root, --verbose, --quiet) on root; (--port, --no-drafts, --refetch) on dev; (--port, --refetch) on serve ✅ done (except --root, dev/serve split)
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
3. If `--profile`, start profiling: `pipeline.StartProfiling(resolveDir(cfg.ProjectRoot, profileDir))`. The `--profile-dir` flag defaults to `.alloy/profiles` but must be resolved relative to `cfg.ProjectRoot` (not CWD) so `-r` works correctly.
4. Call `pipeline.Build(cfg, pipeline.BuildOptions{Profile: profile})`. When `Profile` is true, `Build` instruments 15 pipeline stages via `StageTimer` and populates `BuildResult.StageTimings`.
5. If `--profile`, call `profiler.StopProfiling()` (writes `cpu.prof` + `mem.prof`), then `pipeline.PrintStageTimings(stdout, result.StageTimings)`.
6. Print summary: `fmt.Printf("Built %d pages in %s\n", result.PageCount, result.Duration)`.
7. Return any error from Build — Cobra will handle exit code.

**Profiling types** (`internal/pipeline/profiler.go`):
- `StageTimer` — records named stage durations. `Start(name)` auto-stops the previous stage. `Timings()` returns `[]StageTiming`.
- `Profiler` — wraps pprof. `StartProfiling(dir)` creates the directory and begins CPU profiling. `StopProfiling()` writes `cpu.prof` + `mem.prof`. `Dir()` returns the output directory.
- `PrintStageTimings(w, timings)` — formatted table with Stage/Duration/%Total columns.
- `BuildOptions.Profile bool` — when true, `Build` creates a `StageTimer` and populates `BuildResult.StageTimings`.
- `BuildResult.StageTimings []StageTiming` — empty when `Profile` is false (no overhead).

**Test note**: The existing cmd test `"build command executes the build pipeline successfully"` runs without a project fixture. Build must handle missing content directory gracefully (return zero-page success, not error) for this test to pass.

#### `cmd/dev.go` (issue #256, was #29; watcher fix #371)

Dev server command (`alloy dev`). Uses `ModeDev` — Phase 1 only, in-memory, drafts visible.

1. Load config (same as build).
2. Set `cfg.IncludeDrafts = true` (unless `--no-drafts`).
3. Run initial build via `pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})`. `Build()` persists cache to disk at Stage 9 (`.alloy/cache.json`).
4. Read `--port`, `--no-drafts`, `--refetch` flags.
5. Call `server.NewWithMode(cfg, server.ModeDev)` and `server.Start()`.
6. **Start file watcher (issue #371)** — call `server.WatchDirs(cfg)`, `addRecursiveWatch` on each directory, fsnotify event loop with `ClassifyChange` and debouncer. On file change, dispatch by `ChangeType`:
   - `ContentChange`/`LayoutChange`/`DataChange` → load previous cache via `cache.LoadFrom(cacheDir)`, extract `changedFiles` from debounced events, call `pipeline.BuildIncremental(cfg, nil, previousCache, changedFiles, pipeline.BuildOptions{SkipSSR: true})`. When `contentMap` is nil, `BuildIncremental` discovers content from the filesystem. The cache is persisted to disk by `Build()` at Stage 9 and loaded by the next rebuild via `cache.LoadFrom()`. Bulk changes (10+ files) trigger a full `Build()` instead.
   - `AssetChange`/`StaticChange` → no recopy needed in dev mode (served from source). Browser reload only.
   - `PassthroughChange` → no recopy needed in dev mode (served from source). Browser reload only.
   - `ComponentChange` → full rebuild via `pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})`.
   - All types → `srv.BroadcastReload()` after rebuild.
   - **Bulk change protection**: If debouncer detects 10+ simultaneous changes (e.g., `git checkout`), always do a full rebuild. Cache is re-persisted by `Build()` Stage 9.
7. Block until interrupt.

#### `cmd/serve.go` (issue #256; watcher #291, fix #371)

Production server command (`alloy serve`). Uses `ModePreview` — same pipeline as `alloy build`, writes to `_site/`, SSR if configured, excludes drafts. **Must have a file watcher** — `alloy serve` is NOT a one-shot build (PLAN.md §8).

1. Load config (same as build).
2. Set `cfg.IncludeDrafts = false`.
3. Run initial build via `pipeline.Build(cfg)`.
4. Read `--port`, `--refetch` flags. No `--no-drafts` (production always excludes drafts). No `--preview` (removed).
5. Call `server.NewWithMode(cfg, server.ModePreview)` and `server.Start()`.
6. **Start file watcher (issue #291, #371)** — same setup as `cmd/dev.go`: call `server.WatchDirs(cfg)`, `addRecursiveWatch` on each directory, fsnotify event loop with `ClassifyChange` and debouncer. On file change, dispatch by `ChangeType`:
   - `ContentChange`/`LayoutChange`/`DataChange` → `pipeline.Build(cfg)` (full rebuild — serve mode always does full rebuilds, not incremental)
   - `AssetChange`/`StaticChange` → recopy changed files to `_site/`
   - `PassthroughChange` → `server.RecopyPassthroughFile(changedPath, cfg)` — copies only the changed file to `_site/<to>/<relative-path>`
   - `ComponentChange` → full rebuild (SSR re-render)
   - All types → `srv.BroadcastReload()` after rebuild/recopy
7. Block until interrupt.

**`server.RecopyPassthroughFile(changedPath string, cfg *config.Config) (string, error)`** — Finds the matching passthrough mapping by checking which `from:` directory the `changedPath` is under. Computes the relative path within `from:`, constructs the output path as `_site/<to>/<relative-path>`, copies the single file, and returns the output path. Returns error if no matching mapping is found or the copy fails.

**Test note**: The cmd tests verify both `dev` and `serve` commands are registered via `root.Find()` without calling `Execute()`. Server startup behavior is tested directly in `internal/server/server_test.go`.

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

#### MIME type serving (issue #252)

`serveFileWithReload` must set `Content-Type` based on file extension before writing the response. Add a `MIMEType(ext string) string` function to the server package with a built-in map of common web file extensions. This avoids relying on the platform's MIME database which may be incomplete (macOS missing `.css` → `text/plain`). For unknown extensions, fall back to `mime.TypeByExtension(ext)`, then `application/octet-stream`. In `serveFileWithReload`, call `w.Header().Set("Content-Type", MIMEType(filepath.Ext(filePath)))` before writing.

#### Build progress output

The pipeline needs a `ProgressReporter` interface that `Build()` and `BuildIncremental()` accept (or nil for no progress):

```go
type ProgressReporter interface {
    StartStage(name string, total int)                          // "Rendering", 420. total=-1 for unknown (discovery).
    Message(text string)                                        // "42 pages found" — inline with current stage
    Update(current int, filePath string, elapsed time.Duration) // 1-based: 142, "content/blog/my-post.md", 12ms
    EndStage()
    Summary(pageCount int, duration time.Duration, pagesSkipped int)
}
```

`total=-1` in `StartStage` means the total is unknown (e.g., discovery stage). The implementation shows a message-style output instead of a progress bar. `total=0` is a valid known total (empty stage — zero-page builds are supported). `Message` renders inline with the current stage context (e.g., `[alloy] Discovering content... 42 pages found` is produced by `StartStage("Discovering", -1)` followed by `Message("42 pages found")`). `Update` uses 1-based `current` (first item = 1, not 0).

Two implementations:
- `TTYProgress` — progress bar with carriage return, adapts to terminal width. TTY detection: `term.IsTerminal(int(os.Stdout.Fd()))` (from `golang.org/x/term`).
- `VerboseProgress` — per-file line output with timing. Replaces the progress bar (not combined — mixing per-file lines with carriage-return progress produces messy output).

`cmd/build.go` creates the reporter based on flags:
- `--quiet` → nil (no progress)
- `--verbose` → `VerboseProgress`
- default → `TTYProgress` if terminal, nil if piped

Both `cmd/dev.go` and `cmd/serve.go` must attach a progress reporter before the initial `pipeline.Build(cfg)` call using the same flag-based logic. This is where progress matters most — the user is watching the terminal waiting for the server to start. Without a reporter, there is no output between running the command and seeing `Serving at http://localhost:3000`. The reporter must be cleaned up after the initial build completes (`defer pipeline.SetReporter(nil)` scoped to the initial build, not the entire serve lifetime).

For file-watcher rebuilds, the command should also attach a reporter before calling `pipeline.Build(cfg)` or `pipeline.BuildIncremental(...)`. The reporter is set and cleared around each rebuild call.

`BuildIncremental()` only calls `Summary` on the reporter — no `StartStage`, `Update`, or `EndStage`. Incremental rebuilds are typically 1-3 pages in under 100ms; a multi-stage progress bar would be visual noise. The `Summary` call uses `pagesSkipped` to show cached page count (e.g., "Rebuilt 3 pages in 47ms (417 cached)").

Full rebuilds in serve mode (config changes, 10+ files) go through `Build()`, which uses the full multi-stage reporter sequence.

The pipeline must nil-guard every progress call since the reporter may be nil.

**All build stages must report progress (issue #493)**. Currently only content rendering (Pass 1b) has a progress bar. The 8-12s gap between "Rendering 100%" and build completion has no feedback. Add `StartStage`/`Update`/`EndStage` to:

| Stage | Name | Total | Where |
|-------|------|-------|-------|
| Content rendering | `"Rendering"` | `len(pages)` | Already done (Pass 1b) |
| Layout rendering | `"Layouts"` | `len(pages)` | Pass 2, per-page in layout chain loop |
| Post-render hooks | `"Transforms"` | `len(pages)` | `firePageRenderedHooks`, per-page |
| Output writing | `"Writing"` | `len(pages)` | Output write loop, per-file |

Post-render hook progress reports at the page level, not the batch level — even with worker pools (#491), the caller tracks which pages are complete.

`Build()` reporter calls:
```go
if reporter != nil { reporter.StartStage("Rendering", len(pages)) }
// per page (1-based current):
if reporter != nil { reporter.Update(i+1, page.RelPath, elapsed) }
if reporter != nil { reporter.EndStage() }
if reporter != nil { reporter.Summary(result.PageCount, result.Duration, result.PagesSkipped) }
```

`BuildIncremental()` reporter calls:
```go
// No StartStage/Update/EndStage — compact summary only
if reporter != nil { reporter.Summary(result.PageCount, result.Duration, result.PagesSkipped) }
```

**Summary ownership**: The build summary line is emitted by `reporter.Summary()`, not by `cmd/build.go` directly. When `reporter` is nil (`--quiet` or piped non-TTY), no summary is printed. Remove the `fmt.Printf("Built %d pages...")` from `cmd/build.go` — the reporter is the single source of truth for build output.
```

**Verify**: `go test ./internal/plugin/... ./internal/fetch/... ./internal/i18n/... ./cmd/...`

---

## Phase 6: Server + SSR (~65 tests)

### 6A: `internal/server` — 51 tests
**Files**: `server.go`, `watcher.go`, `overlay.go`

- HTTP server with mode-aware behavior (dev/preview)
- File watcher with debouncing and change classification
- **Passthrough watching (issue #275)**: `WatchDirs(cfg)` must iterate `cfg.Passthrough` and append each `from:` path. `ClassifyChange(path, cfg)` must add a `PassthroughChange` type for files matching passthrough source directories. The serve rebuild handler should recopy only the changed file to `_site/<to>/<relative-path>` on `PassthroughChange` instead of triggering a full pipeline rebuild. In `alloy dev`, passthrough changes only trigger a browser reload (files are served from source). `addRecursiveWatch` must be called on each passthrough `from:` directory.
- **Watch directory config (issue #530)**: `WatchMapping` struct in `internal/config/config.go` — `From string`, `Type string` with yaml/toml/json tags. `Watch []WatchMapping` field on `Config` after `Passthrough`. `Validate()`: reject empty `from`, reject `type` not in {content, layout, data}, include array index in error. Reject nonexistent `from` directories (`os.Stat` or equivalent — fail fast). Reject duplicate `from` paths (build a seen-set, error on collision). Reject `from` matching base structure dirs (content, layouts, data, assets, static). Normalize trailing slashes in `from` (strip before storing/comparing). `WatchDirs()`: loop after passthrough — append `w.From` (or `static.GlobRoot(w.From)` for globs). `ClassifyChange()`: in `default:` branch, check watch dirs before passthrough loop — switch on `w.Type` to return `ContentChange`/`LayoutChange`/`DataChange`. No new `ChangeType` constant needed — reuses existing types. No changes to `cmd/serve.go`, `cmd/watcher.go`, or `RebuildScopeForChangeType` — existing dispatch handles all three types correctly.
- **Content-colocated file serving (issue #300)**: In dev mode, the server's request handler must fall back to the content directory for URLs that don't match a rendered page in memory. `ServeContentFile(urlPath string) ([]byte, error)` reads the file from `content/<urlPath>` and returns its bytes. Returns error if the file doesn't exist. This is only used in dev mode — serve mode has the files in `_site/` from the build.
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
