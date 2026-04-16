# TDD Audit: Alloy Specification Coverage & Test Quality Review

## Context

This is a comprehensive TDD audit of the Alloy static site generator. The project follows a spec-first, test-first approach: PLAN.md defines every requirement, tests encode those requirements, and implementation stubs exist but return `ErrNotImplemented`. The goal is to identify specification coverage gaps, test quality issues, and TDD optimizations before implementation begins.

**Current state**: 45 test files across 16 packages, all using Ginkgo/Gomega BDD framework. All implementations are stubs. Three integration fixture sites exist (`test/fixtures/minimal`, `cascade`, `collections`).

---

## 1. Specification Coverage Gaps (Tests Missing for Spec Requirements)

### 1A. Major Gaps (Entire spec sections with no or minimal tests)

| Spec Section | Gap | Severity |
|---|---|---|
| **S5: Plugin System - Tier 2 (WASM/QuickJS)** | Spec (S11) explicitly lists `wasm_test.go` and `node_test.go` in the test org chart. Neither exists. No tests for QuickJS plugin loading, WASM module loading, sandbox isolation, or Tier 2 filter execution. | **Critical** |
| **S5: Plugin System - Tier 3 (Node Bridge)** | No tests for Node subprocess spawning, JSON-RPC protocol (length-prefixed framing), message types (hook/ssr/filter), process lifecycle, stderr redirection. | **Critical** |
| **S5: Plugin Load Order & Conflicts** | No tests for alphabetical load order, name conflict warnings (e.g., custom filter overwriting built-in `slugify`), or `.js` vs `runtime: "node"` disambiguation. | **High** |
| **S5: Plugin Timeout** | Spec defines 5s default timeout per hook. No test for timeout enforcement or fallback behavior. | **High** |
| **S7: Assets** | No `internal/assets/` package or tests. Spec defines `onAssetProcess` hook, asset directory copying, and `url` filter for asset paths. | **High** |
| **S8: Dev Server - File Watching** | No tests for fsnotify integration, 50ms debounce, bulk change protection, or WebSocket live-reload notifications. | **High** |
| **S8: Dev Server - Error Overlay** | No tests for browser error overlay rendering on template/content failures. | **Medium** |
| **S8: Dev Server - Preview Mode** | No tests for `--preview` flag behavior (Phase 1 + Phase 2 SSR). | **Medium** |
| **S9: CLI Commands** | `cmd/root.go` is a stub. No tests for `alloy build`, `alloy serve`, `alloy init`, `alloy version`, or flag parsing (`--config`, `--output`, `--verbose`, `--quiet`, `--port`, `--no-drafts`, `--refetch`). | **High** |
| **S10: Performance / Caching** | No benchmark tests exist despite spec (S11) explicitly showing `BenchmarkBuild1000Pages`. No `.alloy/cache.json` tests, no content-hash change detection, no template invalidation tracking. | **High** |
| **S6: Two-Phase Rendering Integration** | `pipeline/build_test.go` has only 7 basic tests. No test for Phase 1 -> Phase 2 handoff, Phase 2 skip when no SSR, or intermediate HTML flow. | **High** |

### 1B. Per-Package Gaps (Tests exist but miss specific spec requirements)

#### `internal/config/`
- **Missing**: Config file auto-detection (`alloy.config.yaml` vs `.yml` vs `.toml` vs `.json`)
- **Missing**: CLI flag override merging (e.g., `--output` overrides `build.output`)
- **Missing**: Config validation error tests (invalid baseURL, negative timeout, unknown fields)
- **Missing**: Default value application tests (spec says `build.output` defaults to `_site`, `pagination.path` defaults to `"page"`, etc.)

#### `internal/content/markdown_test.go`
- **Missing**: Footnotes extension (spec mentions it under goldmark extensions)
- **Missing**: Typographer behavior test (spec: `typographer: true` — smart quotes, em-dashes)
- **Missing**: `.txt` format handling (spec: "wrap in `<pre>` or passthrough based on config")
- **Missing**: `templateTags: false` behavior (disabling template tag preservation)
- **Missing**: Escaping template tags with `{% raw %}...{% endraw %}`

