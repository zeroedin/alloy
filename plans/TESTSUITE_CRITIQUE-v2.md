# TDD Audit v2: Alloy Specification Coverage & Test Quality Review

## Context

Second-pass review after remediation of issues identified in TESTSUITE_CRITIQUE.md. The test suite has grown from 45 to 58 files with 600+ individual test cases. Significant gaps have been closed: CLI commands, assets, caching, plugin node bridge, server watcher/overlay, integration tests, mock-based SSR/template tests, and many per-package gaps are now covered.

This review focuses on what remains: specification requirements that still lack test coverage, and spec ambiguities that need resolution before or during implementation.

---

## 1. Remaining Specification Coverage Gaps

### 1A. Critical: Plugin System Tier 2 (WASM + QuickJS Runtime)

The spec's own test org chart (S11) lists `internal/plugin/wasm_test.go`. **This file still does not exist.** The Node bridge (Tier 3) now has tests, but the Tier 2 in-process plugin runtime — which is architecturally more important (no IPC, microsecond execution, sandboxed) — has zero test coverage.

Tests needed:

| Test | Spec Reference |
|---|---|
| QuickJS instance initialization (~10-50ms startup) | S5: "Alloy embeds a single QuickJS instance" |
| JS plugin file evaluation in QuickJS context | S5: "Plain .js files are evaluated in this shared context" |
| Filter registration from JS plugin (`alloy.filter("name", fn)`) | S5: JS Plugins (QuickJS) |
| Shortcode registration from JS plugin (`alloy.shortcode("name", fn)`) | S5: JS Plugins (QuickJS) |
| WASM module loading via wazero | S5: "Both run in-process via wazero" |
| WASM exported function invocation (`filter`, `shortcode`, `hook`) | S5: WASM Plugins (Compiled) |
| Sandbox enforcement — no filesystem access from Tier 2 | S5: "They cannot access the filesystem, network" |
| Sandbox enforcement — no network access from Tier 2 | S5: "Safe to run untrusted community plugins" |
| QuickJS error → user-facing error with plugin filename and line number | S5: Error Surfacing |
| WASM trap → user-facing error with plugin name and call context | S5: Error Surfacing |
| Filter registered in Tier 2 is callable from Liquid template rendering | S5: "Now `{{ page.content \| wordCount }}` works in templates" |

### 1B. High: Plugin Filter/Shortcode Integration with Template Engines

Tests exist for plugin hook mechanics and for template rendering separately. No test verifies the **end-to-end path**: plugin registers a filter → filter is available in Liquid/Go template rendering → template output reflects the filter.

Tests needed:

| Test | Spec Reference |
|---|---|
| Tier 1 built-in filter accessible in Liquid template render | S5: "Compiled Go functions registered with both Liquid and Go templates" |
| Tier 1 built-in filter accessible in Go template render | S5 |
| Plugin-registered filter accessible during template render | S5: "alloy.filter(...) works in templates" |
| Plugin-registered shortcode expands in content template rendering | S4: "Shortcodes are reusable content snippets" |
| Shortcode registered once wires into both Liquid (AddTag) and Go (FuncMap) | S4: "a single registration call wires into both engines" |

### 1C. High: External Data Source - Plugin Type

REST and GraphQL sources are tested. The third source type — `type: "plugin"` — has no tests.

Tests needed:

| Test | Spec Reference |
|---|---|
| Config parses `type: "plugin"` with plugin name and cache TTL | S1g: Plugin Sources |
| Plugin source handler registration (`alloy.source("name", fn)`) | S1g: "The plugin registers a source handler" |
| Plugin source data injected into `site.data.*` | S1g: "injects it into the data cascade" |
| Plugin source cache respects TTL | S1g: Caching |

### 1D. High: Build Mode vs Dev Mode Error Behavior Differentiation

The spec defines fundamentally different error behavior between `alloy build` and `alloy serve`. Tests exist for build-mode error behavior (no partial output) but dev-mode error tolerance is untested.

