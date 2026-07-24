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

### 1B: `internal/data` — 17 tests
**File**: `internal/data/loader.go`

- `LoadFile`: Detect format by extension (.yaml/.yml, .toml, .json), parse with appropriate library. **JSON files return `*ordered.Map`** (issue #453) — the value inside `siteData["filename"]` is an `*ordered.Map` preserving key insertion order. The `LoadFile` return type stays `map[string]interface{}` for YAML/TOML; for JSON, the top-level object is an `*ordered.Map` stored as `interface{}` in the result.
- `LoadDirectory`: Recursively walk dir and subdirectories, `LoadFile` each, key by filename without extension. **Subdirectories create nested namespace maps (issue #983)**: when an entry is a directory, recurse into it; the directory name becomes a key whose value is a `map[string]interface{}` containing the subdirectory's entries. Example: `data/nav/main.yaml` → `result["nav"]["main"]`. Nesting depth is unlimited. Empty subdirectories (no data files at any depth) must not produce a key. **Stem collision detection**: Track seen stem names per directory level. If two files share a stem (e.g., `team.csv` and `team.yaml`), return an error listing both files. **Directory-file stem collision (issue #983)**: if a file and non-empty subdirectory share the same stem (e.g., `nav.yaml` and `nav/` containing data files), return an error — both claim the same key. **Exception (issue #1019)**: a file coexisting with an empty directory of the same stem is not a collision — the empty directory is skipped before collision detection runs. Collision detection applies recursively within subdirectories. No silent overwrites — consistent with output path conflict philosophy (§2).
- `LoadCSV`: `encoding/csv`, first row = headers, subsequent rows = `[]map[string]string`
- **External data files (issue #271)**: `loadSiteData` in `build.go` must also load files from `cfg.Data.Files` (a `map[string]string` of key → path). For each entry, resolve the path relative to `cfg.ProjectRoot`, call `data.LoadFile`, and add the result to `siteData[key]`. Check for collisions with `data/` directory keys. File not found is a build error. Add `DataConfig` struct to `config.go` with `Files map[string]string` field.
- **Data directory errors are fatal (issue #982)**: `loadSiteData` in `context.go` propagates errors from `data.LoadDirectory` as build failures. When `data.LoadDirectory` returns an error that is not `fs.ErrNotExist`, `loadSiteData` returns `(nil, fmt.Errorf("data directory %s: %w", dataDir, err))`. `InitPipelineState` propagates this to `Build()`, aborting the build. `fs.ErrNotExist` (data directory does not exist) is silently ignored — not every project uses data files. The incremental path in `BuildIncremental` handles `loadSiteData` errors as warnings (preserving stale data in dev mode) — that behavior is correct and must not change.

### 1C: `internal/cascade` — 37 tests
**Files**: `internal/cascade/merge.go`, `internal/cascade/context.go`

- `DeepMerge`: Recursive map merge, arrays replaced (not concatenated)
- `LoadDirectoryCascade`: Walk content dir for `_data.yaml` files, merge parent->child. Only creates entries for directories that contain `_data.yaml`.
- `FindCascadeData(cascadeData, contentBase, relPath)`: Nearest-match lookup (issue #219). Given a page's `relPath`, walks up the directory tree and returns the nearest ancestor entry from `LoadDirectoryCascade`. Returns `nil` when no ancestor has data. Since `LoadDirectoryCascade` already accumulates ancestor data into each entry via `DeepMerge(parentData, data)`, the nearest match contains the full chain — no re-merge needed. The pipeline must use this instead of exact key lookup — otherwise pages in directories without `_data.yaml` miss ancestor inheritance (spec §3 requires cascade to flow to all descendants).
- `BuildContext`: Allocate `PageContext` with shared pointers (3-level: Global, Directory, FrontMatter).
- `Get`: Lookup order: FrontMatter > Directory > Global.

### 1D: `internal/validation` — 16 tests
**File**: `internal/validation/conflicts.go`

- `DetectConflicts`: Group `OutputPathEntry` by path, return conflicts where count > 1
- `ValidatePermalinkAliases`: Error if page has no output URL (`p.URL == ""`) AND aliases. Must check the computed `URL` field (populated by `ResolveFromCascade`), not the front-matter `Permalink` field — pages without an explicit `permalink:` in front matter still have valid URLs from `_data.yaml` cascade patterns (issue #110). Permalink patterns come exclusively from `_data.yaml` cascade, not `Config.Permalinks` (issue #832).

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
- **Array**: Sort (numeric-aware, issue #348), Reverse, First, Last, Limit (issue #826), Where, GroupBy, Size, Map, Uniq, Compact, Concat
- **`Sort` numeric awareness (issue #348)**: Update `Sort` comparison to use `isWholeNumber(v)` helper: type-switch on `int` (direct, negatives included), `float64` (no fractional part → int, negatives included), `string` (all digits → `strconv.Atoi`, negative strings NOT parsed). Both values whole numbers → compare as `int`. Either not → fall back to `toString` comparison. Nil/missing values sort to end. No new filter — `sort` itself becomes numeric-aware.
- **Set**: Intersect, Union, Complement
- **URL**: URLEncode, URLDecode, AbsoluteURL, URLFilter — `RegisterURLFilters(baseURL string)` binds the site base URL at engine setup (same pattern as `RegisterAssetFilters`). `AbsoluteURL` uses bound baseURL as fallback when no explicit arg is passed. `URLFilter` prepends the path portion of baseURL (e.g., `/blog` from `https://example.com/blog/`).
- **Math**: Plus, Minus, Times, DividedBy (guard div-by-zero), Modulo, Ceil, Floor, Round, Abs
- **Content**: Markdownify (issues #366, #353) — must use the same config-driven goldmark extensions and parser options as the main content renderer. `createEngine(cfg)` initializes the shared goldmark instance from `cfg.Content.Markdown` (same `Unsafe`, `Typographer`, `AutoHeadingID` settings). Uses its own separate `goldmark.Markdown` instance (not the pipeline's `RenderContext.Goldmark`) because markdownify forces `TemplateTags: false` and has no hooks — it processes already-rendered values. Both instances read base config from `cfg.Content.Markdown`. If the site sets `autoHeadingID: false`, `markdownify` also skips heading IDs.
- **Regex**: FindRE, ReplaceRE
- **Data**: JSONFilter, Default
- **Output safety**: SafeHTML
- **Assets**: CacheBust, GetHash
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
- `LoadWithDefaults`: Call `Load`, then apply defaults: `Build.Output="_site"`, `Templates.Engine="liquid"`, `Content.Markdown.Goldmark.TemplateTags=true`, `Pagination.Path="page"`, `Plugins.Timeout=5000`, `Content.Formats=["md","html"]`, `Language="en"`, `Build.Clean=true`, `Structure.Content="content"`, `Structure.Layouts="layouts"`, `Structure.Assets="assets"`, `Structure.Static="static"`, `Structure.Data="data"`, `Structure.Plugins="plugins"`, `Structure.Components="components"`. `Content.Markdown.TOC` uses `*bool` tri-state — nil (omitted) defaults to true via `TOCValue()`, explicit `false` is preserved. This is separate from goldmark options because TOC extraction is an Alloy-level concern (AST walk after goldmark parsing), not a goldmark parser option (issue #828).
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
- `RenderMarkdown` (issues #353, #700): Single consolidated render function accepting a pre-built goldmark instance. Signature: `RenderMarkdown(source []byte, md goldmark.Markdown) ([]byte, []TOCEntry, error)`. The caller creates the instance via `CreateGoldmark(opts)` once and reuses it. Always returns TOC (AST heading walk is negligible cost). `renderPages` uses `RenderMarkdown(body, rc.Goldmark)` for the pipeline hot path — no per-page goldmark allocation. Configure goldmark with extensions (tables, task lists, typographer, footnotes). Handle `Unsafe` (raw HTML passthrough). Handle `TemplateTags` via a custom goldmark extension (issue #564) — see **Template tag goldmark extension** below. Handle `TemplateBlocks` (issue #202) via the same extension's block parser. NO placeholder substitution — the `protectTemplateTags`/`restoreTemplateTags` functions, `blockShortcodeLineRe` regex, and all placeholder logic must be removed and replaced by proper goldmark parser extensions. The old `RenderMarkdown(source, opts)` and `RenderMarkdownWithTOC(source, opts)` convenience wrappers (issue #693) are deleted — all callers pass a pre-built goldmark instance via `CreateGoldmark(opts)`. Tests use shared instances (e.g., `defaultMD := CreateGoldmark(defaultOpts)`) to minimize boilerplate.

  **Template tag goldmark extension** (issue #564):

  **Custom AST node types** (in `markdown.go`):
  - `TemplateTagInline` — embeds `ast.BaseInline`, stores `TagText []byte`. `Kind()` returns `KindTemplateTagInline`. `IsRaw()` returns `true`.
  - `TemplateTagBlock` — embeds `ast.BaseBlock`, stores `TagText []byte`. `Kind()` returns `KindTemplateTagBlock`. `IsRaw()` returns `true`.

  Both use custom AST node kinds (NOT `ast.RawHTML`) so template tags are preserved regardless of the `unsafe` setting.

  **Inline parser** (`templateTagInlineParser`, implements `parser.InlineParser`):
  - `Trigger()` returns `[]byte{'{'}`.
  - `Parse()` checks for `{{` or `{%` at position, scans for matching `}}` or `%}` (including `{%-`/`-%}` whitespace-trimming variants). Also handles `{{% ... %}}` Go template block shortcode delimiters — when `source[start:start+3]` is `{{%`, scan for `%}}` as closer. On match: creates `TemplateTagInline` with tag text, advances reader. Handles multiline tags via line advancement (for `{{ "hello\nworld" | filter }}`). The `{{% %}}` check must come before the `{{ }}` check to avoid mismatching `{{% foo %}}` as an expression tag.
  - Priority: 90.

  **Block parser** (`templateTagBlockParser`, implements `parser.BlockParser`):
  - `Trigger()` returns `[]byte{'{'}`.
  - `Open()` matches a line containing EXACTLY ONE `{% ... %}` or `{{% ... %}}` tag with only whitespace before/after. For `{{% %}}` tags: check `trimmed[0:3] == '{{%'` and scan for `%}}` as closer. Must NOT match lines with multiple tags or mixed content (e.g., `{% if %}Visible{% endif %}` is NOT a block match — it contains text between tags). Creates `TemplateTagBlock`, returns `(node, NoChildren)`.
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
  - All placeholder logic in the render functions (consolidated into `RenderMarkdown` per issues #353, #700)

  **What remains unchanged**:
  - `escapeTemplateTags()` and `templateTagPattern` (used for `templateTags: false` path)
  - `CreateGoldmark()` signature and `MarkdownOptions` struct (used by callers to build the goldmark instance before passing to `RenderMarkdown`)
  - `RenderMarkdown()` signature `([]byte, goldmark.Markdown) → ([]byte, []TOCEntry, error)` — single consolidated function, no wrappers (issue #700)
  - All render hook logic in `render_hooks.go`
  - `escapeTemplateTagsInCode()` in `build.go` (runs after markdown rendering, independent)

  **TOC integration**: `extractText()` in `markdown.go` only collects `ast.Text` nodes for TOC entries. With the new extension, `TemplateTagInline` inside headings (e.g., `## {{ page.section_title }}`) must also contribute to TOC text. Update `extractText` to write `node.TagText` for `TemplateTagInline` nodes.
- `RenderText`: Wrap in `<pre>` tags.
- **Custom element block parsing (issue #784)**: Add `CustomElements bool` to `MarkdownOptions` and `CustomElements *bool` to `GoldmarkConfig` with `CustomElementsValue()` accessor (nil defaults to `true`, same pattern as `TemplateTags`). When enabled, a custom `BlockParser` (`customElementBlockParser`) detects HTML tags containing a hyphen in the tag name — custom elements per the HTML spec (`<alloy-code>`, `<wa-tab-group>`, `<my-widget>`, etc.) — and treats them as **HTML block type 1** (like `<pre>` and `<script>`). Content is preserved verbatim: no markdown processing, no smart quotes, no `<p>` wrapping. Blank lines inside the element do not terminate the block. The block ends only at the matching `</tag-name>` closing tag on any line. Nested custom elements are handled by matching only the outermost element's closing tag. The parser triggers on `<`, registers at priority 800 (before default HTML block parser at 900, after template tags at 50), and is registered via a `customElementsExtension` in `CreateGoldmark()` when `opts.CustomElements` is true. Standard HTML elements without hyphens are unaffected. Pipeline wiring: pass `CustomElements: cfg.Content.Markdown.Goldmark.CustomElementsValue()` in `MarkdownOptions` construction at `render.go`, `ssr.go`, and `state.go`.
- **Auto heading IDs + heading attributes (issue #274, #306)**: Add `AutoHeadingID bool` to `MarkdownOptions` (default true from `cfg.Content.Markdown.Goldmark.AutoHeadingIDValue()`). When true, enable `parser.WithAutoHeadingID()` and `parser.WithAttribute()` in goldmark parser options. When false, skip both — headings render without `id` attributes. These are goldmark core options, not extensions. Heading attributes (`{#custom-id .class}`) only work when `AutoHeadingID` is true. This is a goldmark-specific parser option — it only affects Markdown files; HTML and Liquid content files are not processed for heading IDs.
- **Block-level attribute parser extensions (issue #892)**: When `AutoHeadingID` is true (i.e., `parser.WithAttribute()` is enabled), register custom goldmark parser extensions that parse `{.class #id key=value}` attributes on fenced code blocks, blockquotes, and tables. No new config option — block attributes are automatically available when heading attributes are available. Attribute syntax follows Hugo's convention:
  - **Fenced code blocks**: attributes on the opening fence line after the language, e.g., `` ```go {.highlight} ``. The extension must parse the `{...}` portion from the info string without interfering with language detection — `n.Language(source)` must still return just `"go"`. Call `parser.ParseAttributes(source)` on the `{...}` portion and set attributes on the `FencedCodeBlock` AST node via `node.SetAttribute()`.
  - **Blockquotes**: trailing attribute block on the line immediately following the blockquote, e.g., `> text\n{.callout}`. The extension must consume the trailing `{...}` line and set attributes on the `Blockquote` AST node. The `{...}` line must not appear as a separate paragraph in the output.
  - **Tables**: trailing attribute block on the line immediately following the table, e.g., `| a |\n|---|\n| 1 |\n{.data}`. Same pattern as blockquotes — consume the trailing `{...}` line and set attributes on the `Table` AST node.
  
  Hugo's `goldmark` module (`github.com/gohugoio/hugo-goldmark-extensions`) is a reference implementation. The extensions should implement `goldmark.Extender` and be appended to the extensions slice in `CreateGoldmark()` inside the `if opts.AutoHeadingID` block. Each render hook function (`renderFencedCodeBlock`, `renderBlockquote`, `renderTable`) must extract attributes via `node.Attributes()` and add them to the `markup` context as `"attributes": attrs` — same pattern as `renderHeading` (already implemented in #824). When no attributes are present, `attrs` must be an empty `map[string]interface{}` (not nil).
- **TOC extraction (issue #274, #828)**: `RenderMarkdown` always walks the goldmark AST after parsing (before HTML rendering) and collects heading nodes (h2-h6, excluding h1) into a nested `[]TOCEntry` structure. `TOCEntry` has `ID`, `Text`, `Level`, `Children`. Returns the TOC alongside the rendered HTML as `([]byte, []TOCEntry, error)` (consolidated signature per issue #353). The pipeline conditionally stores the result on `page.TOC` based on `cfg.Content.Markdown.TOCValue()` — when `toc: false`, the TOC data returned by `RenderMarkdown` is discarded and `page.TOC` remains nil. The `toc` toggle is independent of `autoHeadingID` — headings still get `id` attributes when TOC is disabled. The `onContentTransformed` hook receives the page with `TOC` populated (or nil when disabled) — plugins can mutate it.
- **Render hooks (issues #273, #310, #311)**:
  
  **MarkdownOptions changes**: Add `Hooks map[string]string` (hook name → template source) and `HookRenderer func(templateSrc string, ctx map[string]interface{}) (string, error)` callback. The `content` package cannot import `template` (circular dependency), so the pipeline provides the renderer callback. When `HookRenderer` is nil, hooks in the `Hooks` map are ignored (no fallback to a bare Liquid environment).
  
  **Discovery (`internal/template/hooks.go`, issue #311)**: `DiscoverRenderHooks(layoutsDir string, engine string) (map[string]string, error)` scans `layouts/_markup/` for `render-{type}.{ext}` files. Extension matches the configured engine (`.liquid` for liquid, `.html` for gotemplate). Returns a map: `"codeblock" → templateSource`, `"codeblock-mermaid" → templateSource`, `"link" → templateSource`, etc. Missing `_markup/` directory is not an error (returns empty map). Unrecognized filenames are silently ignored. Valid hook names: `blockquote`, `codeblock`, `codeblock-{language}`, `heading`, `image`, `link`, `table`.
  
  **Pipeline wiring (issues #311, #353, #700)**: In `Build()`, after `createEngine` returns, call `tmpl.DiscoverRenderHooks(layoutsDir, engineName)` once. Build `mdOpts` with hooks and `HookRenderer` closure (wraps `engine.Parse()` + `tpl.Render()`), then call `content.CreateGoldmark(mdOpts)` to create the pipeline goldmark instance. Store it on `RenderContext.Goldmark` (`goldmark.Markdown`). `renderPages` calls `content.RenderMarkdown(body, rc.Goldmark)` — the single consolidated function that accepts a pre-built instance (issue #700). No longer constructs `mdOpts` or discovers hooks internally. Hook discovery moves from `renderPages` to `Build()` so the goldmark instance is fully configured before rendering starts. Parsed templates should be cached (parse once, render per node).
  
  **Goldmark integration**: For each hook type in `opts.Hooks`, register a custom `renderer.NodeRenderer` that builds the `markup.*` context from the AST node and calls `opts.HookRenderer(templateSrc, ctx)`. Context: `inner` from rendered children, `language`/`attributes` from fenced code block, `level`/`id` (custom `{#id}` attribute if present, otherwise auto-slug)/`inner` (rendered HTML)/`text` (plain text)/`attributes` (goldmark-parsed `{.class #id}` map) from heading, `src`/`alt`/`title` from image, `destination`/`text`/`title`/`is_external` from link, `inner`/`attributes` from blockquote, `inner`/`attributes` from table. Render hooks receive only `markup.*` — no `page.*` or `site.*` context. Heading `attributes` are extracted via `node.Attributes()` and converted to `map[string]interface{}` (issue #824). Block-level attributes for fenced code blocks, blockquotes, and tables follow the same extraction pattern via `node.Attributes()` (issue #892).
  
  **Heading hook edge cases (issue #896)**: Empty heading text with `{#id}` only — `extractText` returns empty, `slugifyHeading` returns empty, the explicit `id` attribute from `node.Attributes()` overrides the empty slug. Multiple nested inline elements (bold, links, code) — `renderChildrenToHTML` produces full HTML, `extractText` strips to plain text. Non-`[]byte` attribute values from third-party goldmark extensions — the `default` case in the type switch passes them through as-is to `markup.attributes`. HookRenderer errors propagate through the heading hook's `renderHookTemplate` call, returning `ast.WalkStop` and the error to `RenderMarkdown`.
  
  **Codeblock inner escaping (issues #785, #947, #962)**: `markup.inner` for codeblock hooks must apply two escaping steps before passing to the hook template: (1) Content-only HTML escaping via a new `escapeHTMLText` function — converts `&` → `&amp;`, `<` → `&lt;`, `>` → `&gt;` (ampersand first to avoid double-encoding). Quotes (`"`, `'`) are NOT escaped because codeblock inner is element content, not attribute context. Escaping quotes to `&#34;`/`&#39;` breaks downstream consumers like the shiki plugin whose `decodeHtmlEntities` does not decode quote entities, causing literal `&#34;` text to appear in highlighted output (issue #962). (2) Liquid delimiter escaping via `escapeLiquidDelimiters` — converts `{{`, `}}`, `{%`, `%}` to HTML entities so the template engine does not execute them. Content-only escaping must run first (on `codeBuf.String()`), then Liquid delimiter escaping on the result. The code in `renderFencedCodeBlock` in `render_hooks.go` must change from `html.EscapeString(codeBuf.String())` to `escapeHTMLText(codeBuf.String())`. The `escapeHTMLText` function must be added to `render_hooks.go` — it replaces `&` first, then `<`, then `>`, using `strings.ReplaceAll`. The `language` field continues to use `html.EscapeString` (it goes into an HTML attribute where quote escaping is correct). The `escapeLiquidDelimiters` function remains unchanged.

  **Render hook field escaping (issues #952, #953, #954)**: All render hook context fields carrying raw AST values must be wrapped in `html.EscapeString()` before assignment to the `markup` context map. Fields to escape: `language` in `renderFencedCodeBlock`; `destination`, `text` (from `extractText`), `title` in `renderLink`; `src`, `alt` (from `extractText`), `title` in `renderImage`; `text` (from `extractText`) and `id` in `renderHeading`. Fields that must NOT be escaped: `inner` in heading/blockquote/table hooks (already rendered HTML from `renderChildrenToHTML`), `level` (integer), `is_external` (boolean). Note: heading `id` is safe when it comes from `slugifyHeading` (strips non-alphanumeric chars), but unsafe when overridden by goldmark's quoted attribute syntax `{id="..."}` which preserves raw `&`, `<`, `"` in the value (issue #954). Apply `html.EscapeString` unconditionally since both paths feed the same field — escaping a slug is a no-op. Each field gets a simple `html.EscapeString(value)` wrapper at the assignment site in its respective render function.

  **Block-level raw HTML in markup.inner (issue #955)**: `escapingRawHTMLRenderer` in `render_hooks.go` must register a handler for `ast.KindHTMLBlock` in addition to the existing `ast.KindRawHTML` handler. The block handler reads the node's `Lines()` segments (same pattern as the inline handler's `Segments`) and writes each through `html.EscapeString`. Register in `RegisterFuncs` alongside the existing `ast.KindRawHTML` registration. The renderer is already added to the child goldmark instance at priority 999 in `CreateGoldmark` — no wiring changes needed.

**lifecycle.go**:
- `FilterByLifecycle`: Exclude drafts (unless includeDrafts), future publishDate, past expiryDate.

**Testdata exists**: `internal/content/testdata/site1/` with full content directory

**Verify**: `go test ./internal/config/... ./internal/content/...`

---

## Phase 3: Mid-Level Packages (~79 tests)

### 3A: `internal/permalink` — 44 tests
**File**: `internal/permalink/permalink.go`

- `ResolveTokens`: Replace `:year`, `:month`, `:day`, `:slug`, `:section`, `:filename`, `:title`
- `PermalinkRenderer`: Type alias `func(source string, ctx map[string]interface{}) (string, error)`. Callback provided by the pipeline from the configured template engine.
- `Resolve(pattern, page, renderer ...PermalinkRenderer)`: Front matter permalink > pattern with tokens. Handle `permalink: false`. **Template permalink rendering (issue #830)**: When a front matter permalink string contains `{{`, render it through the provided `PermalinkRenderer` callback instead of returning it verbatim. Token resolution (`:year`, `:slug`, etc.) is skipped entirely in template mode — the two modes are mutually exclusive. The renderer receives a `page` context built from `page.ToTemplateMap()`, wrapped as `map[string]interface{}{"page": pageMap}`. `page.url` is excluded from the context (circular reference — it is the value being computed). **Empty render is a fatal error**: if the renderer returns an empty or whitespace-only string (after trimming), return an error. This is distinct from `permalink: false` which returns `("", nil)` intentionally.
- `ContainsLiquidTags`: Check for `{{` in string (name is legacy — detects both Liquid and Go template syntax since both use `{{`)
- `DefaultFromPath`: `blog/my-post.md` -> `/blog/my-post/`. Handles index files: `index.md` → `/`, `blog/index.md` → `/blog/`, `blog/post/index.md` → `/blog/post/` (strips `/index` suffix).
- `ResolveFromCascade(page, cascadeData, renderer ...PermalinkRenderer)`: The production permalink resolution path. Lookup order: (1) front matter permalink — static string returned verbatim, `permalink: false` returns `("", nil)`, template `{{ }}` rendered through renderer; (2) index file bypass — `isIndexFile(page.RelPath)` skips to `DefaultFromPath`, preventing cascade patterns from turning `index.md` into `/home/` (issue #39); front matter `permalink:` (step 1) still overrides index bypass, enabling subdirectory deployments; (3) cascade `"permalink"` pattern from `_data.yaml` — token patterns resolved via `ResolveTokens`, template `{{ }}` patterns rendered through renderer; (4) `DefaultFromPath` fallback. Accepts nil `cascadeData` (common when `FindCascadeData` finds no ancestor `_data.yaml`). Empty/whitespace render results produce a fatal error, distinct from `permalink: false`. `ResolveForSection` was removed in issue #915 — it used a flat `map[string]string` section→pattern lookup that was superseded by per-page cascade resolution in issue #910.
- `ResolveAliases`: Return page's Aliases slice

### 3B: `internal/collection` — 38 tests
**Files**: `collection.go`, `taxonomy.go`

- `BuildCollections(pages, permalinkCfg, collectionNames []string)`: Group pages by section. A section becomes a collection if it has date-based permalink tokens `:year`/`:month`/`:day` **or** is listed in `collectionNames` (extracted from `cfg.Collections` keys). Both sources seed a single membership map; a section qualifying via both mechanisms produces one collection (no duplication). `collectionNames` may be nil (backward-compatible, date-tokens-only behavior). The `permalinkCfg` map must come from cascade data only (see §4D pipeline step 6, issue #832).
- `BuildCollectionsWithMode(pages, permalinkCfg, collectionNames []string, devMode bool)`: Lifecycle-aware wrapper — applies lifecycle filtering (drafts, future `publishDate`, past `expiryDate`) via `content.FilterByLifecycle` before calling `buildCollectionsIncludeAll`. `devMode=true` includes drafts; `devMode=false` excludes them. Future/expired pages are always excluded.
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
- `ToMap()` must convert `site.pages` items via `ToTemplateMap()` (issue #712) — same treatment as collections and taxonomies. Raw `[]*content.Page` structs are not accessible to Liquid filters (`where`, `sort`, `map`, `group_by`) because `getMapValue()` only handles `map[string]interface{}`. Convert to `[]interface{}` of template maps so front matter fields are top-level properties.
- `TemplateContext` struct additions: `Pagination *pagination.PaginationContext` field, `Custom map[string]interface{}` field for dynamic top-level variables (the `as` alias).
- `ResolveLayout`: Lookup chain per spec (front matter > post (date-based) > section name (index) > default). Handle `layout: false`. **No filename matching (issue #902)**: Automatic filename-based layout lookup is removed — `filenameWithoutExt` is no longer used as a candidate in `ResolveLayout` or `ResolveLayoutForFormat`. When `layout:` is explicitly set (front matter or cascade) and the file does not exist on disk, return a build error immediately — do not fall through to auto candidates. **Bare-extension fallback (issue #827, revised #860)**: When engine is `"liquid"`, each automatic candidate (post, section, default) tries `<name>.liquid` first, then `<name>.html` (per-candidate interleaving, not global). Front matter `layout:` names are checked first but follow the bare-name vs. extension-bearing distinction: bare names (no recognized extension, e.g., `layout: "custom"`) get the same `.liquid` → `.html` fallback; extension-bearing names (e.g., `layout: "custom.html"`) are used as literal filenames with no extension appended. If the explicit name resolves to nothing (neither extension found for bare, or file missing for extension-bearing), the build errors — no fall-through to auto candidates. Recognized extensions: `.liquid`, `.html`, `.xml`, `.json`, `.txt`. Implementation: add a `hasRecognizedExtension(name)` helper; if true, use the name as-is; if false, try `name.liquid` then `name.html` (same interleaving as auto candidates). The Go engine is unchanged — single extension, no fallback.
- `ResolveLayoutWithCascade(page, layoutsDir, engine, permalinkCfg, cascadeData)`: Same lookup chain as `ResolveLayout`, but also considers `_data.yaml` cascade data for layout resolution. Front matter takes priority over cascade data. **Explicit layout names follow the bare-name vs. extension-bearing distinction** (issue #860): bare names get `.liquid` → `.html` fallback; extension-bearing names are used as literal filenames. If the explicit name resolves to nothing, the build errors — no fall-through to auto candidates. Both the front matter and cascade short-circuit paths must apply the same `hasRecognizedExtension` check.
- `ResolveLayoutForFormat(page, layoutsDir, engine, format, permalinkCfg)`: **Unified format layout resolution (issue #864)**: Uses the same chain as `ResolveLayout` with the format infixed. Signature adds `permalinkCfg` to detect date-based sections. Lookup: front matter `layout:` with format infixed → "post" with format (date-based child) → section name with format (index page) → "default" with format → error. **No filename matching (issue #902)**: filename-based format candidates removed. Handles `layout: false` (returns `""`, no error). No `single` concept — deleted. **Bare-extension fallback (issue #827)**: Liquid engine tries `<name>.<format>.liquid` first, then `<name>.<format>` at each step. **Go engine bare format extensions (issue #834)**: Go engine uses `<name>.<format>` directly — the format extension IS the file extension, no `.html` engine suffix appended. `formatLayoutCandidates` must return `[name.format]` for gotemplate (e.g., `default.json`, `post.xml`), not `[name.format.html]`. This is consistent with the Go engine's bare-extension rule: Go templates read `feed.xml`, `api.json`, `default.html` — the file extension determines the output format. **Extension-bearing layout guard (issue #869)**: Before format infixing, check if the explicit layout name (from front matter) is extension-bearing via `hasRecognizedExtension`. If so, return a build error — extension-bearing names bypass format infixing and would silently serve the wrong template. Error message must suggest the bare name alternative (e.g., `use layout: article instead`).
- `ResolveLayoutForFormatWithCascade(page, layoutsDir, engine, format, permalinkCfg, cascadeData)`: **Cascade-aware format layout resolution (issue #864)**: Same as `ResolveLayoutForFormat` but also considers `_data.yaml` cascade data. Front matter takes priority over cascade. Falls through to `ResolveLayoutForFormat` auto candidates when no explicit layout set. **Extension-bearing layout guard (issue #869)**: Same guard applies to cascade layout names — extension-bearing cascade layouts produce the same build error when used with format outputs.
- `ResolveTaxonomyLayout`: `layouts/taxonomies/<name>.liquid` > `layouts/<name>.liquid`. **Bare-extension fallback (issue #827)**: When engine is `"liquid"`, each candidate tries `.liquid` first, then `.html`. Example: `taxonomies/tags.liquid` → `taxonomies/tags.html` → `tags.liquid` → `tags.html`.
- `ResolveLayoutChain`: **Bare-name vs extension-bearing distinction (issue #827, revised #860)**: Parent layout references in `layout:` directives follow the same rules as front matter/cascade: use `hasRecognizedExtension(name)` — if true, use the name as a literal filename; if false, try `<parent>.liquid` first, then `<parent>.html` (Liquid engine) or `<parent>.html` only (Go engine). Missing parent = build error.
- `RegisterShortcode`/`RenderShortcodes`: Registry + inline expansion

### 3D: `internal/output` — 21 tests
**Files**: `writer.go`, `feed.go`, `sitemap.go`, `formats.go`

- `WriteFile`/`CleanOutputDir`/`ComputeOutputPath`/`WriteAliases`/`ShouldWrite`
- **Note**: Feeds use the standard multi-format output mechanism (`outputs: ["html", "xml"]` + format layouts). No dedicated feed discovery or rendering code — `ResolveFeedTemplates`/`RenderFeedTemplate` were removed in issue #822.
- `GenerateSitemap`: XML sitemap with baseURL prefix, per-page exclusions. Skipped when `cfg.Sitemap.Enabled` is false (issue #825). `SitemapConfig` has a custom `UnmarshalYAML` to accept `sitemap: false` (sets `Enabled: false`) alongside the object form. `ApplyDefaults` sets `Enabled: true` when not explicitly disabled.
- `ResolveOutputFormat(page) string`: Returns first entry from `page.Outputs`, defaulting to `"html"` when unset.
- `ResolveFormatLayout`: **Deleted (issue #864)** — format layout resolution is now unified with the standard chain in `internal/template.ResolveLayoutForFormat`. This function was a duplicate resolver with a hardcoded `single` default.

#### Multi-format pipeline wiring (issue #71)

The pipeline currently renders each page once (HTML only). Pages with `outputs: ["html", "json"]` need to render once per format. The wiring change is in Stage 5 (layout resolution and rendering, `build.go` ~line 141):

```
// Current: single render per page
for _, page := range pages {
    layoutPath := tmpl.ResolveLayout(page, layoutsDir, engineName, permalinkCfg)
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
            // Format-specific: unified chain with format infixed (issue #864).
            // Uses the same lookup order as ResolveLayout (front matter → post →
            // section name → filename → default) with .<format> before the engine ext.
            if cascadeData != nil {
                layoutPath, err = tmpl.ResolveLayoutForFormatWithCascade(
                    page, layoutsDir, engineName, format, permalinkCfg, cascadeData)
            } else {
                layoutPath, err = tmpl.ResolveLayoutForFormat(
                    page, layoutsDir, engineName, format, permalinkCfg)
            }
            // Output path: replace extension — /my-post/index.json instead of index.html
        }
    }
}
```

Key points:
- `page.Outputs` is populated from `outputs` front matter during `content.BuildPage` (the field already exists on `content.Page`)
- HTML format uses existing `template.ResolveLayout` (no change). Non-HTML formats use `template.ResolveLayoutForFormat(page, layoutsDir, engine, format, permalinkCfg)` — the unified resolver that mirrors the HTML chain with format infixed and validates the layout file exists on disk. The old `output.ResolveFormatLayout` is deleted (issue #864) — it was a duplicate with hardcoded `single` default.
- When cascade data is available, use `template.ResolveLayoutForFormatWithCascade(page, layoutsDir, engine, format, permalinkCfg, cascadeData)` instead.
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

### 4B: Go Template Engine (~22 tests)
**File**: `internal/template/gotemplate.go`

- Adapt `html/template` to `TemplateEngine` interface via `FuncMap`
- **Includes via `include` function (issues #823, #883)**: Register an `"include"` FuncMap function that resolves and renders partial templates from the layouts directory. Named `include` for parity with Liquid's `{% include %}`. Implementation:
  - Add `SetIncludesDir(dir string)` method to `goEngine` (same interface as `liquidEngine`). The pipeline already calls this at `InitPipelineState` (state.go:62-64) via interface assertion.
  - The `include` function signature: `func(path string, dot interface{}) (gohtml.HTML, error)`. Fixed arity — takes a path string and the current dot. Returns `gohtml.HTML` so output is not double-escaped.
  - **Parse-time dot injection** (`injectIncludeDot`): A regex rewrites `{{ include "path" }}` → `{{ include "path" . }}` before parsing, so Go's template engine passes the *current* dot as the explicit second argument. This is necessary because FuncMap functions are plain Go functions with no access to the calling template's current dot — a variadic signature with pointer-based render context capture (the original plan) always receives the *root* context, producing incorrect behavior inside `{{ range }}` and `{{ with }}` blocks where Go rebinds dot. The parse-time rewrite is the only mechanism that delivers Liquid `{% include %}` scope-sharing semantics within `html/template`. Explicit context (`{{ include "partials/card" (dict "item" . "compact" true) }}`) bypasses the rewrite since the second argument is already present.
  - Resolution: try `layoutsDir/path.html` only (issue #880). The developer must remove the raw path fallback (extensionless file candidate) from both the Go engine's `resolveInclude` candidates slice and Liquid's `ReadTemplateFile` candidates slice. Resolved templates are cached in a `sync.Map` keyed by path.
  - Path sandboxing: `filepath.Rel(absRoot, absPath)` must not start with `..`. Build error on traversal.
  - Parse with the engine's FuncMap (so filters, `dict`, `include` itself are available in nested includes).
  - Nesting depth: engine-scoped `atomic.Int32` (`goEngine.depth`). Increment on entry, decrement on exit (via defer). Build error at depth > 100 with message containing "too deep" or "nesting" (matches Ruby/Go Liquid's `StackLevelError`). The counter is thread-safe (atomic), but concurrent renders on the same engine instance share it — under heavy concurrent load this could produce false "nesting too deep" errors. Currently latent: the pipeline renders sequentially.
  - Missing include file: build error with clear message.
  - The `include` function is registered in the FuncMap in two stages: `NewGoEngine()` registers an error stub (Go's `html/template` requires all FuncMap entries at parse time), then `SetIncludesDir()` replaces it with the real closure that calls `renderInclude`. Templates parsed before `SetIncludesDir` retain the stub and error on use.
- **`*ordered.Map` compatibility (issue #458)**: Go templates use reflection — `*ordered.Map` is a struct, not a map or slice, so neither `{{ index }}` nor `{{ range }}` work natively. Converting to `map[string]interface{}` enables property access but loses iteration order; converting to `[]KVPair` enables ordered iteration but breaks key-based lookup. These are mutually exclusive on the same value. Fix: keep `*ordered.Map` as the context value and register FuncMap helpers in the Go template engine:
  - **`oget`** (`{{ oget .site.data.tokens "white" }}`): calls `m.Get(key)` on `*ordered.Map`, returns the value. Falls back to `index` for regular maps.
  - **`orange`** (`{{ range orange .site.data.tokens }}`): calls `m.Entries()` on `*ordered.Map`, returns `[]KVPair` for ordered iteration. Each entry has `.Key` and `.Value`.
  
  Register both in `goEngine.AddFilter` or directly in the FuncMap during engine creation. The `*ordered.Map` in `siteData` is never mutated. Liquid uses it directly via `Each` and `LiquidMethodMissing`.

### 4B: `internal/fileutil` — 3 tests (issue #782)
**File**: `internal/fileutil/copy.go`

- `CopyFile(src, dst string) error`: Copies a file from `src` to `dst`, preserving the source file's permissions. Creates parent directories of `dst` via `os.MkdirAll`. Propagates `os.ErrNotExist` from `os.Stat` when the source file does not exist — callers must check `os.IsNotExist(err)` if a missing source is expected (e.g., transient files in the dev server watcher).

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

### 4D: `internal/pipeline` — 38 tests
**File**: `internal/pipeline/build.go`

- `BuildWithContent(cfg, contentMap, opts ...BuildOptions)`: Thin wrapper around `Build()`. Writes `contentMap` entries to a temp directory preserving path structure (e.g., `"content/index.md"` → `tmpDir/content/index.md`, `"layouts/default.liquid"` → `tmpDir/layouts/default.liquid`), sets `cfg.ProjectRoot = tmpDir`, and calls `Build(cfg, opts...)`. This ensures every pipeline stage runs — plugins, hooks, data cascade, collections, lifecycle filtering, layout chaining, SSR, validation, output. Zero divergence from `Build()`. The temp directory is cleaned up after `Build()` returns. `BuildWithContent` must NOT duplicate any pipeline logic — it is purely file setup + delegation (issue #283). **CaptureRenderedContent override (issue #1098)**: Before delegating to `Build()`, `BuildWithContent` must force `CaptureRenderedContent: true` on the resolved `BuildOptions`. If the caller passed no options, create `BuildOptions{CaptureRenderedContent: true}`. If the caller passed options (e.g., `BuildOptions{SkipSSR: true}`), set `CaptureRenderedContent = true` on the resolved struct before passing to `Build()`. This preserves backward compatibility for all existing tests that read `result.RenderedContent`.
- `BuildIncremental(cfg, contentMap, previousCache, changedFiles)`: Dev-mode incremental rebuild. Accepts a previous `*cache.Cache` (loaded by the caller, not by this function) and list of changed file paths. Discovers all pages, skips pages where `cache.ShouldSkipFile` returns true (unchanged), and renders pages where it returns false (changed) or that were invalidated via `cache.InvalidatedPages` for layout changes. Returns `BuildResult` with `PagesSkipped` count. When `previousCache` is nil, renders all pages (equivalent to full build). **Disk output (issue #581)**: `BuildIncremental` must write rendered pages to the output directory (same as `Build()`). Extract the page-writing loop from `Build()` into a shared function and call it from both code paths. Only changed pages are written — asset/passthrough copying is handled by existing watches. Integration test in `internal/pipeline/build_test.go` verifies the output file on disk contains updated content after incremental rebuild. **Stale PipelineState.SiteData (issue #717)**: When `BuildOptions.PipelineState` is provided (the `cmd/dev.go` pattern), `ps.SiteData` was loaded once at server startup via `InitPipelineState` → `loadSiteData`. If data files change on disk during a dev session, `ps.SiteData` is stale — `processPagination` resolves `site.data.*` references against the old data, causing paginated virtual pages to lose their data or reflect outdated values. Fix: when `changedFiles` contains any path under `cfg.Structure.Data`, `BuildIncremental` must re-load site data from disk via `loadSiteData(cfg)` and update `ps.SiteData` before calling `processPagination`. After refreshing `ps.SiteData`, plugin runtimes that cache site data (e.g., QuickJS `alloy.data` via `rt.SetSiteData`) must also be re-injected — otherwise plugin filters/hooks still see stale data. Data file changes must also invalidate all paginated pages that reference the changed data source for re-rendering, even when the source template's content hash is unchanged.
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
    SkipSSR                bool               // true = skip Phase 2 entirely, regardless of cfg.SSR
    PipelineState          *PipelineState     // pre-built state to reuse (BuildIncremental only)
    Profile                bool               // true = record per-stage timing in BuildResult.StageTimings
    Reporter               ProgressReporter   // progress output; nil = silent
    CaptureRenderedContent bool               // true = populate BuildResult.RenderedContent (issue #1098)
}

func Build(cfg *config.Config, opts ...BuildOptions) (*BuildResult, error)
```

Note: The snippet above shows the complete struct. `PipelineState`, `Profile`, and `Reporter` are existing fields — only `CaptureRenderedContent` is new in this issue.

The variadic pattern keeps existing callers working (`Build(cfg)` still compiles). When `opts` is provided and `SkipSSR` is true, the pipeline skips Phase 2 entirely and sets `result.SSRSkipped = true`. The SSR check becomes:

```go
ssrSkipped := cfg.SSR == nil || (len(opts) > 0 && opts[0].SkipSSR)
```

`cmd/dev.go` always passes `pipeline.BuildOptions{SkipSSR: true}` — for both the initial `Build()` and watcher `BuildIncremental()` calls. `cmd/build.go` and `cmd/serve.go` call `Build(cfg)` with no options (SSR runs if configured). `BuildIncremental` gets the same variadic parameter.

**`CaptureRenderedContent` (issue #1098)**: When true, `Build()` and `BuildIncremental()` populate `BuildResult.RenderedContent` by iterating all pages and calling `page.HTML()`. When false (the default — Go zero-value), the `renderedContent` map population loop is skipped entirely: no `make(map[string]string, ...)`, no `page.HTML()` calls (avoiding the `renderedStr` caching allocation), and `BuildResult.RenderedContent` is set to nil. The flag is independent of `SkipSSR` — both are off by default and neither implies the other. Production callers (`cmd/build.go`, `cmd/dev.go`, `cmd/serve.go`) must NOT set this flag. `BuildWithContent()` — the test utility function — must always force `CaptureRenderedContent: true` on the resolved options before delegating to `Build()`, regardless of whether the caller passed `BuildOptions` or what value they set. This preserves backward compatibility for all existing tests that read `result.RenderedContent`.

#### `Build()` full orchestration (issue #30)

`Build()` must orchestrate all pipeline stages from §2. Currently it stops after markdown+template rendering. The individual packages for each stage are implemented and pass tests — they need to be called in order:

```
 1. config.ApplyDefaults(cfg)                           ✅ done
 2. validateOutputDir(cfg)                               ✅ done
 3. content.DiscoverWithFormats(contentDir, formats)      ✅ done
 4. content.FilterByLifecycle(pages, now, includeDrafts)  ✅ done (issue #108: must pass includeDrafts from server mode, not hardcode false)
 5. cascade.LoadDirectoryCascade + FindCascadeData + PageContext ✅ done (was step 6, reordered for #302)
 6. permalink.ResolveFromCascade(page, cascadeData, renderer)  ← CHANGED (issues #830, #910, #914)
    For each page, look up per-page cascade data via
    `cascade.FindCascadeData(ps.CascadeData, ps.ContentBase, relPath)` and
    pass it to `ResolveFromCascade` (`relPath` is `page.RelPath` in single-language
    builds; in multi-language builds it is the original un-stripped path — see
    issue #914 note below). `ResolveForSection` was removed in issue #915 —
    it flattened cascade data into a section-name-only map, silently dropping nested
    `_data.yaml` permalink patterns (issue #910). `buildPermalinkCfg` may still be
    needed for `BuildCollections` date-based section detection (step 8) but must no
    longer be used for permalink resolution. Config loaders (YAML/TOML/JSON) silently ignore
    unknown `permalinks:` key in old config files — decoders do not reject unknown
    keys (issue #832).
    **Multi-language cascade lookup (issue #914)**: In multi-language builds, the
    pipeline strips the language prefix from `page.RelPath` before permalink
    resolution to prevent URL doubling (e.g., `/es/es/about/`). The `FindCascadeData`
    call must use the ORIGINAL (un-stripped) `page.RelPath` — e.g.,
    `es/blog/my-post.md` — so it finds cascade keys like `content/es/blog/`.
    Only `ResolveFromCascade` receives the stripped `page.RelPath` (e.g.,
    `blog/my-post.md`) for token resolution and `DefaultFromPath`. The language
    output prefix is applied to the resolved URL afterward. Without this,
    `FindCascadeData` receives `blog/my-post.md`, looks for `content/blog/`, and
    misses language-specific `_data.yaml` entries entirely.
    **Template permalink rendering (issue #830)**: The pipeline must construct a
    `permalink.PermalinkRenderer` callback from the configured template engine and
    pass it to `ResolveFromCascade`. The callback parses and renders the permalink
    string with a `{"page": page.ToTemplateMap()}` context. When the configured
    engine is `"gotemplate"`, the callback must use the Go template engine — it
    must NOT fall back to Liquid. When no engine is explicitly configured, use
    the default Liquid engine.
 7. data.LoadDirectory(dataDir) → siteData                ✅ done
 8. collection.BuildCollections(pages, cascadeData, collectionNames)  ← CHANGED (issues #302, #766)
    Date-based section detection reads permalink from each section's
    _data.yaml cascade instead of Config.Permalinks map.
    `collectionNames` extracted from cfg.Collections keys — sections listed
    there become collections regardless of permalink pattern (issue #766).
    `isDateBasedSection` in layout.go also needs cascade data.
 9. collection.BuildTaxonomies(pages, taxonomies)         ✅ done
 9a. validation.ValidatePermalinkAliases(pages)            ✅ done — **must run before rendering (issue #690)**
 9b. validation.DetectConflicts(outputEntries)              ✅ done — **must run before rendering (issue #690)**
     Build output entries from page.URL, page.Outputs, page.Aliases,
     AND taxonomy page URLs (issue #695). Taxonomy URLs are deterministic
     from batches[i].taxonomies (built in step 9) and cfg.Taxonomies config:
     - Index page: `/<taxonomy-name>/` (e.g., `/tags/`)
     - Term pages: permalink pattern with `:slug` replaced (e.g., `/tags/golang/`)
     - Only include taxonomies where ShouldRender() is true (render: false
       taxonomies build collection data but do not generate output pages)
     No rendering needed — just URL computation from already-available data.
     All inputs are finalized after onPagesReady (step 9 in §2).
     If conflicts detected, return error immediately — no rendering occurs.
     Catches authoring errors (duplicate permalinks, alias collisions,
     authored pages overwriting taxonomy pages) in milliseconds instead
     of after 10+ seconds of wasted rendering.
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
     Dev mode also writes to `_site/` — all modes serve from the output directory.
19. output.GenerateSitemap(pages, baseURL, outputDir)     ✅ done
20. cache.SaveTo(cacheFile)                               ✅ done
```

**Key implementation notes:**
- Steps 5-9 happen before rendering (step 11) so templates can access `page.url`, `collections.*`, filters, etc.
- **Early validation (issue #690)**: Steps 9a-9b (permalink/alias validation, conflict detection) run after all permalink data is finalized (step 6) and after `onPagesReady` injects virtual pages, but before any rendering work. This ensures authoring errors fail fast without wasting render time. The validation code is the same — only its position in the pipeline changes.
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
  - **HookRegistry**: Add `RegisterWithPriority(event, fn, priority int)`. `Register(event, fn)` delegates to `RegisterWithPriority(event, fn, 50)`. `hooks` field changes from `map[HookName][]HookFunc` to `map[HookName][]priorityHook` where `priorityHook` has `fn HookFunc`, `batchFn BatchHookFunc`, `priority int`, `index int`, and `scope *HookScope`. Use insertion sort at registration time to maintain sorted order — do NOT sort per-call in `Run`/`RunWithTimeout` (those fire per-page, sorting per-call is wasteful). Stable insertion preserves registration order for same-priority hooks. **Zero-item batch guard (issue #972)**: `RunBatchWithProgress` must skip each hook when `len(current) == 0`. Add `if len(current) == 0 { continue }` at the top of the `for _, h := range hooks` loop, before the `if h.batchFn != nil` branch. This covers both the batch path and the per-item fallback. Without this guard, `effectiveTimeout = r.timeout * 0 = 0`, creating an already-expired context that triggers a spurious warning for every registered batch hook.
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
    1. **Pageless hooks** (`OnConfig`, `OnBeforeValidation`, `OnAfterValidation`, `OnDataFetched`, `OnAssetProcess`, `OnBuildComplete`, `OnDevServerStart`, `OnFileChanged`) do not receive pages. Any `Pages.Mode` other than `PagesScopeNone` is rejected with an error mentioning "pages". These hooks receive per-build payloads (config, paths, data) — page filtering is meaningless. The caller (`registerRuntime` in `registry.go`) surfaces this error as a **warning** via `hooks.addWarning()` — the build proceeds.
    2. **Pre-taxonomy hooks** that receive pages (`OnPagesReady`) reject `PagesScopeTaxonomy` with an error mentioning "taxonomy" — taxonomy indices are not built yet at step 9.
    
    Post-taxonomy hooks that accept all page scope modes: `OnContentLoaded`, `OnDataCascadeReady`, `OnContentTransformed`, `OnPageRendered`. `PagesScopeNone` is valid on all hooks. Called at plugin load time — warnings surface before the build starts.
  - **Omitted pages default (issue #977)**: In `parseScopeMap`, the `case nil` branch (omitted/null `pages` key) must set `scope.Pages.Mode = PagesScopeNone`, not `PagesScopeAll`. This aligns with the documented default (`pages: false`) and eliminates two compounding defects: (1) spurious validation warnings on every pageless hook registered with `{}` or `{data: [...]}`, and (2) unnecessary full-page serialization across the bridge when the plugin never requested pages. `parseScopeJSON` delegates to `parseScopeMap`, so both paths are fixed by this single change. Plugins that want pages must explicitly declare `pages: true`.
  - **`addPages` return shape (issue #971)**: `runOnPagesReady` must support a second return shape alongside the existing `{ pages: [...] }` full-array return. Implementation changes in `runOnPagesReady`:
    1. **Send-side: honor `Pages.Mode` on `onPagesReady`**. Remove the `"Pages.Mode intentionally ignored"` comment and guard. When the union scope's `Pages.Mode` is `PagesScopeNone`, set `serialized` to nil (skip the page serialization loop). This matches the pattern in `serializePagesForHook`. The `HookPagesReadyPayload.Pages` field will be nil, serialized as `null` in JSON. When `Pages.Mode` is not `PagesScopeNone`, serialize pages as today.
    2. **Return-side: dispatch on return shape**. After `toGoMap(result)`, check for both `addPages` and `pages` keys in the returned map:
       - **Both present** → return error: `"onPagesReady returned both 'pages' and 'addPages' — use one or the other"`.
       - **`addPages` only** → type-assert as `[]interface{}` (if type assertion fails, return error: `"onPagesReady: addPages must be an array"`). Iterate entries. For each entry: `toGoMap` → `virtualPageFromMap` → URL collision check against `urlIndex` → append to `pages`. Same validation and error messages as the existing virtual page loop. The `urlIndex` must be seeded from existing pages before processing `addPages` entries.
       - **`pages` only** → type-assert as `[]interface{}`. A `null`-valued `pages` key (when the hook echoes back the payload from a `pages: false` scope, where `HookPagesReadyPayload.Pages` was nil) must be treated as absent — not as the full-array return shape. Check: if `resultMap["pages"]` is present but type-asserts to `nil` (i.e., `.([]interface{})` returns `nil, false`), treat as no-op. Only a non-nil array triggers the full-array behavior. This ensures `return payload` echo from `pages: false` hooks is a safe no-op.
       - **Neither key** (map has other keys but not `pages` or `addPages`) → return error: `"onPagesReady returned unrecognized shape — expected \"pages\" or \"addPages\""`.
       - **Non-map result** (`toGoMap` returns false) → no-op (`return pages, nil`), preserving backward compatibility with hooks that return nothing.
    3. **Shared `urlIndex`**: The URL collision index must be built from existing Go-side pages regardless of return shape. For `addPages`, the index is seeded then checked for each new page. For `pages`, the existing `urlIndex` code is unchanged. The `len(returned) >= originalCount` check on the `pages` path uses the Go-side page count (before serialization), not the serialized count — a `pages: false` hook that fabricates a `{pages: [...]}` return with fewer entries than the Go-side count is correctly rejected.
    4. **Multi-hook invocation (issue #971)**: `runOnPagesReady` must NOT use `RunWithTimeout` for `onPagesReady` when multiple hooks are registered. `RunWithTimeout` chains results (`current = result`), but `addPages` return shape ≠ payload shape — passing `{addPages: [...]}` as the next hook's input breaks it. Instead, `runOnPagesReady` must iterate hooks individually (either via a new `HookRegistry.RunEach`-style method, or by accessing the hook list directly). After each hook: apply the return (append addPages entries or process pages array), rebuild the canonical `HookPagesReadyPayload` from the updated Go-side pages, and pass it to the next hook. URL collision checks accumulate across hooks. If only one hook is registered, the behavior is identical to a single `RunWithTimeout` call.
  - **Scope-aware serialization**: Each serialization site in the pipeline (`runOnPagesReady`, `serializePagesForHook`, `fireContentTransformedHooks`, etc.) calls `registry.ScopeFor(event)` to get all scopes, computes the union of requested fields across all hooks for that event, and serializes only the union. If any scope requests `Data: ["*"]`, all site data is serialized. If any scope requests `Pages.Mode == PagesScopeAll`, all pages are serialized. If all scopes have `Pages.Mode == PagesScopeNone`, no pages are serialized. Glob and taxonomy filters are applied after the union computation — pages matching ANY hook's filter are included. `PageFields` union determines which fields are populated on each `HookPagePayload`. **Union scope is not an isolation boundary (issue #543)**: The union means all hooks on the same event receive the same superset payload — if hook A requests `["html"]` and hook B requests `["toc"]`, both see `["html", "toc"]`. This is a performance optimization (one serialization per event, not per hook), not a security or privacy mechanism. Plugin authors must not rely on scoping to hide fields from other plugins. **Taxonomy term dedup (issue #546)**: When merging `PagesScopeTaxonomy` scopes, `computeUnionScope` must deduplicate terms per taxonomy key. Two hooks scoping `tags: ["go"]` must produce `tags: ["go"]`, not `tags: ["go", "go"]`. Use a set during merge.
  - **Bridge updates**:
    - **QuickJS** (`wasm.go`): `alloy.hook(name, opts, fn)` at line 96-99 — swap second and third arguments. `__registerHook` Go callback at line 71-79 — receives `(name, priority, scopeJSON)` instead of `(name, priority)`. Parse `scopeJSON` into `HookScope`. Scope validation happens later in `registry.go` during `registerRuntime()`, not in the `__registerHook` callback itself. **QuickJS duplicate hook detection (issue #558)**: `__registerHook` callback must detect duplicate hook names (checked against `r.hooks` map) and append a warning to `evalWarnings`. Warning format matches Node bridge: `"duplicate hook registration: \"<name>\" registered multiple times, last registration wins"`. `EvalWarnings() []string` accessor returns accumulated warnings. Last-wins semantics preserved (map key overwrites). **EvalWarner forwarding (issue #562)**: `registerRuntime` in `registry.go` must check if the runtime implements `EvalWarner` and, if so, forward accumulated `EvalWarnings()` to `HookRegistry.warnings` with a `"plugin <name>: "` prefix. This surfaces QuickJS duplicate warnings through the same `HookRegistry.Warnings()` API that surfaces timeout warnings. **Both `__registerHook` code paths (issue #562)**: The duplicate detection check (`r.hooks[name]` existence) must fire before the `len(args) >= 2` branch, so it applies to both the 3-arg path (priority + scope from `alloy.hook()`) and the 1-arg fallback path (default priority 50, no scope). Test fixture `duplicate-hook-no-scope.js` exercises the 1-arg path directly via `__registerHook("hookName")` calls.
    - **Node** (`node.go`): Hook registration in eval response includes scope JSON alongside hook name and priority. `loadPluginRuntime` parses scope and calls `RegisterWithOptions`. **Eliminate double JSON round-trip (issue #545)**: `EvalFile` currently marshals the Go `map[string]interface{}` scope to JSON via `json.Marshal`, then immediately parses it back via `parseScopeJSON`. Refactor: extract the type-switch logic from `parseScopeJSON` into `parseScopeMap(m map[string]interface{}) (*HookScope, error)` which builds `HookScope` directly from a Go map via type assertions. `parseScopeJSON` becomes a thin wrapper: unmarshal JSON → call `parseScopeMap`. `EvalFile` calls `parseScopeMap` directly, eliminating the round-trip. One code path for the polymorphic `pages` handling. **Warn on duplicate hookScope clobber (issue #544)**: Detection must happen in `bridge.js` because JS objects deduplicate keys before serialization — Go's `EvalFile` receives pre-deduplicated `hookScopes` and cannot detect within-plugin duplicates. In `bridge.js`, the `hook()` method must check if `hooks[name]` already exists before overwriting. If so, push a warning string (e.g., `"duplicate hook registration: <name> — last registration wins"`) to a module-level `warnings` array. The eval response must include `warnings: warnings` alongside `filters`, `shortcodes`, `hooks`, and `hookScopes`. In `node.go`, add `evalWarnings []string` field to `NodeRuntime` and `EvalWarnings() []string` accessor. `EvalFile` parses the `warnings` array from the eval response and appends each string to `r.evalWarnings`. Last-wins semantics are preserved — the warning is informational, not an error. **Defensive hooks dedup (issue #555)**: `r.hooks` must not contain within-plugin duplicates. bridge.js already prevents this via `Object.keys(hooks)` (JS objects have unique keys), but Go must defensively deduplicate as a protocol invariant guard. `RegisteredHookDetails()` must deduplicate by hook name when building the output slice (use a seen-set; skip names already emitted). This ensures one `HookRegistration` per unique hook name regardless of how duplicates enter `r.hooks`. Cross-plugin duplicates (different plugins registering the same hook) are intentional and handled by `HookRegistry` — this dedup applies only within a single runtime's accumulated hooks.
- **Plugin filter bridging (issue #93)**: After `registry.LoadPlugins(hooks)` (step 0) and engine creation (step 10), bridge plugin-discovered filters to the template engine. For each filter name from `LoadPlugins()`, call `engine.AddFilter(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallFilter()`. This must happen before content rendering (step 11) so templates can use plugin filters. Similarly, `alloy.hook()`/`alloy.on()` registrations discovered by `EvalFile()` must be wired into the `HookRegistry` during `LoadPlugins()`.
- **CallFilter must pass args to JS (issue #318)**: `QuickJSRuntime.CallFilter` currently sets `__callInput` from `input` but ignores `args`. Fix: serialize `args` as a JS array global (`__callArgs`), then invoke `__filters[__callFilterName](__callInput, ...__callArgs)` using spread syntax. Each arg must be type-switched (string/int/float64/bool/map/slice) and converted to a QuickJS value. Maps and slices should be JSON-serialized and parsed in JS.
- **Plugin site data access (issue #317, wiring #339)**: `QuickJSRuntime` already implements `SetSiteData(data map[string]interface{})` which JSON-serializes `data` and sets it as `alloy.data` in the JS context. **Pipeline wiring (issue #339)**: After `loadSiteData(cfg)` AND external source fetching complete, inject `siteData` into each runtime. Either add `SetSiteData` to the `Runtime` interface, or use a type assertion (`if sdr, ok := rt.(SiteDataReceiver); ok { sdr.SetSiteData(siteData) }`). This must happen before content rendering (step 11) and after ALL data sources (local + external) are merged. Without this call, `alloy.data` is `undefined` in plugins. For Node (Tier 3), send site data as a `{type: "siteData", payload: data}` message via the bridge. `alloy.data` is read-only — changes in JS do not propagate back to Go.
- **`{% inline %}` tag (issue #288, wiring #295)**: `createEngine()` in `build.go` must call `tmpl.RegisterInlineTag(engine)` after `RegisterBuiltinFilters(engine)`. Without this, `{% inline %}` works in unit tests but fails with "unknown tag" in actual builds. Register `inline` as a custom tag via `engine.AddTag("inline", inlineTagFunc)`. The tag function:
  1. Extracts the path argument (first positional arg, string)
  2. Validates path starts with `./` or `../` — bare relative paths (e.g., `diagram.svg`) and absolute paths are both rejected
  3. Checks file extension against an allowlist (`.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`) — error with guidance for binary types. Extension matching is case-insensitive via `strings.ToLower` (issue #931).
  4. Resolves the path relative to the current content file's directory (passed via render context as `_contentDir`)
  5. **Sandboxes** the resolved path: `filepath.Rel(contentRoot, resolved)` must not start with `..`. Rejects paths that escape the content directory (e.g., `../../../../etc/passwd`). The content root is passed via `_contentRoot` in the render context. **Fail-closed (issue #932):** If `_contentRoot` is empty or missing from the render context, return an error (`"inline tag requires _contentRoot in render context"`) rather than silently skipping the sandbox check. This matches the existing `_contentDir` check. The `if contentRoot != ""` guard must become a hard error.
  6. Reads the file and returns raw contents (no template processing)
  
  The template context must include both keys before rendering each page: `ctx["_contentDir"] = filepath.Dir(filepath.Join(contentDir, page.RelPath))` and `ctx["_contentRoot"] = contentDir`. These are internal context keys (prefixed with `_`) — accessible in templates but unsupported/unstable.
- **Plugin shortcode bridging (issue #139)**: Same pattern as filter bridging. After engine creation, iterate `rt.RegisteredShortcodes()` and call `engine.AddTag(name, wrapperFn)` where `wrapperFn` routes through `QuickJSRuntime.CallShortcode(name, args, innerContent)`. Both inline and block shortcodes must be supported. `CallShortcode()` is currently a stub (returns input unchanged) and must be implemented to actually invoke the JS shortcode function.
- **Shortcode calling convention (issue #981)**: Two changes to `internal/template/liquid.go`, both implemented:
  1. **Variable argument resolution**: Three functions implement the `parseTagTokens → resolveTagArgs → resolveVariable` pipeline:
     - `parseTagTokens(markup string) []parsedArg` — tokenizes tag markup into `[]parsedArg` where each element carries `value string, quoted bool`. Both single and double quotes are recognized. Supports mixed forms (e.g., `"primary" page.size`).
     - `resolveTagArgs(tokens []parsedArg, context liquid.TagContext) []string` — iterates tokens: quoted args use the literal value; unquoted args call `resolveVariable`. If `resolveVariable` returns `(nil, false)`, the literal token string is used as fallback. Non-string resolved values are converted via `fmt.Sprint()`. The resulting `[]string` is passed to `t.fn(args, content)` as before. The `TagFunc` signature is unchanged.
     - `resolveVariable(name string, context liquid.TagContext) (interface{}, bool)` — parses the variable name (possibly dotted, e.g., `page.videoId`) via `liquid.VariableLookupParse(name, nil, nil)` into a `*VariableLookup`, then calls `context.Evaluate(vl)` on the parsed lookup. `TagContext.Evaluate` expects evaluable objects (e.g., `*VariableLookup`), not plain strings — passing a raw string returns the string unchanged, bypassing variable resolution. Returns `(nil, false)` when `Evaluate` returns nil.
     - `parseTagArgs(markup string) []string` — preserved as a thin wrapper over `parseTagTokens` for callers that don't need variable resolution (e.g., `{% inline %}`).
     Go engine is unaffected — Go's `html/template` resolves expressions before calling FuncMap functions.
  2. **Empty return placeholder removal**: In both `alloyInlineTag.Render` and `alloyBlockTag.Render`, the `if result == ""` branch that emitted `<alloy-shortcode data-tag="%s"></alloy-shortcode>` is removed. When `t.fn` returns `""`, the `Render` method returns `""` — no placeholder, no wrapper element. This achieves cross-engine parity with Go template shortcodes.
- **Go template block shortcode preprocessor (issue #1002)**: `ProcessBlockShortcodes(content []byte, callShortcode func(name string, args []string, innerContent string) (string, error)) ([]byte, error)` in `internal/template/gotemplate_block_shortcode.go`. This function scans post-Goldmark HTML for `{{% tag "args" %}}...{{% /tag %}}` pairs and replaces each with the shortcode callback's output. Implementation details:
  - **Pattern matching**: Use regex-based scanning to find `{{% ... %}}` delimiters. Opening tags match `{{% tagname "arg1" "arg2" %}}` — extract the tag name and quoted arguments. Closing tags match `{{% /tagname %}}` — extract the tag name for pairing. The scanner must skip content inside `<pre>` and `<code>` elements — `{{% %}}` delimiters in code blocks are literal text, not shortcode invocations. The existing `escapeTemplateTagsInCode` step handles `{{ }}` and `{% %}` but not `{{% %}}`, so the preprocessor itself must be code-block-aware.
  - **Nesting**: Resolved innermost-first via iterative approach: find the first (leftmost) closing tag not in a code region, then find the nearest opening tag before it (also not in a code region). If names don't match, return an error. Replace the matched pair with the callback's output and repeat. This naturally resolves innermost-to-outermost and rejects interleaved mismatches (e.g., `{{% a %}}{{% b %}}{{% /a %}}{{% /b %}}` errors because the nearest opening tag before `{{% /a %}}` is `{{% b %}}`, not `{{% a %}}`).
  - **Escaped quotes in arguments (issue #1008)**: The argument regex must support backslash-escaped characters within quoted arguments. Change `blockShortcodeArgRe` from `"([^"]*)"` to `"((?:[^"\\]|\\.)*)"` — the outer capture group is required because `parseShortcodeArgs` reads extracted content via `m[1]`. The `parseShortcodeArgs` function must unescape the extracted argument values: replace `\"` with `"` and `\\` with `\`. The opening tag regex `blockShortcodeOpenRe` must also be updated to use the escaped-aware quoted string pattern in its argument capture group: change `(?:\s+"[^"]*")*` to `(?:\s+"(?:[^"\\]|\\.)*")*` (non-capturing here is correct — `blockShortcodeOpenRe` captures the entire args substring, not individual args). Existing unescaped arguments are unaffected — the new regex is a superset of the old one.
  - **Depth guard**: The outer loop uses a `maxShortcodeIterations` constant (100) to cap total iterations. If a callback's output contains new `{{% %}}` pairs, they are processed in subsequent iterations. If the iteration limit is reached, return an error referencing the limit. This prevents infinite loops from self-referential shortcode output.
  - **Error handling**: Return an error (with the tag name) if an opening tag has no matching closing tag (unclosed tag), if a closing tag has no matching opening tag (unexpected closing tag), or if the nearest opening tag name doesn't match the closing tag name (mismatched tags). When the opening tag is inside a `<code>` element but the closing tag is outside, the opening tag is treated as literal text and the closing tag triggers an "unexpected closing tag" error. Propagate callback errors as build errors.
  - **Passthrough**: Only process `{{% %}}` delimiters. Liquid-style `{% %}` tags and Go template `{{ }}` expressions must pass through unchanged. Content without `{{% %}}` tags returns unchanged.
  - **Pipeline integration (issue #1011)**: In `renderPages()` (`internal/pipeline/render.go`), when the configured engine is `"gotemplate"`, call `ProcessBlockShortcodes` on the post-Goldmark HTML before passing it to `rc.Engine.Parse()`. This step runs after `escapeTemplateTagsInCode` and before `hasTemplateSyntax`/template rendering. The callback must route through the plugin registry's `CallShortcode` — the developer must add a mechanism for `renderPages` to access shortcode resolution (e.g., a `BlockShortcodeCallback` field on `RenderContext`, or passing the registry). The callback function signature must match `BlockShortcodeCallback`: for each `(name, args, content)` invocation, call `runtime.CallShortcode(name, args, content)` for the appropriate runtime. Errors from `ProcessBlockShortcodes` must propagate as build errors with context (e.g., `"block shortcode processing: %s: %w", page.RelPath, err`). Without this wiring, `ProcessBlockShortcodes` is dead code and users get a Go template parse error (`unexpected "%" in command`) because `{{% %}}` tags reach the Go template parser unprocessed.
- **Plugin filters in `{% include %}` partials (issue #376)**: Plugin-registered filters fail with "undefined filter" when used inside `{% include %}` partials. Root cause: alloy's `Parse()` method rewrites novel filter names (e.g., `{{ x | tokenType }}` → `{{ x | plugin_filter: "tokenType" }}`) before liquidgo parses the template. But when liquidgo encounters `{% include %}` at render time, it calls `alloyFileSystem.ReadTemplateFile()` to read the partial source, then parses it internally — bypassing alloy's `rewriteFilterToPlugin` step. The partial source is parsed as-is, so plugin filter names are unknown to liquidgo. **Fix**: `alloyFileSystem` must have access to the `dynamicFilters` map. `ReadTemplateFile` must apply `rewriteFilterToPlugin` for each dynamic filter before returning the source string. This ensures partials go through the same pre-processing as content templates and layouts.
- **Plugin filter shadowing (issue #140)**: When a plugin registers a filter with the same name as a built-in liquidgo filter (e.g., `reverse`), the plugin's version must take precedence. Per spec §4: "the last one loaded wins." The current implementation fails because `knownLiquidFilters` prevents plugin filters from being treated as dynamic filters, so liquidgo's native implementation intercepts the call. The fix must ensure plugin-registered filters override built-in filters in the template engine's dispatch chain.
- **Hook payload contract (issue #182)**: All hook payloads must be JSON-serializable for JS/WASM plugins. Seven categories:
  - **`onContentTransformed`** (issue #448): fires once per page with a typed `HookTransformPayload` struct: `HookTransformPayload{HTML: string(page.RenderedBody), TOC: convertTOCEntries(page.TOC), Path: page.RelPath, URL: page.URL, FrontMatter: convertOrderedMaps(page.FrontMatter)}` where `convertTOCEntries` copies `[]content.TOCEntry` → `[]plugin.TOCEntry` (shallow field copy, recursive for children). The returned object (inbound as `map[string]interface{}`) is applied back: `html` → `page.RenderedBody`, `toc` → `page.TOC`, `frontMatter` → `page.FrontMatter`. This allows plugins to build TOC for non-markdown pages, modify front matter post-render, or transform HTML. `fireContentTransformedHooks` must build a typed `HookTransformPayload`, not a `map[string]interface{}`. **Performance (issue #463)**: Before building the payload, recursively convert any `*ordered.Map` values in `page.FrontMatter` to plain `map[string]interface{}` via `ToGoMap()`. `ordered.Map.MarshalJSON` is expensive (13.7% of CPU in profiling). The conversion is read-only — `page.FrontMatter` is not mutated, only the serialization copy is converted. Templates still see the original `*ordered.Map` values after hooks complete. **Performance (issue #529)**: Use `sonic.ConfigStd` for JSON serialization instead of `encoding/json`. Typed payload structs allow sonic's JIT compiler to generate optimized encoders for the fixed fields (`Path`, `URL`, `HTML`, `TOC`) — only `FrontMatter` (user-defined keys) requires reflection. See §Typed outbound payload structs below.
  - **`onPageRendered`** (issue #1095): fires once per page with a typed `HookRenderedPayload` struct: `HookRenderedPayload{HTML: page.HTML(), FrontMatter: convertOrderedMaps(page.FrontMatter), URL: page.URL, Path: page.RelPath}`. The developer must create a `HookRenderedPayload` struct in `internal/plugin/payload.go` with JSON tags `html`, `frontMatter`, `url`, `path`. In `Build()`, replace the current `payloads[i] = page.HTML()` with `payloads[i] = plugin.HookRenderedPayload{HTML: page.HTML(), FrontMatter: convertOrderedMaps(page.FrontMatter), URL: page.URL, Path: page.RelPath}`. The apply-back block must change from the current `case string` / `case []byte` switch to extracting `html` from the returned map via `toGoMap(result)`: if the result is a map with an `"html"` key (string), call `pages[i].SetRenderedBody([]byte(html))`. `frontMatter`, `url`, and `path` in the return are ignored — they are read-only context for conditional processing. This is a breaking change to the callback signature: plugins must update from `(html) => html.replace(...)` to `(page) => { page.html = page.html.replace(...); return page; }`. **Edge-case handling (issue #1120):** `extractPageRenderedHTML` must handle four edge cases gracefully: (1) nil/undefined result → no-op, original HTML preserved, no warning; (2) map without `html` key → original preserved, migration warning logged; (3) map with non-string `html` → original preserved, migration warning logged; (4) raw string return (old API) → original preserved, migration warning logged. `buildPageRenderedPayload` must call `convertOrderedMaps(page.FrontMatter)` to recursively flatten `*ordered.Map` values to `map[string]interface{}` at all nesting levels. Both `Build()` and `BuildIncremental()` must produce identical payload shapes with all four fields correctly populated — including updated front matter values on incremental rebuilds.
  - **Pre-taxonomy hook** (`onPagesReady`, issues #525, #971): fires once per language batch inside `applyBatchContext()`, after cascade data application but before `BuildTaxonomies()`. Payload: typed `HookPagesReadyPayload{Pages: serialized, SiteData: siteData}` where `serialized` is nil when the union scope's `Pages.Mode` is `PagesScopeNone` (all hooks declared `pages: false`), or `[]HookPagePayload` otherwise. Each serialized page is `HookPagePayload{Path: page.RelPath, URL: page.URL, FrontMatter: convertOrderedMaps(page.FrontMatter), Content: string(page.Body)}`. No `HTML` field — content has not been rendered yet. **Return value dispatch (issue #971)**: inbound as `map[string]interface{}`. Check for `addPages` and `pages` keys — both present is an error, neither present is an error (unrecognized shape), non-map result is a no-op. **`addPages` path**: type-assert as `[]interface{}`, iterate entries. For each entry: `toGoMap` → `virtualPageFromMap` → URL collision check → append to `pages`. **`pages` path**: existing behavior — pages at indices `>= len(originalPages)` are virtual pages, constructed via `virtualPageFromMap`. Validate: `path` and `url` required (error if missing). Check for output-path collisions with existing pages (error on collision). Appended virtual pages are added to the `pages` slice **before** `BuildTaxonomies()` and `buildCollectionsContext()` — so they participate in taxonomy collection and appear in `taxonomies.*` in templates. Virtual pages with raw `content` flow through the content rendering pipeline (markdown → HTML). Per-batch firing means each invocation only sees pages from one language batch, avoiding the language routing problem in #521.
  - **Content hooks** (`onContentLoaded`, `onDataCascadeReady`): `onContentLoaded` fires once with the full pages array as `[]HookPagePayload` (typed structs with fields `Path`, `URL`, `FrontMatter`, `Content`, `HTML`). Return value is inbound as `[]interface{}` of `map[string]interface{}` — applied back: existing pages at indices `0..len(pages)-1` have their `frontMatter` merged back to `page.FrontMatter` and `html` written back to `page.RenderedBody`. Only `frontMatter` and `html` are applied — `content`, `path`, and `url` in the return are informational (present in the payload for plugin inspection, but mutations to those fields are not written back to the page). **html merge-back (issue #976)**: The current apply-back block only reads `pageMap["frontMatter"]` — `html` mutations are silently dropped. The fix: restructure the merge-back loop so `returnedPath` extraction and scope filtering happen outside the `frontMatter` block, then after merging `frontMatter`, check for `pageMap["html"]` as a string and call `pages[origIdx].SetRenderedBody([]byte(html))` — same pattern as `onContentTransformed` (the per-page html merge-back in `fireContentTransformedHooks`). This runs before layout rendering, so the mutated html flows into the template engine as `{{ content }}`. **Virtual page injection is not supported** — if the returned array length exceeds the input length, produce a validation error. Virtual page injection belongs exclusively in `onPagesReady` (#525). This also resolves #521 (virtual pages appended to wrong language batch) — since `onContentLoaded` no longer accepts virtual pages, the batch routing problem is eliminated. `onDataCascadeReady` fires once with the full pages array as `[]HookCascadePayload` — each entry is `HookCascadePayload{Path: page.RelPath, Data: page.FrontMatter}` (cascade is already merged into FrontMatter at this pipeline stage). Return value is inbound as `[]interface{}` of `map[string]interface{}` — cascade data applied back per page. No virtual page support. Apply-back pattern mirrors the `onContentLoaded` apply-back block in `build.go`: type-assert return as `[]interface{}`, validate length matches input (reject injection and removal with specific error messages referencing `onPagesReady`), build `pathToIdx` map from `pages`, iterate returned entries, extract `data` field via `toGoMap`, merge each key back into `pages[origIdx].FrontMatter`. The serialization must use `[]HookCascadePayload` instead of `serializePagesForHook` — build the slice directly where each entry has `Path: page.RelPath` and `Data: convertOrderedMaps(page.FrontMatter)`.
  - **Per-asset hook** (`onAssetProcess`, issue #974): fire once per asset with `HookAssetPayload{Path: relPath, Content: fileContent}`. Return value's `"content"` key replaces the asset content written to the output directory. The `path` field must be relative within the assets directory (e.g. `css/main.css`), not a full filesystem path. The pipeline must replace the current `CopyAssets` + single directory-level hook fire with per-asset dispatch using `ProcessAssets` from `internal/assets/assets.go`. The hook must not fire when the assets directory does not exist or contains no files. Hook errors must propagate as build errors with a message containing `"onAssetProcess"`. Edge cases: if the hook returns `nil` (null/undefined from JS), preserve the original content. If the return is a map but has no `"content"` key, preserve the original content. The `"path"` key in the return value is ignored — only `"content"` is applied back. Both `CopyAssets` call sites in `Build()` must be updated: the main asset copy path and the zero-pages early-return path. The `ProcessAssets` function and `HookAssetPayload` struct already exist — the change is in `Build()` wiring only.
  - **Per-build hooks** (`onConfig`, `onBeforeValidation`, `onAfterValidation`, `onDataFetched`): convert Go structs to `map[string]interface{}` before passing to `CallHook`. Deserialize returned map and apply changes back. Requires enhancing the QuickJS/WASM hook bridge so structured Go values are marshaled as real JSON objects (not stringified) — encode the map to JSON, parse in JS, then deserialize returned JS objects back into Go maps.
  - **onBeforeValidation payload and return contract (issue #975)**: The current `Build()` code fires `onBeforeValidation` too early (before content discovery) with a trivial one-entry map (`{"<output-dir>": "build output"}`) and discards the return value. The developer must:
    1. **Move the hook call** to fire immediately before `DetectConflicts()` — after output path computation, after `onPagesReady`, after taxonomy collection and pagination. At this point, `outputEntries` (type `[]validation.OutputPathEntry`) contains all computed output paths.
    2. **Build the payload**: Extract the `Path` field from each `outputEntries` element into a string array. Payload shape: `map[string]interface{}{"outputPaths": pathArray}`.
    3. **Process the return**: Use `RunEachWithTimeout` (not `RunWithTimeout`) so each registered hook gets the same snapshot and can add its own outputs independently. Result handler: type-assert result to `map[string]interface{}` via `toGoMap`. First, validate all keys against the allowlist (`addOutputs` is the only recognized key) — any unrecognized key produces an error `"onBeforeValidation returned unrecognized key \"<key>\" — only \"addOutputs\" is recognized"`, even if `addOutputs` is also present. Then, if `addOutputs` is present, type-assert as `map[string]interface{}` (error if not a map: `"addOutputs must be a map, got %T"`). Iterate entries: each key is an output file path (string), each value is a source label (coerce to string via `fmt.Sprintf`). Append `validation.OutputPathEntry{Path: key, Source: sourceLabel}` to `outputEntries`. No-op (nil/null return or empty map) is allowed. Remove the old call site that fires before content discovery.
    4. **Known return keys**: `addOutputs` (the only recognized key). This is additive-only — plugins cannot remove or modify existing output paths. Unrecognized keys produce an error even alongside `addOutputs`.
    5. **Test coverage**: 8 spec tests in `internal/pipeline/hooks_test.go` covering: correct payload shape, addOutputs feeding conflict detection, addOutputs with no conflict, non-map addOutputs rejection, unrecognized return keys (without addOutputs), unrecognized keys alongside addOutputs, no-op return, and conflict error message includes plugin source label.
  - **onAfterValidation payload and return contract (issue #975)**: The current `Build()` code fires `onAfterValidation` too early (before content discovery) with the same trivial one-entry map and discards the return value. The developer must:
    1. **Move the hook call** to fire immediately after conflict detection passes (after `DetectConflicts()` returns with no conflicts). At this point, `outputEntries` contains all validated output paths and `siteData` contains the assembled site data.
    2. **Build the payload**: Extract `Path` from each `outputEntries` into a string array. Build a cascade map from `siteData`. Payload shape: `map[string]interface{}{"outputPaths": pathArray, "cascade": siteData}`.
    3. **Process the return**: Use `RunEachWithTimeout` so each hook receives the current cascade state (accumulating previous plugins' changes). Result handler: type-assert result to `map[string]interface{}` via `toGoMap`. First, validate all keys against the allowlist (`cascade` and `outputPaths` are the only recognized keys) — any unrecognized key produces an error `"onAfterValidation returned unrecognized key \"<key>\" — only \"cascade\" and \"outputPaths\" are recognized"`, even if `cascade` is also present. Then, if the result has a `cascade` key, type-assert it as `map[string]interface{}` (error if not a map: `"onAfterValidation cascade must be a map, got %T"`). Merge each key from the returned cascade into `siteData`. Ignore `outputPaths` key in the return (post-validation, no path changes). No-op (nil/null return, empty map, or return with only `outputPaths`) is allowed.
    4. **Known return keys**: `cascade` (mutable, merged into siteData), `outputPaths` (ignored in return — read-only). Unrecognized keys produce an error even alongside recognized keys.
    5. **Test coverage**: 8 spec tests in `internal/pipeline/hooks_test.go` covering: correct payload shape with outputPaths and cascade, cascade mutation applied to site data (verified via template rendering), outputPaths changes ignored in return, observation-only plugin (no return), multiple plugins' cascade mutations merged (cross-plugin), cascade returned as non-map rejection, unrecognized keys alongside cascade, and outputPaths includes plugin-added paths from onBeforeValidation.
    6. **Remove the old call site** that fires before content discovery. Remove the now-unused `outputPathMap` variable.
  - **onConfig merge-back (issue #973)**: The `Build()` call site currently discards the `onConfig` return value (`_, err := hooks.RunWithTimeout(plugin.OnConfig, cfg)`). The developer must:
    1. Capture the return value: `result, err := hooks.RunWithTimeout(plugin.OnConfig, cfg)`.
    2. Create a function `applyOnConfigResult(cfg *config.Config, result interface{}) error` in `internal/pipeline/build.go` (or a new `config_apply.go` in the same package). This function:
       - Type-asserts `result` to `map[string]interface{}`. If the assertion fails, returns an error: `"onConfig hook must return an object, got %T"`.
       - Extracts mutable fields from the returned map and writes them to `cfg`. The **mutable allowlist**: `build.output` (string), `build.clean` (bool → `*bool`), `structure.content` (string), `structure.layouts` (string), `structure.assets` (string), `structure.static` (string), `structure.data` (string), `passthrough` (deserialize `[]interface{}` → `[]config.PassthroughMapping`), `plugins.workers` (preserve `interface{}` — `"auto"` or int), `plugins.timeout` (int → `int`, noting JSON numbers arrive as `float64`).
       - **Path-containment validation (issue #998)**: Before writing each directory path string field (`build.output`, `structure.content`, `structure.layouts`, `structure.assets`, `structure.static`, `structure.data`) to `cfg`, validate the value against path traversal. Create a helper function `validateOnConfigPath(field, value string) (string, error)` that: (1) normalizes the value via `filepath.Clean` (note: `filepath.Clean("")` returns `"."`), (2) rejects `"."` (current directory — would target the project root itself; with `clean: true`, `CleanOutputDir` deletes all source files), (3) rejects absolute paths via `filepath.IsAbs` with error `"onConfig: <field>: absolute path <value> is not allowed"`, (4) rejects paths that start with `".."` (i.e., `cleaned == ".."` or `strings.HasPrefix(cleaned, ".."+string(filepath.Separator))`) with error `"onConfig: <field>: path <value> traverses above the project root"`, (5) returns the cleaned value on success. The cleaned value (not the raw plugin value) is written to `cfg`. This ensures no downstream code (`resolveDir`, `validateOutputDir`, `CleanOutputDir`, `CopyPassthrough`) ever sees an unsafe or denormalized path. Non-path fields (`build.clean`, `plugins.workers`, `plugins.timeout`) are not validated.
       - **Passthrough path validation (issues #1031, #1034)**: Extend path-containment validation to passthrough `from` and `to` fields. During the passthrough deserialization loop in `applyOnConfigResult`, the existing empty-`from` filter (`if mapping.From == "" { continue }`) runs first — entries with empty `from` are silently skipped before validation. For non-empty `from` values, validate with `validateOnConfigPath(fmt.Sprintf("passthrough[%d].from", i), from)` — same rules as `build.output` (reject absolute, traversal, and `.`). For `to`, first clean the value via `filepath.Clean`, then check if the cleaned result is `"."` — if so, accept it as a valid output-root destination and store the cleaned value. Only run `validateOnConfigPath` when the cleaned value is not `"."`. This handles equivalent forms: `to: "."`, `to: "./"`, and `to: "subdir/.."` all clean to `"."` and are accepted. `to: ""` is also valid (empty string bypasses validation entirely — the default meaning is "root of output directory"). Unlike `build.output` where `"."` targets the project root for `CleanOutputDir` deletion, passthrough `to: "."` means "root of the output directory". When the cleaned value is not `"."`, validate with `validateOnConfigPath(fmt.Sprintf("passthrough[%d].to", i), to)`. All passthrough path validations must pass before the passthrough slice is written to `cfg.Passthrough` — collect validated mappings first, then apply only after the full loop succeeds. Passthrough validation must also complete before any structure/build fields are applied, to maintain the existing atomicity guarantee (no partial config mutation on failure). Test coverage: 19 spec tests in `internal/pipeline/config_apply_test.go`.
       - For each mutable field, only updates `cfg` if the key is present in the returned map (partial returns preserve original values).
       - Fields outside the allowlist (`title`, `baseURL`, `language`, `templates`, `content`, `data`, `taxonomies`, `pagination`, `sitemap`, `collections`, `watch`, `sources`, `ssr`, `languages`, `structure.plugins`, `structure.components`) are not applied. Changes to these fields may log a warning but must not modify `cfg`.
       - After applying mutable fields, if `plugins.timeout` changed, update the hook registry timeout via `hooks.SetTimeout(cfg.Plugins.Timeout)` so subsequent hooks use the new value.
    3. Call `applyOnConfigResult` after `RunWithTimeout` in `Build()`, before validation and output-dir resolution.
    4. Update the code comment above the call site to remove the inaccurate claim about what fields can be mutated and reference the mutable allowlist in PLAN.md instead.
    5. **Test coverage (issue #999)**: Spec tests in `internal/pipeline/hooks_test.go` cover all mutable allowlist fields: `build.output` (PR #997), `build.clean`, `structure.content` (PR #997), `structure.layouts` (PR #997), `structure.assets`, `structure.static`, `structure.data`, `passthrough` (including empty-`from` filtering), `plugins.workers` (int and string `"auto"`), `plugins.timeout` (including `SetTimeout` call verification). Edge case tests: null/nil return error, hook timeout preserves original config, zero/negative timeout values preserve original timeout, title exclusion from mutable allowlist (PR #997), chaining across plugins (PR #997), non-object return error (PR #997).
  - **Read-only hooks** (`onBuildComplete`, `onDevServerStart`, `onFileChanged`): serialize to JSON for observation, return value ignored. The runtime bridge must still preserve object payloads at the JS boundary for observation hooks. **`onBuildComplete` payload trimming (issue #1098)**: The `onBuildComplete` hook must NOT receive the full `*BuildResult` as its payload. The current code passes `result` directly to `RunWithTimeout`, which serializes the entire struct — including `RenderedContent` (megabytes of HTML), `Cache` (internal state), and `SiteData` — to every plugin. The developer must construct a trimmed payload matching the documented shape from PLAN.md §5: `map[string]interface{}{"pageCount": result.PageCount, "duration": result.Duration.String(), "errors": errorStrings, "outputDir": result.OutputDir}`. `result.Errors` is `[]error` — Go's `error` interface serializes to `{}` via `json.Marshal` since it has no exported fields. The developer must convert to `[]string` by calling `.Error()` on each element: `errorStrings := make([]string, len(result.Errors)); for i, e := range result.Errors { errorStrings[i] = e.Error() }`. When `result.Errors` is nil, use an empty `[]string{}` so plugins receive `[]` not `null`. `outputDir` is a cheap string field (`cfg.Build.Output`) that plugins need to write post-build artifacts (e.g., `search-index.json` via `path.join(result.outputDir, ...)`); it was accidentally omitted in the initial trimming (issue #1110). Apply this trimming in both `Build()` (at the `RunWithTimeout(plugin.OnBuildComplete, ...)` call site) and `BuildIncremental()` (at its `RunWithTimeout(plugin.OnBuildComplete, ...)` call site).
- **Typed outbound payload structs (issue #529)**: The outbound serialization path (Go → Plugin) must use typed structs with json tags instead of `map[string]interface{}`. This separates the plugin bridge serialization path from the template rendering path (which uses `map[string]interface{}` for liquidgo). Typed structs allow `sonic.ConfigStd` to JIT-compile optimized encoders for fixed fields — only `FrontMatter` and `SiteData` (user-defined keys) require reflection. The inbound path (Plugin → Go) remains `map[string]interface{}` since plugin return shapes are dynamic. Structs are defined in `internal/plugin/payload.go`:
  - `HookPagePayload` — `Path string`, `URL string`, `FrontMatter map[string]interface{}`, `Content string` (omitempty), `HTML string` (omitempty). Used by `serializePagesForHook` (replaces `[]interface{}` return) and `serializePagesForPagesReady`. `FrontMatter` must NOT use `omitempty` — serialization path must coerce nil to `map[string]interface{}{}` before building the struct to avoid `null` in JSON (which causes `TypeError` on JS property access).
  - `HookTransformPayload` — `Path string`, `URL string`, `FrontMatter map[string]interface{}`, `HTML string`, `TOC []TOCEntry` (omitempty). Used by `fireContentTransformedHooks`.
  - `TOCEntry` — **consolidated (issue #592)**: `content.TOCEntry` now carries JSON tags (`json:"id"`, `json:"text"`, `json:"level"`, `json:"children,omitempty"`), so `plugin.TOCEntry` is removed. `HookTransformPayload.TOC` changes type from `[]plugin.TOCEntry` to `[]content.TOCEntry`. The `contentTOCToPlugin()` conversion function in `build.go` is removed — direct assignment replaces it. The empty-slice `children` field is omitted via `omitempty` (verified by spec test using non-nil empty slice `[]content.TOCEntry{}` to discriminate omitempty from nil-omission).
  - `HookPagesReadyPayload` — `Pages []HookPagePayload`, `SiteData map[string]interface{}`. Used by `onPagesReady` serialization.
  - `HookCascadePayload` — `Path string`, `Data map[string]interface{}`. Used by `onDataCascadeReady`.
  - `HookAssetPayload` — `Path string`, `Content string`. Used by `onAssetProcess`.
  - `HookBuildCompletePayload` — `PageCount int`, `Duration string`, `Errors []string`, `OutputDir string`. Used by `buildCompletePayload()` in `build.go`. The `OutputDir` field is `cfg.Build.Output` passed through as-is (not resolved to absolute) — a cheap string that plugins need to write post-build artifacts (issue #1110). Must be set from `r.OutputDir` in the `buildCompletePayload` helper.
  - **Bridge type switch (issue #529)**: The QuickJS `CallHook` at `wasm.go:323` uses a type switch with explicit cases for `map[string]interface{}` and `[]interface{}`. Typed payload structs hit the `default` case which calls `fmt.Sprint(v)` — producing a Go struct literal string, not JSON. The `default` case must be changed to `json.Marshal(v)` → parse in JS, matching the `map[string]interface{}` path. This applies to both the QuickJS bridge and the Node bridge's `EncodeMessage` at `node.go:56` (which already uses `json.Marshal` and handles structs correctly).
- **JSON library replacement (issue #529)**: Replace `encoding/json` with `sonic.ConfigStd` in `internal/plugin`. Add `var json = sonic.ConfigStd` package-level alias — all existing `json.Marshal`/`json.Unmarshal` calls work unchanged. sonic v1.15.1 (Apache 2.0) auto-falls back to `encoding/json` on non-amd64/arm64 architectures via build constraints. `sonic.ConfigStd` matches `encoding/json` behavior: HTML escaping enabled, map keys sorted, strings copied on decode. One known difference: backspace encoded as `\b` vs `` — both valid JSON, no behavioral impact. Add `github.com/bytedance/sonic` to `go.mod`.
- **WASM runtime (issue #181)**: `WASMRuntime.LoadModule()` uses wazero to compile and instantiate the WASM binary, discover exported functions (`alloc`, `filter`, `shortcode`, `hook`, `hooks`), and register them. `alloc` export is required — used by the host to get a safe write offset in WASM linear memory (issue #186). `CallExport(name, args...)` must call `alloc(inputLen)`, write input bytes at the returned pointer, call the export with `(ptr, len)`, and read the result from the returned `(resultPtr, resultLen)` (issue #190). `RegisteredFilters()` must return filter names from the module's exports. `LoadModule` must return an error for invalid WASM binaries. See PLAN.md §5 WASM Calling Convention for full ABI.
- **WASM hook support (issue #444)**: `WASMRuntime` must support hook registration and execution via the `hooks()`/`hook()` export ABI:
  - **Hook discovery**: `LoadModule` must check for a `hooks` export after module instantiation. If present, call it (no arguments — zero-arg function returning `(ptr i32, len i32)`, same pattern as `last_error`), read the returned bytes as a JSON array. Elements can be plain strings (backward compat) or objects with metadata. Store results in `r.hookRegistrations []HookRegistration` alongside `r.hookNames []string` (derived from registrations for `RegisteredHooks()` compat). If no `hooks` export, both fields remain nil. Add `hooks` to `wasmRuntimeExports` (note: `hook` is already reserved but `hooks` is not — add it).
  - **`RegisteredHooks()`**: Return `r.hookNames` instead of nil.
  - **`CallHook(name string, payload interface{}) (interface{}, error)`**: New method. JSON-marshal the payload: if payload is `map[string]interface{}`, merge `{"event": name}` into the map; otherwise wrap as `{"event": name, "payload": payload}`. Call the `hook` export via the alloc/write/call/read ABI (same as `callStringFilter`). Parse the returned JSON. Return the result.
  - **Hook error handling**: Three error paths:
    1. **`hook()` returns `(0, 0)`**: Same convention as `filter`/`shortcode`. `CallHook` must check `last_error()` if available and return an error containing the module's error message. No silent fallback to original payload.
    2. **`hooks()` returns invalid JSON**: If `LoadModule` calls `hooks()` and the returned bytes are not a valid JSON array (parse error, non-array type), `LoadModule` must return an error. Fail loud — do not silently treat the module as hook-less. Object elements missing the required `name` field or with a non-string `name` must also cause an error.
    3. **`hook()` returns malformed JSON response**: If `hook()` returns valid `(ptr, len)` (non-zero) but the bytes are not valid JSON, `CallHook` must return an error. Do not silently return nil or fall back to the original payload.
  - **Priority and scope (issue #742)**: `hooks()` return format extended to accept mixed-type JSON arrays: plain strings (priority 50, nil scope) and objects with `name` (required string), `priority` (optional int, default 50), and scope fields (`pages`, `data`, `pageFields` — parsed via existing `parseScopeMap()`). `discoverHooks()` must unmarshal into `[]interface{}` and type-switch each element. `RegisteredHookDetails()` must return stored registrations with real priority/scope values instead of hardcoding priority 50. Object elements missing the `name` field or with a non-string `name` must cause `LoadModule` to return an error. `WASMRuntime` implements `HookDetailer` so `registerRuntime` takes the `RegisterWithOptions`/`RegisterWithPriority` path based on scope presence. Test fixtures: `wasm-mixed-hooks.wasm`, `wasm-priority-only-hooks.wasm`, `wasm-name-only-hooks.wasm`, `wasm-missing-name-hooks.wasm`, `wasm-bad-name-type-hooks.wasm`, `wasm-scope-pages-false.wasm`, `wasm-scope-taxonomy.wasm`.
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
      // Bridge sources — register Go-level handlers backed by bridge RPC
      if sr, ok := rt.(interface {
          RegisteredSources() []string
          CallSource(string, map[string]interface{}) (interface{}, error)
      }); ok {
          for _, name := range sr.RegisteredSources() {
              fetch.RegisterPluginSource(name, wrapSource(sr.CallSource))
          }
      }
  }
  ```

  The pipeline never knows or cares which tier a plugin is. The `Runtime` interface is the only integration point.

- **Plugin data source wiring (issue #979)**: `alloy.source(name, fn)` is the JS API for registering data source handlers. Tier 3 (Node) only — source plugins need network access, npm packages, and environment variables. The implementation connects three layers: bridge registration, Go-side handler wrapping, and pipeline dispatch.
  - **bridge.js changes**: Add a `sources` map (alongside `filters`, `shortcodes`, `hooks`). Add `alloy.source(name, fn)` method that stores the handler in `sources`. Duplicate source name detection follows the same pattern as hooks — push a warning string to the `warnings` array if `sources[name]` already exists before overwriting. The `case 'eval'` response must include `sources: Object.keys(sources)` alongside the existing `filters`, `shortcodes`, `hooks`, `hookScopes`, `warnings` fields. Add `case 'source'` to the message handler: invoke the registered source handler with `msg.payload` as config argument (the handler may ignore it), await the result, return via `sendMessage({id: msg.id, result})`. Error handling: if the handler throws, return `{id: msg.id, error: e.message}` (same pattern as hook/filter errors).
  - **node.go changes**: Add `sources []string` field to `NodeRuntime`. In `EvalFile`, parse the `"sources"` array from the eval response (same pattern as `"filters"`, `"shortcodes"`, `"hooks"` parsing). Add `RegisteredSources() []string` method returning `r.sources`. Add `CallSource(name string, config map[string]interface{}) (interface{}, error)` method: send `Message{Type: "source", Name: name, Payload: config}` to the bridge, return `resp.Result` (same pattern as `CallFilter`/`CallHook`). When `r.bridge` is nil, return `nil, error` (not a silent passthrough — sources must be fetchable).
  - **Pipeline dispatch in build.go**: In the source-fetch loop (`Build()`, the `switch src.Type` block), add `case "plugin"`: call `fetch.FetchPluginSource(src.Plugin, configMap)` where `configMap` is the source config serialized as `map[string]interface{}`. The existing `fetch.PluginSourceHandler` / `FetchPluginSource` scaffolding handles the dispatch. On error, abort the build with `return nil, fmt.Errorf("source %q (plugin %q): %w", name, src.Plugin, err)` — per PLAN.md §5, source failures abort the build. Do NOT warn and continue (the current `default:` branch behavior). The `configMap` must include the source config fields (`type`, `plugin`, `cache`, `as`) so the bridge-backed handler can forward them to the JS function as an optional argument.
  - **Registration wiring**: After `registry.InitRuntimes()` and `EvalFile()` complete, but before the source-fetch loop runs, bridge discovered sources to `fetch.RegisterPluginSource`. For each source name in `rt.RegisteredSources()`, create a `fetch.PluginSourceHandler` closure that calls `rt.CallSource(name, config)`. This happens during `DiscoverPlugins` → `registerRuntime`, alongside filter/shortcode/hook bridging. Use a `SourceCaller` interface type assertion (same pattern as `HookDetailer` and `EvalWarner`) rather than adding `CallSource` to the `PluginFilterRuntime` interface — source registration is Tier 3 only and does not apply to QuickJS or WASM runtimes.

  **LoadPlugins Node wiring (issue #244)**: Node plugins must be evaluated and bridged the same way as QuickJS plugins. For each Node plugin: (1) evaluate the plugin JS to discover registrations, (2) register discovered filters, (3) bridge discovered hooks to HookRegistry. On evaluation failure, produce a warning and continue loading other plugins — do not abort.

  **NodeRuntime specifics:**
  - `Init()` spawns the Node subprocess, sends all Tier 3 plugin source files for evaluation via JSON-RPC `eval` messages
  - The subprocess reports back which filters/shortcodes/hooks were registered via JSON-RPC `registered` response
  - `CallFilter`/`CallShortcode`/`CallHook` send a JSON-RPC request with the payload, wait for response, return the result
  - Payload serialization: Go `interface{}` → JSON → Node subprocess → JS function → JSON → Go `interface{}`. Same JSON-serializable contract as the hook payload spec (per-page HTML strings, `{path, content}` objects, etc.)
  - Timeout: each call respects `cfg.Plugins.Timeout` (default 5s). Node subprocess crash → error, not silent failure
  - Process lifecycle: spawned once at startup, kept alive for the build duration (or serve session). Killed on shutdown.
  - **Process group isolation (issue #723)**: Node subprocesses must be spawned in their own process group (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`). `Stop()` kills the entire process group (`syscall.Kill(-pgid, syscall.SIGTERM)`) instead of the individual PID, ensuring grandchild processes (e.g., Lit SSR workers) are cleaned up. Without this, orphaned Node workers accumulate across `alloy dev` sessions and cause progressive performance degradation.
  - **PID file for crash recovery (issue #723)**: `Start()` writes the worker PID to `.alloy/workers.pid` (one PID per line). `Stop()` removes the PID from the file. On next `Start()`, any PIDs remaining in the file are stale (from a crashed session) — attempt `syscall.Kill(pid, 0)` to check existence, then `syscall.Kill(pid, syscall.SIGTERM)` to clean up survivors. This handles the case where alloy panics or is killed with SIGKILL before `defer registry.Close()` runs. **Concurrency**: PID file reads and writes must use advisory file locking (`syscall.Flock` with `LOCK_EX`) to prevent races when `PrepareWorkerPool` spawns multiple bridges concurrently.
  - **Post-Stop cleanup (issue #723)**: After `Stop()`, `PID()` must return 0 (not the stale PID of the dead process). `Stop()` must be idempotent — calling it twice must not error.
  - **Stale PID cleanup must kill real processes (issue #726)**: `cleanStalePIDs` must terminate live stale processes, not just remove their entries from the file. Validated by spawning a real process, writing its PID to `workers.pid`, and verifying it is killed on the next `Start()`.
  - **Concurrent PID file integrity (issue #726)**: When `PrepareWorkerPool` starts multiple bridges concurrently against the same project root, all PIDs must appear in `workers.pid` without corruption or lost writes.
  - **Malformed PID file resilience (issue #726)**: `cleanStalePIDs` must handle non-numeric lines, negative values, zero, and empty lines without crashing or preventing `Start()` from succeeding.
  - **Stdout isolation (issue #968)**: The bridge script (`bridge.js`) must patch `process.stdout.write` at startup, before any plugin code runs, to redirect all writes to `process.stderr`. Implementation:
    1. At the top of `bridge.js`, save the real write: `const realStdoutWrite = process.stdout.write.bind(process.stdout);`
    2. Replace: `process.stdout.write = (chunk, encoding, callback) => process.stderr.write(chunk, encoding, callback);` — preserves the `(chunk, encoding, callback)` signature and return value so libraries depending on backpressure semantics keep working
    3. In `sendMessage`, use `realStdoutWrite(frame)` instead of `process.stdout.write(frame)` — the bridge holds the only reference to real stdout
    4. Keep the existing `console.log/warn/info/debug` patches — they provide nicer arg formatting via `args.join(' ')` and are now redundant for safety but harmless
  - **Malformed frame diagnostic (issue #968)**: When `DecodeMessage` or `NodeBridge.Send` encounters bytes that don't start with `Content-Length:`, the error message must: (1) include a bounded snippet (≤ 80 chars) of the offending bytes, and (2) name stdout pollution as the likely cause. In `DecodeMessage`, replace the current `"malformed frame: missing Content-Length header"` with a message like `"plugin bridge protocol error: expected Content-Length header, got \"<snippet>...\" — a plugin or one of its dependencies wrote non-protocol output to stdout"`. Apply the same pattern in `NodeBridge.Send` where it currently returns `"unexpected response: %s"`. No resync or recovery — fail loudly with a diagnostic pointer.
- **Incremental build via cache (issue #105, #225)**: This applies to `BuildIncremental` only (dev mode). `Build()` and `BuildWithContent()` always do full rebuilds — they do not read the cache. **Cache ownership is in-memory, caller-side**: the dev-mode watcher loop in `cmd/dev.go` initializes `previousCache` from the initial `Build()` result's `BuildResult.Cache` and updates it from each `BuildIncremental` result's `BuildResult.Cache` after every rebuild — the cache stays in memory across the dev session, never round-tripping through disk. `BuildIncremental` consumes `previousCache` for skip/invalidation decisions and returns an updated cache via `BuildResult.Cache` containing fresh content hashes for rendered pages. After discovering pages, use `previousCache.ShouldSkipFile(relPath, content)` to skip unchanged pages — no re-parse, no re-render. Template changes override content-hash skipping: if a layout file changed, `previousCache.InvalidatedPages(layoutPath)` returns the affected pages, which must be rebuilt even if their content hash is unchanged. Config changes (`previousCache.IsConfigChanged(currentHash)`) trigger a full rebuild. The `BuildResult.PagesSkipped` field reports the skip count (e.g., "Rebuilt 5 pages, 27 skipped (cached)").
- **Untracked layout partial invalidation (issue #781)**: After collecting `layoutChanges` from `changedFiles` and calling `previousCache.InvalidatedPages()` for each, check whether any layout change returned zero invalidated pages. A zero result means the changed file is not tracked as a direct layout for any page — it's either a partial (`layouts/partials/*.liquid`) or a brand-new layout. In either case, rebuild all pages (`pagesToRender = allPages`). Implementation: add an `untrackedPartial` bool flag that flips to true when `len(pages) == 0` for any layout change; if set, skip the per-page content-hash check and render everything. This is the conservative fallback — precise `{% include %}`/`{% render %}` dependency tracking is a future refinement.
- **Data file changes must refresh PipelineState.SiteData (issue #717)**: In `cmd/dev.go`, `PipelineState` is created once at server startup via `InitPipelineState()` and reused across all `BuildIncremental` calls. `ps.SiteData` is loaded from `cfg.Structure.Data` at creation time and never refreshed. When data files change on disk during a dev session, `ps.SiteData` becomes stale. This breaks paginated virtual pages that reference `site.data.*` — `processPagination` calls `ResolveDataSource(dataRef, siteData, ...)` with stale data, causing: (1) new data items to be invisible (no virtual pages generated), (2) modified data values to not appear in re-rendered pages, (3) removed data items to persist as ghost virtual pages. Fix: `BuildIncremental` must detect data file changes in `changedFiles` (any path under `cfg.Structure.Data`) and re-load site data from disk via `loadSiteData(cfg)`, updating `ps.SiteData` before `processPagination` runs. Additionally, data file changes must invalidate all paginated pages whose `pagination.data` references `site.data.*` from the changed data source, forcing re-rendering even when the source template's content hash is unchanged.
- **BuildIncremental lifecycle hook dispatch (issue #731)**: `BuildIncremental` must fire the same page-level and site-level lifecycle hooks as `Build()`. Currently only `OnDataFetched` (on data file changes) and `OnPagesReady` (via `applyBatchContext`) are dispatched. The following hooks are missing and must be added: (1) `OnContentLoaded` — after content discovery and markdown rendering, before cascade. Must fire for rebuilt pages only, applying frontMatter mutations back to page state. (2) `OnDataCascadeReady` — after cascade resolution. Must fire for rebuilt pages only, applying cascade data mutations. (3) `OnContentTransformed` — after content rendering, before layout wrapping. Must fire for rebuilt pages only via `fireContentTransformedHooks`. (4) `OnPageRendered` — after layout rendering, before output writing. Must fire as a batch dispatch via `RunBatchWithProgress` for rebuilt pages only, transforming final HTML (SSR, post-render). (5) `OnBuildComplete` — after all output is written. Must fire with a trimmed payload matching the documented shape `{ pageCount, duration, errors, outputDir }`, not the full `*BuildResult` (issue #1098, #1110). Hooks that are N/A for incremental: `OnConfig` (config change triggers full rebuild), `OnBeforeValidation`/`OnAfterValidation` (validation is full-build only), `OnAssetProcess` (asset pipeline not re-run incrementally).
- **Data reload edge cases (issue #719, #1018)**: The data reload path handles five edge cases: (1) **loadSiteData error preserves stale data (issue #1018)**: when `loadSiteData` returns a non-nil error during incremental rebuild, the existing `ps.SiteData` is preserved — pages re-render with previous values rather than crashing or producing empty output. `loadSiteData` returns errors from three sources: data directory parse failures (malformed JSON, stem conflicts via `data.LoadDirectory` — issue #982), missing/unreadable external files (`cfg.Data.Files`), and external-vs-directory key collisions. The `if err != nil` branch in `BuildIncremental` never mutates `ps.SiteData`, so stale data is always preserved on error. Test coverage: three tests in `incremental_test.go` — malformed JSON (issue #719), missing external file (issue #1018), and external key collision (issue #1018). (2) **Data directory deleted (nil data)**: when `loadSiteData` returns `(nil, nil)` (data directory does not exist — `fs.ErrNotExist` is silently ignored, no external files configured), the `else` branch checks whether the directory still exists on disk. If deleted, `ps.SiteData` is explicitly set to `nil`. If present, stale data is preserved. (3) **Collections-based pagination**: pages with `pagination.data: "collections.posts"` are NOT invalidated by data file changes — only `site.data.*` sources trigger invalidation. (4) **Non-string pagination.data**: the type assertion chain (`pagination.data` → string → `HasPrefix`) silently skips non-string values without panic. (5) **Custom data directory**: the `dataPrefix` check uses `cfg.Structure.Data` (configured value), falling back to `"data"` only when unconfigured.
- **Virtual page incremental rebuild (issue #970)**: Virtual pages injected by `onPagesReady` must participate in incremental rebuilds. The developer must implement:
  1. **Cache changes** (`internal/cache/cache.go`): Add a `virtualPages map[string]bool` field to the `Cache` struct (initialized in `New()`). Add `TrackVirtualPage(relPath string)` to record a RelPath as a virtual page. Add `VirtualPagePaths() []string` to return all tracked RelPaths (return non-nil empty slice when none are tracked — callers range over the result without nil checks). Update `Clone()` to deep-copy the `virtualPages` map. Update `Clear()` to reset the `virtualPages` map. **Empty-key guard (issue #1061)**: `SetHash` must silently discard calls with an empty key (`""`) — taxonomy pages have empty `RelPath`, and the `SetHash` loop in `Build()` passes `SetHash("", hash)` for each taxonomy page, creating a spurious cache entry.
  2. **Identifying virtual pages** (`internal/pipeline/incremental.go`): Before `applyBatchContext` runs, snapshot the set of discovered page RelPaths (the pages from filesystem discovery or contentMap). After `applyBatchContext` returns (which runs `onPagesReady` and may inject virtual pages), any page in `allPages` whose RelPath is NOT in the pre-snapshot set is a virtual page. Track each via `buildCache.TrackVirtualPage(relPath)` AND add each to `renderRelPaths[vp.RelPath] = true` so they are included in the post-pagination `pagesToRender` rebuild. This covers the initial build (nil cache) where step 3 is a no-op but virtual pages still need to be in `renderRelPaths`.
  3. **Pre-populating renderRelPaths from previous cache** (`internal/pipeline/incremental.go`): After `renderRelPaths` is built from `pagesToRender`, merge the previous build's virtual page RelPaths into it: `for _, vp := range previousCache.VirtualPagePaths() { renderRelPaths[vp] = true }`. This ensures virtual pages from the previous build are included when the post-pagination `pagesToRender` is rebuilt. When `previousCache` is nil (initial build), this step is a no-op — the initial build relies on step 2 to add newly identified virtual pages to `renderRelPaths`.
  4. **Removed virtual pages**: If a plugin stops returning a virtual page in a subsequent build, its RelPath is in `renderRelPaths` (from the previous cache) but no page in `allPages` has that RelPath after `onPagesReady` runs. The post-pagination filter (`pagesToRender` rebuild) naturally excludes it — the RelPath is in the set but no page matches it. The new cache only tracks virtual pages from the current build, so the removed RelPath will not be in the next build's cache.
  5. **Build()→BuildIncremental() handoff** (`internal/pipeline/build.go`): `Build()` must track virtual page RelPaths in its result cache, matching the `BuildIncremental()` tracking pattern. In `cmd/dev.go`, `Build()` runs first and its `result.Cache` is passed to the first `BuildIncremental()` call. Without tracking in `Build()`, the first incremental rebuild has no virtual page RelPaths to pre-populate into `renderRelPaths`. Both `Build()` and `BuildIncremental()` must skip pages with empty `RelPath` (taxonomy pages) when tracking virtual pages.
  6. **Test coverage**: 8 tests in `incremental_virtual_test.go` and 8 tests in `cache_test.go` covering: initial build renders virtual pages, subsequent incremental builds re-render them, layout changes re-render virtual pages, cache tracks virtual page RelPaths, output files are written, multi-generation cache chain works, removed virtual pages are excluded, Build()→BuildIncremental() cache handoff tracks and carries virtual pages, SetHash rejects empty keys, VirtualPagePaths returns non-nil empty slice.

- **File-derived virtual page dependency tracking (issue #1058)**: Optimizes the #970 naive approach (re-render all virtual pages on every incremental rebuild) by making virtual page re-rendering selective based on declared file dependencies. The developer must implement:
  1. **Page struct** (`internal/content/page.go`): Add a `Dependencies []string` field to `Page`. This is `nil` when the plugin does not declare dependencies (safe fallback — always re-render), an empty slice `[]string{}` when `dependencies: []` is declared (tracked, no file deps — skip during incremental rebuilds), or populated with file paths when `dependencies: ['...']` is declared.
  2. **extractVirtualPage** (`internal/pipeline/hooks.go`): After `virtualPageFromMap` constructs the page, `extractVirtualPage` reads the `dependencies` field from the plugin-returned map. If the key is absent → `page.Dependencies` remains `nil` (safe fallback). If present, validation is strict — malformed values produce a hard error that fails the entire `onPagesReady` invocation (one bad dependency entry in one virtual page prevents all virtual pages from being injected). Rationale: dependencies come from plugin JS code, not user-authored content, so malformed values indicate a plugin bug that should fail loudly. Validation: (1) Non-array value → error `"dependencies must be an array, got <type>"`. (2) Non-string entry → error `"dependencies[N] must be a string, got <type>"`. (3) Entry that resolves to empty or `"."` after `filepath.Clean` → error `"dependencies[N]: empty or current-directory path"`. (4) Absolute path → error `"dependencies[N]: absolute paths not allowed"`. (5) Path with `"../"` prefix or equal to `".."` after cleaning → error `"dependencies[N]: path escapes project root"`. Valid entries are normalized via `filepath.ToSlash(filepath.Clean(s))`. If the original `[]interface{}` was empty → assign `page.Dependencies = []string{}` (tracked, empty). Otherwise → assign `page.Dependencies = deps`.
  3. **Cache** (`internal/cache/cache.go`): Add a `virtualDependencies map[string]map[string]bool` field (source path → set of virtual page RelPaths). Initialize in `New()`. Add `TrackVirtualDependency(virtualRelPath, sourcePath string)` following the `TrackTemplateUsage` pattern (reverse map insertion). Add `InvalidatedVirtualPages(changedFile string) []string` following the `InvalidatedPages` pattern (lookup + collect). Update `Clone()` to deep-copy `virtualDependencies`. Update `Clear()` and `ClearVirtualPages()` to reset `virtualDependencies`.
  4. **BuildIncremental selective rendering** (`internal/pipeline/incremental.go`): Replace the #970 blocks that unconditionally add all virtual pages to `renderRelPaths` (the `previousCache.VirtualPagePaths()` pre-population and the `discoveredPaths` loop) with dependency-aware logic. Build a `changedFileSet` from `changedFiles` using `filepath.ToSlash()` for normalization. Build a `previousVirtualSet` from `previousCache.VirtualPagePaths()` (empty set when cache is nil). When `previousCache != nil`: iterate pages not in `discoveredPaths`; if the page is **new** (RelPath not in `previousVirtualSet`) → add to `renderRelPaths` (must render at least once before dependency filtering applies); if `page.Dependencies == nil` → add to `renderRelPaths` (safe fallback); if `page.Dependencies` is non-nil → add only if any dependency is in `changedFileSet`. When `previousCache == nil`: add all virtual pages to `renderRelPaths` (initial build, same as current behavior).
  5. **Cache tracking in BuildIncremental and Build** (`internal/pipeline/incremental.go`, `internal/pipeline/build.go`): After the build cache is populated with virtual page RelPaths (existing `TrackVirtualPage` call), also track dependencies: for each virtual page with non-nil `Dependencies`, call `buildCache.TrackVirtualDependency(page.RelPath, dep)` for each dependency path. This populates the cache for persistence.
  6. **Test coverage**: 9 cache unit tests in `internal/cache/virtual_dep_test.go` covering: TrackVirtualDependency + InvalidatedVirtualPages roundtrip, nil for unknown source, multiple pages sharing a source, multi-dep invalidation, deduplication, Clear resets virtualDependencies, ClearVirtualPages resets virtualDependencies, Clone preservation, Clone independence. 5 pipeline integration tests in `internal/pipeline/virtual_dep_incremental_test.go` covering: selective rebuild (matching dep rendered, non-matching skipped, untracked rendered, empty-deps skipped), initial build renders all, shared dependency invalidation, cache tracks dependencies across generations, Build()→BuildIncremental() handoff tracks dependencies. 5 malformed dependency validation tests in `internal/pipeline/hooks_test.go` (issue #1064) covering: non-array dependencies value produces error, non-string entry in dependencies array produces error, empty/dot path produces error, absolute path produces error, path escaping project root produces error.

#### Cascade wiring (PR #55)

The pipeline uses `cascade.PageContext` per spec §3 for proper 3-level cascade:

1. `cascade.LoadDirectoryCascade(contentDir)` — loads all `_data.yaml` files with parent→child merge
2. `cascade.FindCascadeData(cascadeData, contentBase, page.RelPath)` — walks up the directory tree to find the nearest ancestor with cascade data (handles directories without their own `_data.yaml`)
3. `cascade.BuildContext(siteData, dirData, page.FrontMatter)` — creates a `PageContext` with shared pointers for Global (level 1) and Directory (level 2), per-page FrontMatter (level 3)
4. `pctx.ToMap()` — flattens via `PageContext.Get()` (lazy deep-merge only for conflicting nested keys) into `page.FrontMatter` so downstream consumers (taxonomy building, collection sorting) see effective values

### WALKING SKELETON MILESTONE
At this point, `alloy build` works end-to-end on test fixtures.

**Verify**: `go test ./internal/template/... ./internal/static/... ./internal/pipeline/...`

---

## Phase 5: Plugin + Fetch + I18n + CLI (~117 tests)

### 5A: `internal/plugin` — 74 tests
**Files**: `hooks.go`, `registry.go`, `node.go`, `wasm.go`

- **hooks.go**: Hook registry with timeout, chained execution, warnings. `HookFunc` signature is `func(ctx context.Context, payload interface{}) (interface{}, error)` — context carries timeout deadline for cooperative cancellation (issue #13). `Run()` passes `context.Background()`. `RunWithTimeout()` uses `context.WithTimeout()` and passes the derived context to each hook.
- **registry.go**: Plugin classification by file type, discovery, filter registration, conflict warnings. Plugin source cleanup on `Close()` (issue #1042, 3 tests — Close clears plugin sources from global map, after Close FetchPluginSource returns "not registered" not "bridge not started", dev server rebuild after Close can register fresh handlers).
- **node.go**: LSP-style message encoding/decoding, bridge state management, stdout isolation (issue #968, 9 tests — 3 integration tests via fixture plugins proving process.stdout.write/console.log don't corrupt the protocol, 2 unit tests proving DecodeMessage includes actionable diagnostic for non-frame bytes, 4 unit tests proving NodeBridge.Send includes the same diagnostic for non-frame bytes including non-newline-terminated garbage (ReadString+EOF edge case), truncation at the 80-char boundary, and binary/non-UTF8 data in the snippet). Plugin source registration via `alloy.source()` (issue #979, 5 tests — EvalFile parses sources from eval response, CallSource invokes handler and returns data, CallSource propagates errors, CallSource returns error for unregistered source, duplicate source name produces warning). Plugin source error paths (issues #1043, #1045, 2 tests — CallSource with bridge==nil returns descriptive error identifying bridge state, CallSource returns timeout error for slow handlers instead of hanging indefinitely).
- **wasm.go**: QuickJS/WASM runtime with filter/shortcode/hook registration and execution.
  - `EvalFile()` parses `alloy.filter()`, `alloy.shortcode()`, and `alloy.hook()`/`alloy.on()` registrations
  - `CallFilter()` must execute the actual JS filter function and return the transformed value — not passthrough, not pattern-matching. The current `simulateJSFilter` approach only handles known patterns (word count); arbitrary JS like `toUpperCase()` returns input unchanged. Real QuickJS execution via wazero is required (issue #103).
  - `RegisteredHooks()` returns hook names discovered during `EvalFile()`
  - `LoadPlugins()` returns discovered filter names + hook registrations so the pipeline can bridge them to the template engine and HookRegistry

### 5B: `internal/fetch` — 25 tests
**File**: `internal/fetch/fetch.go`

- REST/GraphQL fetching, file-based caching, XML/CSV parsing, GraphQL data unwrapping
- Plugin source handler registration, invocation, error propagation, reset/cleanup (issue #979)
- Plugin source caching contract (issue #1044, 4 tests — cache round-trip preserves plugin source data, --refetch bypasses cache, cache populated after first fetch, failed fetch does not populate cache)

#### Source pipeline wiring (issue #107)

After `loadSiteData()` loads local data files, the pipeline must iterate `cfg.Sources` and fetch/cache each source before template rendering:

1. For each `SourceConfig` in `cfg.Sources`:
   - Check cache via `fetch.GetCached(name, cacheDir, source.Cache)` — if found and TTL valid, use cached data
   - If not cached (or `--refetch` flag set), call the appropriate fetcher based on `source.Type`:
     - `"rest"` → `fetch.FetchREST(source.URL)`
     - `"graphql"` → `fetch.FetchGraphQL(source.Endpoint, source.Query)`
     - `"plugin"` → `fetch.FetchPluginSource(source.Plugin, configMap)` (issue #979)
   - Save fetched data to cache via `fetch.SaveCache(name, cacheDir, data)`
   - Merge result into `siteData` under the `source.As` key so templates access it as `site.data.<as>`
   - When `source.As` is empty, use the source config key (map key from `cfg.Sources`) as the data key
2. Fire `onDataFetched` hook after all sources are merged (existing hook call stays in place)
3. On fetch failure: abort build with `return nil, fmt.Errorf(...)` — PLAN.md §5 says source failures abort the build. The current code warns and continues for all source types. The `case "plugin"` path must abort on error. The pre-existing warn-and-continue behavior for `"rest"` and `"graphql"` is a separate divergence from PLAN.md.

- **Plugin source caching (issue #1044)**: The `case "plugin"` branch in `Build()` must wrap `FetchPluginSource` with `GetCached`/`SaveCache`, matching the pattern used by `FetchRESTWithRefetch` for REST sources. Steps: (1) if `!cfg.Refetch`, check `fetch.GetCached(src.Plugin, cacheDir, src.Cache)` — if cache hit, skip the handler call; (2) on cache miss, call `fetch.FetchPluginSource(src.Plugin, configMap)`; (3) on success, call `fetch.SaveCache(src.Plugin, cacheDir, fetched)`. Currently the handler is called on every build, re-executing expensive API calls unnecessarily. Tests: 2 pipeline-level tests in `build_test.go` (cache hit skips handler on second build, cache populated after first build) + 1 pipeline test (--refetch bypasses cache) + 4 fetch-level tests (caching infrastructure handles plugin source data).
- **Plugin source handler leak on Close (issue #1042)**: `Registry.Close()` must call `fetch.ResetPluginSources()` to remove stale closures from the global `pluginSources` map. Without this, closures registered by `registerRuntime` capture stopped `NodeRuntime` instances — subsequent `FetchPluginSource` calls hit the dead bridge and return "bridge not started" instead of the expected "not registered". This matters for the dev server: after a full rebuild triggered by plugin file changes, the old registry is closed and a new one creates fresh handlers. Tests: 3 registry-level tests in `registry_test.go`.
- **Plugin source CallSource timeout (issue #1043)**: `NodeRuntime.CallSource()` must apply `cfg.Plugins.Timeout` (or the default 5s) via `context.WithTimeout`, matching the pattern used by `RunWithTimeout` for hook calls. Currently `CallSource` calls `r.bridge.Send()` with no timeout — a slow source handler (DNS timeout, unresponsive CMS API) blocks the build indefinitely. Tests: 1 test in `node_test.go` (CallSource returns timeout error for slow handler, not hang).

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
    //
    // IMPORTANT (issue #914): FindCascadeData must use the FULL
    // (lang-prefixed) relPath so it finds language-specific _data.yaml
    // entries (e.g., content/es/blog/ for es/blog/my-post.md).
    // Only ResolveFromCascade receives the stripped relPath.

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

### 5D-0: `internal/update` — 30 tests (issue #1071)
**File**: `internal/update/update.go`

Version update checking via the GitHub Releases API. Two modes: explicit (`alloy version --check`) and passive (background notification in `alloy dev`/`alloy serve`).

- **`UpgradeURL` constant**: `"https://alloyproject.org/docs/upgrade/"` — stable URL printed in update notifications. Must not change across releases. The docs page at this URL is tracked in issue #1072.
- **`IsNewer(current, latest string) bool`**: Semantic version comparison. Strip leading `v` prefix from both strings. Split on `-` to separate the version core from pre-release metadata. Parse the core as major.minor.patch integers. When cores are equal: stable (no pre-release) is newer than any pre-release; between two pre-releases, compare the numeric suffix (e.g., `rc2 > rc1`). When cores differ, compare major.minor.patch normally — pre-release metadata is irrelevant. A stable version is never older than its own pre-release (`0.5.0` is newer than `0.5.0-rc1`). Return `false` if either string is unparseable.
- **`CheckLatestVersion() (string, error)`**: Hit `GET https://api.github.com/repos/zeroedin/alloy/releases/latest` with a 10-second `http.Client` timeout. Parse the JSON response for `tag_name`. Return error for non-200 status, invalid JSON, or empty `tag_name`. Unauthenticated — subject to GitHub's 60 requests/hour rate limit.
- **`CheckLatestVersionFrom(baseURL string) (string, error)`**: Same as `CheckLatestVersion` but uses the given base URL. Tests use this with `httptest.NewServer` to avoid hitting the real GitHub API.
- **`CacheResult` struct**: `LatestVersion string`, `CheckedAt time.Time`. JSON tags for `bytedance/sonic` serialization.
- **`LoadCache() (CacheResult, error)`**: Read `update-check.json` from `CacheDir()`. Return zero `CacheResult` and nil error for missing file (`os.IsNotExist`). Return zero `CacheResult` and nil error for corrupt JSON (treat as cache miss, not an error). Return error only for unexpected I/O failures.
- **`SaveCache(result CacheResult) error`**: Write `update-check.json` to `CacheDir()`. Create the directory via `os.MkdirAll` if it doesn't exist.
- **`ShouldCheck(cached CacheResult) bool`**: Return `true` if `CheckedAt` is zero, `LatestVersion` is empty, or `time.Since(CheckedAt) >= 24 * time.Hour`.
- **`CacheDir() string`**: Return `filepath.Join(xdgConfigHome, "alloy")` where `xdgConfigHome` is `$XDG_CONFIG_HOME` if set, otherwise `~/.config` (via `os.UserHomeDir()`).
- Use `bytedance/sonic` for JSON (project convention — not `encoding/json`).

### 5D: `cmd/` + `main.go` — 15 tests
**Files**: `main.go`, `root.go`, `build.go`, `serve.go`, `init.go`, `version.go`

- **`main.go` exit code handling (issue #28)**: `main()` must check the error return from `cmd.Execute()` and call `os.Exit(1)` on failure. Without this, all CLI errors exit 0, breaking scripts and CI. Current code discards the error.
- Register Cobra flags (--config, --output, --root, --verbose, --quiet) on root; (--port, --no-drafts, --refetch) on dev; (--port, --refetch) on serve ✅ done (except --root, dev/serve split)
- `Version`: Set to non-empty string ✅ done
- **`version.go` `--check` flag (issue #1071)**: Register a `--check` bool flag on the version subcommand. When set: (1) print `alloy <version>`, (2) call `update.CheckLatestVersionFrom` (or `CheckLatestVersion` in production), (3) if error → print `Update check failed: <err>` and return nil (exit 0, not a build error), (4) if `update.IsNewer(Version, latest)` → print `Update available: <current> → <latest>` and `Upgrade: <update.UpgradeURL>`, (5) if not newer → append `(up to date)` to the version line, (6) save result via `update.SaveCache()`. Without `--check`, behave exactly as today.
- **Passive update notification wiring (issue #1071)**: In `cmd/dev.go` and `cmd/serve.go`, after the `Serving at http://localhost:...` line, if `cfg.UpdateCheckValue()` is true and `!cfg.Quiet`: (1) load cache via `update.LoadCache()`, (2) if `update.ShouldCheck(cached)` → launch a goroutine that calls `update.CheckLatestVersion()`, saves the cache, and prints `Update available: <current> → <latest> — <update.UpgradeURL>` to the command's stdout if an update is available, (3) if cache is fresh and has a newer version → print the notification immediately (no goroutine needed), (4) all errors silently swallowed. **`alloy build` must NOT import or reference `internal/update`** — tested via source code assertion in `cmd_test.go`.

#### `cmd/init.go` (issue #26)

`RunInit` needs a full rewrite — the current implementation only creates a bare config file and errors on existing projects. The new behavior scaffolds a complete starter project:

1. **Detect existing config** — Use `config.DetectConfigFile(dir)` to check all four extensions (`.yaml`, `.yml`, `.toml`, `.json`). If a config exists, print a message containing `"already exists"` via `cmd.PrintOut()` and return `nil` (no-op, not an error).
2. **Create target directory** if it doesn't exist (`os.MkdirAll(dir, 0755)`).
3. **Register six Cobra flags** on the init command: `--content`, `--layouts`, `--assets`, `--static`, `--data`, `--plugins` (all string, default empty). These override the corresponding `StructureConfig` directory names.
4. **Create seven project directories**: the six structure paths (using flag values if set, defaults if not). Use `os.MkdirAll` for each. When `--plugins` is not set, defaults to `"plugins"`.
5. **Generate `alloy.config.yaml`**: Must include `title` + `baseURL` (passes `config.Validate`). If any structure flag is non-default, include a `structure:` block with only the non-default values. If all flags are defaults, omit the `structure:` block entirely.
6. **Create starter files**:
   - `<layouts>/default.liquid` — HTML5 shell with `<!DOCTYPE html>`, `{{ page.title }}` in `<head>`, `{{ content }}` in `<body>`, link to `/style.css`.
   - `<content>/index.md` — YAML frontmatter with `title:` and `layout: default`.
   - `<static>/style.css` — Non-empty CSS with minimal reset and readable styling.
7. **Print success message** after scaffolding.

**Test coverage** (18 tests in `cmd/cmd_test.go`):
- Fresh project: config creation, 6 directories, default.liquid HTML5 content, index.md frontmatter, style.css non-empty, config validation, no structure: block with defaults, nested dir creation
- Custom flags: --content=pages, --layouts=templates, --static=public, all 5 flags combined, structure: block in config, only non-default values in structure: block, config validation with custom flags
- Existing project: DescribeTable for 4 config extensions returns nil, no directories created, prints "already exists"

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

#### `cmd/dev.go` (issue #256, was #29; watcher fix #371; PipelineState stale data fix #717)

Dev server command (`alloy dev`). Uses `ModeDev` — Phase 1 only, in-memory, drafts visible.

1. Load config (same as build).
2. **Check server lockfile (issue #1094)**: Call `server.CheckAndWarnLockfile(cfg.ProjectRoot)`. Print any returned warnings to stderr. Proceed regardless — lockfile warnings never block startup.
3. Set `cfg.IncludeDrafts = true` (unless `--no-drafts`).
4. Run initial build via `pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})`. `Build()` persists cache to disk at Stage 9 (`.alloy/cache.json`).
5. Read `--port`, `--no-drafts`, `--refetch` flags.
6. Call `server.NewWithMode(cfg, server.ModeDev)` and `server.Start()`.
7. **Write server lockfile (issue #1094)**: Call `server.WriteLockfile(cfg.ProjectRoot, server.LockfileInfo{PID: os.Getpid(), Port: actualPort, Mode: "dev", StartedAt: time.Now().Format(time.RFC3339)})`.
8. **Start file watcher (issue #371)** — call `server.WatchDirs(cfg)`, `addRecursiveWatch` on each directory, fsnotify event loop with `ClassifyChange` and debouncer. On file change, dispatch by `ChangeType`:
   - `ContentChange`/`LayoutChange`/`DataChange` → extract `changedFiles` from debounced events, call `pipeline.BuildIncremental(cfg, nil, previousCache, changedFiles, pipeline.BuildOptions{SkipSSR: true, PipelineState: ps})`. `previousCache` is kept in memory — initialized from the initial `Build()` result's `BuildResult.Cache` and updated from each `BuildIncremental` result's `BuildResult.Cache` (no disk round-trip). When `contentMap` is nil, `BuildIncremental` discovers content from the filesystem. Bulk changes (10+ files) trigger a full `Build()` instead. **Stale PipelineState.SiteData (issue #717)**: `ps` is created once at startup (line 135) and reused for all incremental rebuilds. When data files change, `BuildIncremental` must detect data file paths in `changedFiles` and re-load `ps.SiteData` from disk before `processPagination` runs — otherwise paginated virtual pages referencing `site.data.*` use stale data. After refreshing `ps.SiteData`, plugin runtimes that cache site data (e.g., QuickJS `alloy.data` via `rt.SetSiteData`) must also be updated — otherwise plugin filters/hooks still see stale data even though `ps.SiteData` is fresh.
   - `AssetChange`/`StaticChange` → **recopy changed files to `_site/` (issue #737)**. Dev mode serves from `_site/`, not source directories. Call `static.CopyStatic(staticDir, outputDir)` for `StaticChange` and `assets.CopyAssets(assetsDir, outputDir)` for `AssetChange`. For efficiency, a targeted single-file copy is preferred over recopying all files in the directory — but correctness (full recopy) is acceptable as a first implementation. **Transient file handling (issue #782)**: `fileutil.CopyFile` errors where `os.IsNotExist(err)` is true are silently skipped — atomic-write editors create `.tmp` files that vanish before the debounced copy runs; the rename target triggers its own event.
   - `PassthroughChange` → **recopy changed file to `_site/<to>/<relative-path>` (issue #737)**. Use `server.RecopyPassthroughFile(changedPath, cfg)` for targeted single-file copy, or `static.CopyPassthroughWithValidation(...)` for full passthrough recopy.
   - `ComponentChange` → full rebuild via `pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})`.
   - All types → `srv.BroadcastReload()` after rebuild.
   - **Bulk change protection**: If debouncer detects 10+ simultaneous changes (e.g., `git checkout`), always do a full rebuild. Cache is re-persisted by `Build()` Stage 9.
9. Block until interrupt.
10. **Remove server lockfile (issue #1094)**: In the shutdown path (after `<-sigCh`), read the lockfile and only remove if the PID matches the current process: `if info, _ := server.ReadLockfile(cfg.ProjectRoot); info != nil && info.PID == os.Getpid() { server.RemoveLockfile(cfg.ProjectRoot) }`. This PID guard prevents Process A from deleting a lockfile that Process B has since overwritten (cascade scenario, issue #1124).

#### `cmd/serve.go` (issue #256; watcher #291, fix #371)

Production server command (`alloy serve`). Uses `ModePreview` — same pipeline as `alloy build`, writes to `_site/`, SSR if configured, excludes drafts. **Must have a file watcher** — `alloy serve` is NOT a one-shot build (PLAN.md §8).

1. Load config (same as build).
2. **Check server lockfile (issue #1094)**: Call `server.CheckAndWarnLockfile(cfg.ProjectRoot)`. Print any returned warnings to stderr. Proceed regardless.
3. Set `cfg.IncludeDrafts = false`.
4. Run initial build via `pipeline.Build(cfg)`.
5. Read `--port`, `--refetch` flags. No `--no-drafts` (production always excludes drafts). No `--preview` (removed).
6. Call `server.NewWithMode(cfg, server.ModePreview)` and `server.Start()`.
7. **Write server lockfile (issue #1094)**: Call `server.WriteLockfile(cfg.ProjectRoot, server.LockfileInfo{PID: os.Getpid(), Port: actualPort, Mode: "serve", StartedAt: time.Now().Format(time.RFC3339)})`.
8. **Start file watcher (issue #291, #371)** — same setup as `cmd/dev.go`: call `server.WatchDirs(cfg)`, `addRecursiveWatch` on each directory, fsnotify event loop with `ClassifyChange` and debouncer. On file change, dispatch by `ChangeType`:
   - `ContentChange`/`LayoutChange`/`DataChange` → `pipeline.Build(cfg)` (full rebuild — serve mode always does full rebuilds, not incremental)
   - `AssetChange`/`StaticChange` → recopy changed files to `_site/`
   - `PassthroughChange` → `server.RecopyPassthroughFile(changedPath, cfg)` — copies only the changed file to `_site/<to>/<relative-path>`
   - `ComponentChange` → full rebuild (SSR re-render)
   - All types → `srv.BroadcastReload()` after rebuild/recopy
9. Block until interrupt.
10. **Remove server lockfile (issue #1094)**: In the shutdown path (after `<-sigCh`), read the lockfile and only remove if the PID matches the current process: `if info, _ := server.ReadLockfile(cfg.ProjectRoot); info != nil && info.PID == os.Getpid() { server.RemoveLockfile(cfg.ProjectRoot) }`. This PID guard prevents Process A from deleting a lockfile that Process B has since overwritten (cascade scenario, issue #1124).

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

## Phase 6: Server + SSR (~96 tests)

### 6A: `internal/server` — 73 tests
**Files**: `server.go`, `watcher.go`, `overlay.go`, `lockfile.go`

- HTTP server with mode-aware behavior (dev/preview)
- File watcher with debouncing and change classification
- **Passthrough watching (issue #275)**: `WatchDirs(cfg)` must iterate `cfg.Passthrough` and append each `from:` path. `ClassifyChange(path, cfg)` must add a `PassthroughChange` type for files matching passthrough source directories. On `PassthroughChange`, recopy only the changed file to `_site/<to>/<relative-path>` instead of triggering a full pipeline rebuild — this applies to both `alloy dev` and `alloy serve` since both serve from `_site/`. `addRecursiveWatch` must be called on each passthrough `from:` directory.
- **Watch directory config (issue #530)**: `WatchMapping` struct in `internal/config/config.go` — `From string`, `Type string` with yaml/toml/json tags. `Watch []WatchMapping` field on `Config` after `Passthrough`. `Validate()`: reject empty `from`, reject `type` not in {content, layout, data}, include array index in error. Reject nonexistent `from` directories (`os.Stat` or equivalent — fail fast). Reject duplicate `from` paths (build a seen-set, error on collision). Reject `from` matching base structure dirs (content, layouts, data, assets, static, plugins). Normalize trailing slashes in `from` (strip before storing/comparing). `WatchDirs()`: loop after passthrough — append `w.From` (or `static.GlobRoot(w.From)` for globs). `ClassifyChange()`: in `default:` branch, check watch dirs before passthrough loop — switch on `w.Type` to return `ContentChange`/`LayoutChange`/`DataChange`. No new `ChangeType` constant needed — reuses existing types. No changes to `cmd/serve.go`, `cmd/watcher.go`, or `RebuildScopeForChangeType` — existing dispatch handles all three types correctly.
- **Configurable plugins directory (issue #802)**: `StructureConfig` gets a `Plugins string` field (yaml/toml/json: `"plugins"`, default `"plugins"`). `ApplyDefaults()` sets it when empty. `Validate()` includes `cfg.Structure.Plugins` in the `baseDirs` overlap map so watch `from:` paths cannot conflict. `WatchDirs()` includes `structureDir(cfg.Structure.Plugins, "plugins")` in the base directories list. `ClassifyChange()` adds a `PluginChange` case before the `default:` fallback — matches `hasPathPrefix(path, pluginsDir)`. `PluginChange` is a new `ChangeType` constant; `RebuildScopeForChangeType` returns `RebuildPipeline` for it (handled by the existing `default:` case). In `cmd/dev.go`, the `hasComponentChange` check must also check for `PluginChange` to trigger a full `Build()` instead of `BuildIncremental` — plugin file changes require re-discovery via `DiscoverPlugins()`. In `internal/pipeline/state.go`, `DiscoverPlugins()` uses `cfg.Structure.Plugins` instead of hardcoded `"plugins"`. The `internal/plugin/Registry` already receives `pluginsDir` as a parameter, but its `SetProjectRoot` derivation via `filepath.Dir(pluginsDir)` breaks for nested paths like `tools/plugins` — `DiscoverPlugins` must pass `cfg.ProjectRoot` to a new `Registry.SetProjectRoot()` method instead. The `managedDirs` lists in `internal/pipeline/build.go` (passthrough validation and `validateOutputDir`) use `cfg.Structure.Plugins` instead of the hardcoded string. `cmd/init.go` registers a `--plugins` flag (default `"plugins"`) and uses it in directory creation and structure block generation.
- **Configurable components directory (issue #1099)**: `StructureConfig` gets a `Components string` field (yaml/toml/json: `"components"`, default `"components"`). `ApplyDefaults()` must set it when empty, same pattern as all other structure fields. `Validate()` must include `cfg.Structure.Components` in the `baseDirs` overlap map so watch `from:` paths cannot conflict. `WatchDirs()` must use `structureDir(cfg.Structure.Components, "components")` instead of the hardcoded `"components"` string when SSR is configured. `ClassifyChange()` must use `hasPathPrefix(path, componentsDir)` where `componentsDir` reads from config instead of the hardcoded `"components"`. In `internal/pipeline/incremental.go`, the component definition change detection block (the `strings.HasPrefix(normalized, "components/")` check) must use the configured components directory path from `cfg.Structure.Components` (falling back to `"components"` when empty). The `strings.TrimPrefix` that extracts the component tag name must also use the configured path. Fields outside the onConfig mutable allowlist — `structure.components` is not mutable via `onConfig` (same rationale as `structure.plugins`).
- **Content-colocated file serving (issue #300)**: Content-colocated non-content files (SVGs, images, JS in `content/`) are copied to `_site/` during the build. The server serves them from `_site/` like any other output file — no special request handler fallback needed.
- **Server lockfile — concurrent instance detection (issue #1094)**: Create `lockfile.go` in `internal/server/`. Detects when another `alloy dev` or `alloy serve` process is already watching the same project directory, and warns the user instead of silently corrupting output. Lockfile lives at `.alloy/server.lock` — inside the project-local cache directory, already gitignored. Uses `bytedance/sonic` via the package-level `jsonCodec` for JSON serialization (same as `watcher.go`).

  **Types and functions to create in `lockfile.go`:**

  - `LockfileInfo` struct — `PID int` (`json:"pid"`), `Port int` (`json:"port"`), `Mode string` (`json:"mode"`), `StartedAt string` (`json:"startedAt"`). `Mode` is `"dev"` or `"serve"`. `StartedAt` is RFC 3339 format.
  - `LockfilePath(projectRoot string) string` — returns `filepath.Join(projectRoot, ".alloy", "server.lock")`. When `projectRoot` is empty, produces relative path `.alloy/server.lock`.
  - `WriteLockfile(projectRoot string, info LockfileInfo) error` — creates `.alloy/` directory via `os.MkdirAll(dir, 0755)` if it does not exist, marshals `info` to JSON, writes atomically to `.alloy/server.lock`. Overwrites any existing lockfile.
  - `ReadLockfile(projectRoot string) (*LockfileInfo, error)` — reads and unmarshals `.alloy/server.lock`. Returns `(nil, nil)` when the file does not exist (`os.IsNotExist`). Returns parse error for corrupt JSON.
  - `RemoveLockfile(projectRoot string)` — removes `.alloy/server.lock`. No-op when the file does not exist. Does NOT remove the `.alloy/` directory — other files (fetch cache, WASM cache, profiles, plugin logs) live there.
  - `CheckAndWarnLockfile(projectRoot string) []string` — orchestrates the check-on-startup flow. Reads the lockfile. If missing, returns nil. If corrupt JSON, removes the lockfile and returns nil. If the PID is ≤ 0 (PID 0 or negative — invalid on all platforms), treats as stale, removes the lockfile, and returns nil. If the PID is dead (POSIX: `syscall.Kill(pid, 0)` returns error; Windows: `os.FindProcess` + process signal check), removes the stale lockfile and returns nil. If the PID is alive, returns warning messages (at least 3 strings): (1) identifies the conflicting process with PID, mode, port, and start time; (2) explains the impact (concurrent instances cause missing pages and 404s); (3) provides the kill command (`kill <PID>`). Does NOT remove the lockfile when the PID is alive — the other process owns it. The PID liveness check must use platform-specific code (existing pattern: `internal/plugin/node_posix.go` and `node_windows.go` use `syscall.Kill(pid, 0)` and `os.FindProcess` respectively).

  **Wiring in `cmd/dev.go` and `cmd/serve.go`:**

  After config loading but before the initial build, call `server.CheckAndWarnLockfile(cfg.ProjectRoot)`. Print any returned warnings to stderr. After the build and server start, call `server.WriteLockfile(cfg.ProjectRoot, server.LockfileInfo{PID: os.Getpid(), Port: actualPort, Mode: "dev"/"serve", StartedAt: time.Now().Format(time.RFC3339)})`. In the shutdown path (after `<-sigCh`), read the lockfile and only remove if the PID matches: `if info, _ := server.ReadLockfile(cfg.ProjectRoot); info != nil && info.PID == os.Getpid() { server.RemoveLockfile(cfg.ProjectRoot) }`. This PID guard prevents cascade deletion — if another process overwrote the lockfile, the shutting-down process must not remove it.

  **27 tests** in `lockfile_test.go`:
  - `LockfilePath` returns `.alloy/server.lock` under project root (2 tests)
  - `WriteLockfile` creates directory and writes correct JSON, overwrites existing, returns error when .alloy is a regular file, writes correct mode (5 tests)
  - `ReadLockfile` returns nil/nil when .alloy/ exists but server.lock does not, parses valid JSON, errors on corrupt JSON, returns error when .alloy is a regular file, returns nil/nil when .alloy/ directory missing (5 tests)
  - `RemoveLockfile` removes file, no-op when missing, preserves .alloy/ directory (3 tests)
  - `CheckAndWarnLockfile` returns nil when no lockfile, removes stale lockfile (dead PID) and returns nil, returns warnings for active lockfile (live PID) with correct content, format validation, treats corrupt lockfile as stale, non-blocking startup, treats PID 0 as stale and removes lockfile, treats negative PID as stale and removes lockfile, preserves lockfile when PID alive (9 tests)
  - PID ownership lifecycle: cascade scenario — Process A cannot delete lockfile after Process B overwrites (1 test)
  - `LockfileInfo` JSON round-trip and field name validation (2 tests)

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
| 6 | server, ssr | ~96 | ~570 |
| 7 | integration tests + remaining | ~86 | ~656 |

## Key Risks

1. **Liquid engine compatibility** — Biggest unknown. Evaluate osteele/liquid vs Notifuse/liquidgo against test expectations in Phase 0.
2. **Template tag preservation in Markdown** — goldmark must preserve `{{ }}`/`{% %}`. Requires custom extension or placeholder substitution.
3. **WASM/QuickJS runtime** — May need `wazero` dependency. Most infrastructure-heavy feature. Defer if needed.
4. **Pipeline test expectations** — `pipeline.Build` tests call `Build(cfg)` without specifying content directory. Need to handle defaults or infer from config.