#### `internal/content/frontmatter_test.go`
- **Missing**: `outputs` field parsing (spec S1e: `outputs: ["html", "json"]`)
- **Missing**: `sitemap` field parsing (per-page override: `sitemap: { priority: 0.8 }` or `sitemap: false`)
- **Missing**: `aliases` array parsing (spec S1b)
- **Missing**: `pagination` nested config with all fields (`data`, `perPage`, `as`)

#### `internal/content/discovery_test.go`
- **Missing**: Content format filtering (spec: `content.formats: ["md", "html"]` — should ignore `.txt` if not in list)
- **Missing**: Deeply nested directories (3+ levels)
- **Missing**: Empty directories handling
- **Missing**: Symlink handling (follow or ignore?)

#### `internal/template/filters_test.go`
- **Missing**: `replace_first` filter (listed in spec but no test)
- **Missing**: `contains` filter (listed in spec but only tested for existence, not behavior)
- **Missing**: `map` filter (spec: array map)
- **Missing**: `concat` filter (spec: array concat)
- **Missing**: `group_by` filter (spec: array group_by)
- **Missing**: `absolute_url` filter (spec: URL filter)
- **Missing**: `safeHTML` filter (spec: output safety for Go templates)
- **Missing**: Edge cases for math filters (division by zero, negative numbers)
- **Missing**: Edge cases for string filters (empty string, nil input)

#### `internal/template/layout_test.go`
- **Missing**: Layout lookup for Liquid engine (`.liquid` first, then bare extension fallback)
- **Missing**: Layout lookup for Go engine (bare extension directly)
- **Missing**: Taxonomy layout lookup (`layouts/taxonomies/tags.liquid` then `layouts/tags.liquid`)
- **Missing**: Multiple output format layout lookup (`single.json.liquid` -> `single.json`)
- **Missing**: Build error when no layout found (spec says build must error)

#### `internal/template/shortcodes_test.go`
- **Missing**: Shortcode invocation within template rendering (tests only call the Go func directly, not through `{% youtube "id" %}` in a template)
- **Missing**: Block shortcode with `{% callout "warning" %}...{% endcallout %}` syntax
- **Missing**: Shortcode argument edge cases (no args, many args, quoted strings)

#### `internal/template/liquid_test.go` and `gotemplate_test.go`
- **Missing**: Partials/includes (`{% include "partials/header" %}`)
- **Missing**: `{% render %}` tag (Shopify Liquid spec)
- **Missing**: Scope isolation between content and layout rendering
- **Missing**: Go template `{{ block }}` / `{{ define }}` layout inheritance

#### `internal/collection/collection_test.go`
- **Missing**: Custom sort order from config (`sortBy`, `order` fields)
- **Missing**: Default sort order verification (date descending, dateless pages after)
- **Missing**: Inline sort filters (`sort`, `reverse`)
- **Missing**: Collection freezing (read-only after build)

#### `internal/collection/taxonomy_test.go`
- **Missing**: Taxonomy page template context (`taxonomy.term`, `taxonomy.terms`, `taxonomy.pages`)
- **Missing**: Taxonomy index page generation at `/tags/`
- **Missing**: Custom taxonomy permalink patterns
- **Missing**: Layout error when no taxonomy layout found

#### `internal/pagination/pagination_test.go`
- **Missing**: Liquid permalink rendering for virtual pages (`/team/{{ member.slug }}/`)
- **Missing**: Data source resolution (`site.data.team`, `collections.articles`)
- **Missing**: Custom page path segment (`path: "p"` -> `/articles/p/2/`)

#### `internal/validation/conflicts_test.go`
- **Missing**: Auto-generated file conflicts (sitemap.xml, feed.xml)
- **Missing**: Pagination virtual page conflicts
- **Missing**: Taxonomy page conflicts
- **Missing**: Plugin-registered path conflicts (`onBeforeValidation` hook)