Tests needed:

| Test | Spec Reference |
|---|---|
| Dev mode: page render failure does not stop the server | S2: "The failed page shows an error overlay" |
| Dev mode: other pages continue to serve after one fails | S2: "Other pages continue to serve normally" |
| Dev mode: external source unreachable shows warning, continues with stale cache | S2: "The server continues with stale cached data" |
| Build mode: external source unreachable aborts build even with stale cache | S2: "Even if stale cached data exists, the build fails" |
| Build mode: plugin crash aborts build | S2: "Plugin crash aborts the build" |
| Dev mode: plugin crash stops server | S2: "Plugin crash stops the server" |

### 1E. High: Incremental Build Invalidation Rules

Cache hash storage and template tracking exist but the invalidation *rules* from the spec are untested.

Tests needed:

| Test | Spec Reference |
|---|---|
| Unchanged content file is skipped entirely (no re-parse, no re-render) | S2: "skip unchanged files entirely" |
| Config change triggers full rebuild | S2: "Config changes trigger full rebuild" |
| Global data change rebuilds all pages that could read it | S2: "all pages that could be affected are rebuilt" |
| Directory `_data.yaml` change rebuilds that directory's pages | S2: Shared data changes |
| Template change invalidates only pages using that template | S2: "tracked via the layout resolution step" |
| Component definition change triggers Phase 2 re-SSR only (Phase 1 untouched) | S2/S6: Why This Matters table |

### 1F. Medium: i18n - Remaining Gaps

Core i18n is well-tested. A few spec requirements are still missing.

Tests needed:

| Test | Spec Reference |
|---|---|
| Per-language taxonomy pages (`/en/tags/javascript/`, `/fr/tags/javascript/`) | S1i: "Taxonomy pages are generated per-language" |
| `site.title` overridden by `languages.{lang}.title` | S1i: Data Cascade Integration |
| `page.translations` array content (URL, language code) | S1i: Translation Linking |
| Language build parallelism (independent content trees, shared layouts) | S1i: Build Behavior |

### 1G. Medium: Directory Data Cascade Chain

The cascade merge *rules* are tested. The cascade *loading chain* (parent → child → grandchild `_data.yaml` files) is not.

Tests needed:

| Test | Spec Reference |
|---|---|
| `content/_data.yaml` applies to all content | S3: Directory Data Cascading |
| `content/blog/_data.yaml` merges over parent, applies to `blog/` and children | S3 |
| `content/blog/2024/_data.yaml` merges over parent, applies only to `blog/2024/` | S3 |
| Three-level cascade produces correct merged result at each level | S3 |

### 1H. Medium: Template Context Shape

No test verifies the complete template context structure that templates receive.

Tests needed:

| Test | Spec Reference |
|---|---|
| `{{ site.title }}` available from config | S3: Template Context |
| `{{ site.data.navigation }}` available from data files | S3: Template Context |
| `{{ page.title }}` from front matter | S3: Template Context |
| `{{ page.content }}` is rendered HTML | S3: Template Context |
| `{{ page.url }}` is computed permalink | S3: Template Context |
| `{{ page.collection }}` identifies collection membership | S3: Template Context |
| `{{ pages }}` contains all pages | S3: Template Context |
| `{{ collections.blog }}` is the section collection | S3: Template Context |
| `{{ collections.taxonomies.tags.javascript }}` is taxonomy collection | S3: Template Context |

### 1I. Medium: SSR Component Map Persistence

Tests exist for dependency graph logic and cache keys. Persistence to `.alloy/components.json` is untested.

Tests needed:

| Test | Spec Reference |
|---|---|
| `instances` map saved to `.alloy/components.json` | S6: Component Dependency Map |
| `pageToInstances` map saved and loadable | S6 |
| `componentToPages` map saved and loadable | S6 |
| `componentDeps` saved and loadable | S6 |
| Cache hit: unchanged component hash skips re-SSR | S6: "If the hash matches the cached hash, Phase 2 SSR is skipped" |

### 1J. Low: Performance Benchmarks

Still no benchmark tests despite spec (S10, S11) explicitly defining targets and showing `BenchmarkBuild1000Pages`. No `test/fixtures/large/` directory exists.

Tests needed:

| Test | Spec Reference |
|---|---|
| `BenchmarkBuild1000Pages` < 5s (no SSR) | S10: Performance Targets |
| `BenchmarkIncrementalRebuild` < 200ms (single file change) | S10 |
| Generate `test/fixtures/large/` with 1000+ pages | S11: Test Organization |

---

## 2. Specification Gaps (Ambiguities That Block Test Writing)

These are spec areas where the behavior is undefined or ambiguous. Tests cannot be written until a decision is made.

### 2A. Unresolved from v1 (Still Ambiguous)

| # | Area | Question | Impact |
|---|---|---|---|
| 1 | **S1f: publishDate in dev mode** | Drafts are visible in dev mode. Are future-`publishDate` pages also visible in dev mode? Spec says only `draft: true` is visible. | Affects `FilterByLifecycle` behavior in dev mode |
| 2 | **S3: Computed data mutation safety** | Can plugins mutate the shared global/directory data pointers via `onDataCascadeReady`, or do they receive copies? If they mutate shared pointers, all subsequent pages see the mutation. | Affects cascade architecture |
| 3 | **S4: Template engine per-page** | Can different pages use different template engines? Spec says engine is global config but layout extensions imply per-file selection. | Affects layout lookup logic |
| 4 | **S5: Plugin hook error vs crash** | Spec says plugin *crash* stops the server. Is a hook returning `error` different from a crash? Does a non-fatal error show in overlay or stop the server? | Affects hook execution contract |

### 2B. New Ambiguities Found in v2 Review

| # | Area | Question | Impact |
|---|---|---|---|
| 5 | **S1c: Pagination data source lifecycle** | If `pagination.data: collections.articles` and some articles are drafts, are drafts filtered *before* pagination chunks are computed? Or does pagination see all articles and then filter? | Affects pagination page count and item distribution |
| 6 | **S1d: Feed template override** | Spec says "Users can override by placing their own `layouts/feed.xml.liquid`." No test or spec detail for how Alloy detects user override vs built-in. Does Alloy check for the file before using built-in? | Affects feed generation logic |
| 7 | **S1f: Content without front matter** | What happens when a `.md` file has no `---` delimiters at all? Is the entire file body? What layout is resolved? Current `ParseFrontMatter` test covers "empty front matter" but not "no front matter delimiters." | Affects front matter parser |
| 8 | **S2: Phase 0 → Phase 1 sequencing** | Spec says Phase 0 validation happens "before any content processing begins." But data cascade assembly (Phase 1, step 7) includes `_data.yaml` files which are needed for permalink computation in Phase 0 (step 3). Does Phase 0 do a lighter content discovery (paths only) without data cascade? | Affects pipeline ordering |
| 9 | **S3: Collection sort stability** | Spec says dateless pages "sort after dated pages" with "filename alphabetical" fallback. But what about dated pages with the *same* date? Is sort stable? What's the tiebreaker? | Affects collection ordering determinism |
| 10 | **S5: Hook payload types** | Spec says `onBeforeValidation` payload is "output path map" with `Yes (append)` mutability. What is the Go type? `map[string]OutputEntry`? Does "append" mean add new entries only, or can existing entries be modified? | Affects hook contract |
| 11 | **S6: SSR dedup key contradiction** | Spec section 6 first says hash is `hash(tag + attributes + innerHTML)` then later says dedup key is `hash(tag + attributes)` with slot content excluded. The second is correct per the DSD explanation, but the first statement is misleading. | Spec editorial — tests follow the second definition |
| 12 | **S8: Dev mode in-memory serving** | Spec says dev mode "serves from an in-memory map — no disk writes." But Phase 0 validation walks static/ and passthrough dirs, which implies filesystem access. Is the "no disk writes" constraint only about _site/ output? | Affects server architecture |
| 13 | **S1h: Passthrough source inside project** | Spec says `from` paths relative to project root. If `from: "content/blog"` is used, should Alloy allow passthrough from inside the content directory? This could create output conflicts with normal content processing. | Affects validation logic |