#### `internal/permalink/permalink_test.go`
- **Missing**: `:title` token (spec lists it but no test)
- **Missing**: Config-level section-to-pattern lookup (e.g., `permalinks.blog` applied to blog section)
- **Missing**: Fallback chain: front matter -> section pattern -> default pattern -> file path
- **Missing**: Alias URL generation

#### `internal/cascade/`
- **Missing**: Layered lookup with 5 levels (only 3-4 tested)
- **Missing**: Post-render computed data (Level 5: `onContentTransformed` plugins)
- **Missing**: Lazy deep merge verification (only merges on conflicting nested key access)

#### `internal/fetch/fetch_test.go`
- **Missing**: XML response parsing (spec: `text/xml` / `application/xml`)
- **Missing**: CSV response parsing
- **Missing**: GraphQL `data` envelope unwrapping
- **Missing**: Cache TTL enforcement (expired cache should not be used in build mode)
- **Missing**: Cache directory location (`.alloy/fetch-cache/`)
- **Missing**: `--refetch` flag behavior (bypass cache on startup)

#### `internal/output/`
- **Missing**: Pretty URL generation (no trailing `.html`, uses `index.html` inside directories)
- **Missing**: Multi-format output (same page -> `index.html` + `index.json`)
- **Missing**: `permalink: false` pages (process but don't write)
- **Missing**: Alias output (same content written to multiple paths)

#### `internal/ssr/`
- **Missing**: SSR config parsing (`ssr.render`, `ssr.serve.cmd`, `ssr.serve.endpoint`)
- **Missing**: HTTP protocol integration (POST HTML -> receive SSR'd HTML)
- **Missing**: stdio protocol integration (NUL-terminated HTML over stdin/stdout)
- **Missing**: Cache key: `hash(tag + sorted_attributes + component_definition_hash)`
- **Missing**: Component definition change invalidation
- **Missing**: Phase 2 output hashing (skip SSR when hash matches)

#### `internal/i18n/`
- **Missing**: Content tree routing (`content/en/` vs `content/fr/`)
- **Missing**: `site.language` data cascade integration
- **Missing**: Per-language collections (English blog only has English posts)
- **Missing**: Per-language taxonomy pages
- **Missing**: `root: true` output at root instead of under prefix

#### `internal/server/`
- **Missing**: HTTP request handling (serving pages)
- **Missing**: Static file serving from source (dev mode, no copy)
- **Missing**: Passthrough path mapping (URL path -> source directory)
- **Missing**: Port conflict handling
- **Missing**: Auto-browser-open behavior

#### `internal/plugin/registry_test.go`
- **Missing**: Plugin file extension routing (`.js` -> QuickJS, `.wasm` -> wazero, `.js` with `runtime: "node"` -> Node)
- **Missing**: Alphabetical load order
- **Missing**: Missing plugins directory handling
- **Missing**: Plugin error surfacing (QuickJS error -> user-facing message)

#### `internal/plugin/hooks_test.go`
- **Missing**: 9 of 12 lifecycle hooks not explicitly tested (only `onConfig`, `onContentLoaded`, `onBuildComplete` have constant tests)
- **Missing**: `onBeforeValidation` mutability (append to output map)
- **Missing**: `onAfterValidation` read-only enforcement
- **Missing**: `onDataCascadeReady` (inspect/modify cascade)
- **Missing**: `onContentTransformed` (modify rendered HTML)
- **Missing**: `onPageRendered` (post-process page HTML)
- **Missing**: `onAssetProcess` (asset file transforms)
- **Missing**: `onDataFetched` (modify fetched data)
- **Missing**: `onDevServerStart`, `onFileChanged` (server events)
- **Missing**: Hook error propagation (hook error -> build abort in production)

---

## 2. Test Quality Issues

### 2A. Structural Issues

| Issue | Location | Details |
|---|---|---|
| **Weak error path test** | `pipeline/build_test.go:43-58` | The "produces no output on failure" test has a conditional (`if err != nil`) that allows the test to pass whether the build fails OR succeeds. This is a no-op test — it can never fail. Should force a known-bad config that *must* error. |
| **Testing the function, not the integration** | `template/shortcodes_test.go` | Tests call the Go function directly (`fn([]string{"world"})`) instead of invoking the shortcode through the template engine (`{% hello "world" %}`). This validates the function signature but not the template integration path. |
| **Duplicate tests** | `plugin/registry_test.go` | Both tests (`DiscoverPlugins returns nil error` and `scans plugins directory`) do the exact same thing. Second test adds no value. |
| **Real API dependency** | `fetch/fetch_test.go` | Tests hit `jsonplaceholder.typicode.com`. This makes tests flaky (network failure = test failure), slow (HTTP round-trip), and non-hermetic. Should use `httptest.NewServer` for unit tests. |
| **Hardcoded temp path** | `fetch/fetch_test.go` | Uses `/tmp/alloy-cache-test` instead of `GinkgoT().TempDir()`. Could collide with parallel test runs or leave artifacts. |
| **Missing test cleanup** | `fetch/fetch_test.go` | No `AfterEach` to clean up the `/tmp/alloy-cache-test` directory. |
| **Overly permissive assertions** | `pipeline/build_test.go:92` | `Expect(result.PageCount).To(BeNumerically(">=", 0))` — zero is always >= 0. This test will pass even if the builder returns 0 pages. Should test against a fixture with known page count. |
| **Summary test misplaced** | `content/lifecycle_test.go:129-153` | Summary tests are inside `FilterByLifecycle` but summaries are not filtered by lifecycle. They're data access tests that should live in their own Describe block or in frontmatter_test.go. |
| **Redundant nil checks** | Many files | Pattern: `Expect(result).NotTo(BeNil())` followed by field assertions. The field assertions will panic on nil anyway, and Gomega will catch it. The nil check adds noise without safety. |

### 2B. Missing Test Patterns

| Pattern | Why It Matters |
|---|---|
| **No table-driven tests** | `filters_test.go` has 60+ tests with repetitive setup. Go/Ginkgo `DescribeTable`/`Entry` would reduce ~300 lines to ~80 while adding more cases. |
| **No error message contract tests** | Spec emphasizes descriptive errors with file paths and line numbers. Only `i18n_test.go` and `fetch_test.go` validate error message content. Other packages just check `HaveOccurred()`. |
| **No interface mock tests** | Spec (S11) calls for `testify/mock` for `SSREngine`, `TemplateEngine`, and Node bridge interfaces. Zero mock tests exist. No `go.sum` entry for testify/mock either. |
| **No integration tests** | Fixture sites exist (`test/fixtures/`) but zero test files reference them. Spec (S11) shows integration tests using `Build("test/fixtures/minimal")` — none written. |
| **No benchmark tests** | Spec (S10, S11) defines performance targets and shows `BenchmarkBuild1000Pages`. No benchmark files exist. No `test/fixtures/large/` directory. |
| **No concurrency tests** | Spec (S2, S10) describes worker pools and parallel pipeline stages. No tests verify concurrent safety, race conditions, or `sync.Pool` reuse. |
| **No fuzz tests** | Front matter parsing, markdown rendering, and permalink token replacement are prime candidates for Go's built-in `testing.F` fuzzing. |

---

## 3. Specification Gaps (Spec Ambiguities Found During Review)

| Area | Question |
|---|---|
| **S1f: Content without front matter** | Spec doesn't define behavior for `.md` files with no front matter block at all (no `---` delimiters). Is the entire file treated as content body? What's the default layout? |
| **S1b: Permalink `false` + aliases** | If a page has `permalink: false` AND `aliases`, are aliases also suppressed? Spec is silent. |
| **S1c: Pagination + lifecycle** | If a paginated data source contains draft pages, are they filtered before or after pagination chunks are computed? |
| **S1f: `publishDate` in dev mode** | Drafts are visible in dev mode. Are future-`publishDate` pages also visible in dev mode? Spec says only `draft: true` is visible in dev. |
| **S3: Computed data mutation safety** | Spec says computed data (Levels 4-5) has highest priority. Can plugins mutate the shared global/directory data pointers, or do they receive copies? |
| **S4: Template engine per-page** | Can different pages use different template engines? Spec says engine is global config, but layout extensions (`.liquid` vs `.html`) suggest per-file engine selection. |
| **S5: Plugin hook error in dev mode** | Spec says plugin *crash* stops the server, but what about a hook returning an error? Does a non-fatal hook error show in the overlay, or stop the server? |
| **S6: SSR for non-Lit components** | Spec focuses on Lit components. What happens when SSR encounters a custom element that's not a Lit component? (Vanilla Web Components, Stencil, etc.) |
| **S1h: Passthrough `from` paths** | Spec says "relative to project root" and "absolute paths also supported" but doesn't define behavior when `from` path is inside `content/` or `layouts/`. |
| **S3: Collection membership** | A page in `content/blog/archive/old-post.md` — is its section `blog` or `blog/archive`? Spec says "first directory segment" but test data doesn't cover nested sections. |

---

## 4. TDD Optimization Recommendations

### 4A. Immediate Fixes (Low effort, high impact)

1. **Fix the no-op pipeline error test** (`pipeline/build_test.go:43-58`). Replace the conditional with a deterministic error-inducing scenario (e.g., invalid content dir, missing layouts).

2. **Replace real API calls** in `fetch/fetch_test.go` with `httptest.NewServer`. Add a separate integration test file (`fetch_integration_test.go`) gated by a build tag for when you want to test real network calls.

3. **Use `GinkgoT().TempDir()`** in `fetch/fetch_test.go` instead of `/tmp/alloy-cache-test`.

4. **Delete the duplicate test** in `plugin/registry_test.go` (the second "scans plugins directory" test).

5. **Move summary tests** from `lifecycle_test.go` to `frontmatter_test.go` where they semantically belong.

### 4B. Structural Improvements

6. **Convert `filters_test.go` to `DescribeTable`**. Each filter category becomes a table. Reduces LOC by ~60%, makes adding new cases trivial:
   ```go
   DescribeTable("String filters",
       func(filter string, input string, expected string) {
           result, err := tmpl.ApplyFilter(filter, input)
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(Equal(expected))
       },
       Entry("upcase", "upcase", "hello", "HELLO"),
       Entry("downcase", "downcase", "HELLO", "hello"),
       // ...
   )
   ```

7. **Add the missing test files from the spec's own test org chart** (S11):
   - `internal/plugin/wasm_test.go` — Tier 2 WASM plugin loading and execution
   - `internal/plugin/node_test.go` — Tier 3 Node bridge protocol and lifecycle

8. **Create integration test files** that use the fixture sites:
   - `test/integration/build_test.go` — full pipeline through `test/fixtures/minimal`
   - `test/integration/cascade_test.go` — data cascade through `test/fixtures/cascade`
   - `test/integration/collections_test.go` — collections through `test/fixtures/collections`

9. **Add a `test/fixtures/components/` fixture** (listed in spec S11 but doesn't exist) for SSR pipeline testing.

10. **Add benchmark test file** `internal/pipeline/build_bench_test.go` with targets from S10:
    - 1,000 pages no SSR < 5s
    - Single file change < 200ms

### 4C. Test Discipline

11. **Add error message contract tests** for every package that produces user-facing errors. Pattern:
    ```go
    It("includes file path in error message", func() {
        _, err := SomeOperation("content/blog/bad.md")
        Expect(err.Error()).To(ContainSubstring("content/blog/bad.md"))
    })
    ```

12. **Add `testify/mock` dependency** to `go.mod` as the spec requires, and create mock interfaces for `TemplateEngine`, `SSREngine`, and the Node bridge IPC layer.

13. **Establish a lifecycle hook test matrix**. All 12 hooks from the spec should have at least:
    - Registration test
    - Execution test
    - Payload mutation test (for mutable hooks)
    - Read-only enforcement test (for immutable hooks: `onAfterValidation`, `onBuildComplete`, `onDevServerStart`, `onFileChanged`)

### 4D. Missing Test Categories

14. **Error path tests with spec-mandated error formats**. The spec defines exact error output formats (e.g., `[alloy] ERROR content/blog/my-post.md:14 ...`). Tests should verify these formats.

15. **Boundary condition tests**:
    - Zero pages site
    - Single page site
    - Page with empty body
    - Page with only front matter
    - Config with zero taxonomies
    - Pagination with perPage > total items
    - Permalink pattern with no matching tokens

16. **Negative/adversarial tests**:
    - Circular layout references (layout A includes layout B includes layout A)
    - Front matter with conflicting special fields (`permalink: false` AND `aliases: ["/foo/"]`)
    - Duplicate taxonomy term slugs from different source values
    - Content file outside content dir referenced by passthrough

---

## 5. Summary Scorecard

| Dimension | Score | Notes |
|---|---|---|
| **Spec coverage breadth** | 60% | Core packages (config, content, template, cascade, permalink, pagination, collection, validation, ssr, i18n, output, fetch) have tests. Plugin system (tiers 2-3), assets, CLI, dev server, and performance are untested. |
| **Spec coverage depth** | 40% | Most test files cover the "sunny day" path. Edge cases, error formats, and spec-specific behavioral contracts are sparse. |
| **Test quality** | 65% | Good Ginkgo structure, readable assertions, proper fixture use. Dragged down by the no-op pipeline test, real API dependency, duplicate test, and misplaced summary tests. |
| **TDD readiness** | 70% | Stub-first approach is solid. Test files exist before implementation. Gaps are additive (write more tests) not structural (rewrite existing tests). |
| **Integration coverage** | 10% | Fixture sites exist but no integration tests use them. |
| **Performance coverage** | 0% | No benchmarks, no performance targets encoded in tests. |

---

## 6. Recommended Priority Order

1. Fix the 5 immediate quality issues (S4A items 1-5)
2. Add the missing lifecycle hook tests (all 12 hooks)
3. Add plugin system test files (wasm_test.go, node_test.go)
4. Add integration tests using existing fixtures
5. Convert filters to DescribeTable
6. Add missing filter tests (replace_first, contains, map, concat, group_by, absolute_url, safeHTML)
7. Add error message contract tests
8. Add CLI command tests
9. Add dev server behavioral tests
10. Add benchmark tests

---

## Files Reviewed

**Spec**: `PLAN.md` (all 12 sections + future features, ~2500 lines)

**Test files reviewed in detail**:
- `internal/pipeline/build_test.go`
- `internal/config/config_test.go`
- `internal/content/discovery_test.go`, `markdown_test.go`, `frontmatter_test.go`, `lifecycle_test.go`
- `internal/template/filters_test.go`, `layout_test.go`, `shortcodes_test.go`, `liquid_test.go`, `gotemplate_test.go`
- `internal/cascade/merge_test.go`, `context_test.go`
- `internal/collection/collection_test.go`, `taxonomy_test.go`
- `internal/pagination/pagination_test.go`
- `internal/permalink/permalink_test.go`
- `internal/validation/conflicts_test.go`
- `internal/output/formats_test.go`, `writer_test.go`, `sitemap_test.go`, `feed_test.go`
- `internal/plugin/registry_test.go`, `hooks_test.go`
- `internal/server/server_test.go`
- `internal/fetch/fetch_test.go`
- `internal/ssr/scanner_test.go`, `depgraph_test.go`
- `internal/i18n/i18n_test.go`
- `internal/data/loader_test.go`
- `internal/static/copy_test.go`