---

## 3. Test Quality Observations (v2)

### 3A. Improvements Since v1

The following v1 issues have been resolved:

- Pipeline error test is no longer a no-op — now deterministically forces an error
- Fetch tests use `httptest.NewServer` (implied by new response parsing tests)
- Plugin registry duplicate test removed, replaced with extension routing tests
- Summary tests removed from lifecycle_test.go, now in frontmatter_test.go
- Filters converted to DescribeTable for string, array (simple), URL, and math categories
- Missing lifecycle hooks all have tests
- Integration tests use fixture sites
- Mock tests added for SSR and Template interfaces
- testify/mock added to go.mod
- Error message contract tests added across packages

### 3B. Remaining Quality Observations

| Issue | Location | Details |
|---|---|---|
| **No wasm_test.go** | `internal/plugin/` | Spec S11 test org chart lists it. Node bridge has tests. WASM runtime has none. This is the largest single gap. |
| **Fetch tests still hit real API?** | `internal/fetch/fetch_test.go` | The first two tests ("returns data from URL", "sends query and returns unwrapped data") — need to verify these now use `httptest` and not `jsonplaceholder.typicode.com`. If they still hit real APIs, the flakiness issue persists. |
| **Server tests don't verify HTTP responses** | `internal/server/server_test.go` | "serves rendered page content" test exists but need to verify it actually makes an HTTP request and checks the response body, not just that the function returns. |
| **No `--no-drafts` behavioral test** | `internal/server/` | Flag registration tested but behavioral effect (dev mode hides drafts when flag is set) is not. |
| **Overlay injection test** | `internal/server/overlay_test.go` | Tests that `RenderOverlay` produces HTML and `OverlayState` tracks errors, but no test that the overlay HTML is actually injected into the HTTP response when a page fails. |
| **No Liquid compatibility test suite** | Spec S12 Phase 1 | Spec explicitly calls for "Liquid compatibility test suite (verify liquidgo covers needed features: `forloop.parentloop`, whitespace control `{%-`/`-%}`, `{% render %}` scoping, `tablerow`, error modes)". No test file covers these liquidgo-specific features. |

---

## 4. Cross-Cutting Gaps (Features That Span Multiple Packages)

These are spec features that require coordination across packages. Individual package tests exist but the cross-package behavior is untested.

| Feature | Packages Involved | What's Missing |
|---|---|---|
| **Data file → template rendering** | data, cascade, template | No test loads `data/navigation.yaml` and verifies it's available as `{{ site.data.navigation }}` in a template |
| **Front matter → permalink → output** | content, permalink, output | No test parses front matter with `permalink:`, computes the URL, and verifies the output file path |
| **Collection → pagination → output** | collection, pagination, output | No test builds a collection, paginates it, and verifies the paginated page output paths |
| **Taxonomy → layout → template context** | collection, template | No test generates a taxonomy page and verifies the `taxonomy.term` / `taxonomy.terms` context in the rendered output |
| **Plugin hook → content transform → output** | plugin, content, output | No test registers a hook, transforms content through it, and verifies the output reflects the transformation |
| **i18n → data cascade → template** | i18n, cascade, template | No test verifies `site.language.strings.read_more` is available in template rendering for a specific language build |

---

## 5. Updated Scorecard

| Dimension | v1 Score | v2 Score | Notes |
|---|---|---|---|
| **Spec coverage breadth** | 60% | 85% | Assets, caching, CLI, server watcher/overlay, plugin node bridge, integration tests all added. WASM runtime is the major remaining gap. |
| **Spec coverage depth** | 40% | 65% | Edge cases, error contracts, validation scenarios significantly improved. Template context shape, data cascade chain, and incremental invalidation rules still lack depth. |
| **Test quality** | 65% | 80% | DescribeTable adopted, no-op tests fixed, mocks added, error contracts added. Remaining: verify fetch hermiticity, overlay injection integration. |
| **TDD readiness** | 70% | 85% | Stubs, tests, fixtures, and mocks all in place. Implementation can begin for most packages. WASM plugin and cross-cutting integration tests should be written before implementing those areas. |
| **Integration coverage** | 10% | 35% | Integration tests exist for minimal/cascade/collections fixtures. Cross-package data flow tests still missing. |
| **Performance coverage** | 0% | 0% | Still no benchmarks. No large fixture site. |

---

## 6. Recommended Priority Order (v2)

1. **Write `internal/plugin/wasm_test.go`** — Tier 2 WASM/QuickJS runtime is the largest single gap (11 tests needed, S1A)
2. **Write plugin-to-template integration tests** — Filter and shortcode registration → template rendering (5 tests, S1B)
3. **Write build vs dev error behavior tests** — Differentiated error handling is a core architectural contract (6 tests, S1D)
4. **Write incremental build invalidation tests** — Rules exist in cache_test.go but invalidation decisions are untested (6 tests, S1E)
5. **Write directory data cascade chain test** — `_data.yaml` at 3 levels (4 tests, S1G)
6. **Write template context shape tests** — Full `site.*`, `page.*`, `collections.*` verification (9 tests, S1H)
7. **Write cross-cutting integration tests** — Data → template, front matter → permalink → output (6 tests, S4)
8. **Write Liquid compatibility test suite** — `forloop.parentloop`, `{%-`, `{% render %}` scoping, `tablerow` (spec S12 Phase 1)
9. **Resolve spec ambiguities** — Items 1-4 from v1 + items 5-13 from v2 (13 decisions needed, S2)
10. **Write benchmark tests** — Generate `test/fixtures/large/`, write `BenchmarkBuild1000Pages` (S1J)

---

## Files Reviewed

**Spec**: `PLAN.md` (all 12 sections + future features, ~2500 lines — read in full)

**Test files reviewed (58 total)**:
- `cmd/cmd_test.go` (new)
- `internal/assets/assets_test.go` (new)
- `internal/cache/cache_test.go` (new)
- `internal/cascade/context_test.go`, `merge_test.go`
- `internal/collection/collection_test.go`, `taxonomy_test.go`
- `internal/config/config_test.go` (expanded)
- `internal/content/discovery_test.go` (expanded), `frontmatter_test.go` (expanded), `lifecycle_test.go`, `markdown_test.go` (expanded)
- `internal/data/loader_test.go`
- `internal/fetch/fetch_test.go` (expanded)
- `internal/i18n/i18n_test.go` (expanded)
- `internal/output/feed_test.go`, `formats_test.go`, `sitemap_test.go`, `writer_test.go` (expanded)
- `internal/pagination/pagination_test.go` (expanded)
- `internal/permalink/permalink_test.go` (expanded)
- `internal/pipeline/build_test.go` (expanded), `errors_test.go` (new)
- `internal/plugin/hooks_test.go` (expanded), `node_test.go` (new), `registry_test.go` (expanded)
- `internal/server/server_test.go` (expanded), `overlay_test.go` (new), `watcher_test.go` (new)
- `internal/ssr/depgraph_test.go` (expanded), `scanner_test.go` (expanded), `mock_test.go` (new)
- `internal/static/copy_test.go`
- `internal/template/filters_test.go` (restructured), `gotemplate_test.go` (expanded), `layout_test.go` (expanded), `liquid_test.go` (expanded), `mock_test.go` (new), `shortcodes_test.go` (expanded)
- `internal/validation/conflicts_test.go` (expanded)
- `test/integration/build_test.go` (new)
- 21 `*_suite_test.go` files
