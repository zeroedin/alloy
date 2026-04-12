# Alloy SSG Specification Critique

## Context

This is an architectural review of PLAN.md — a 2,393-line specification for Alloy, a Go-based static site generator combining Hugo's speed with 11ty's extensibility. The project has no code yet. The goal is to identify overcomplexities, specification holes, missed opportunities, and architectural concerns before implementation begins.

---

## 1. OVERCOMPLEXITIES — Defer or Simplify for v1

### 1.1 Three-Tier Plugin System (HIGH)

The Tier 2 stack — QuickJS compiled to WASM, running inside wazero (Go WASM runtime), executing user JavaScript — is **three layers of abstraction**. When a plugin throws an error, the stack trace traverses Go → wazero → QuickJS → user code. Debugging will be nearly impossible. The claimed performance numbers (~10-50us per call) don't account for marshaling Go values into WASM linear memory and back.

**Recommendation:** Ship v1 with Tier 1 (Go built-in) + Tier 3 (Node subprocess) only. Tier 3 already provides full extensibility. The `TemplateEngine` interface is tier-agnostic, so Tier 2 can be added later without breaking changes.

### 1.2 Dual Template Engine From Day One (MEDIUM)

Supporting Liquid AND Go `html/template` doubles every surface:
- Layout lookup differs (`.liquid` fallback vs bare extension)
- Shortcode syntax differs (`{% youtube %}` vs `{{ youtube }}`)
- Data read tracking only works with Liquid (two different invalidation strategies)
- Every filter registered twice, every doc example shown twice

Users who want Go templates already have Hugo.

**Recommendation:** Ship v1 with Liquid only. The interface supports adding Go templates later.

### 1.3 Phase 2 SSR Infrastructure (MEDIUM)

~400 lines of spec (lines 1565-1960) for component scanning, deduplication by attribute hash, dependency graphs, stamp-back markers, and three SSR engine integrations. This serves one audience: teams using Lit Web Components who want Declarative Shadow DOM. The spec already shows this working as a simple Node plugin hooking `onPageRendered` (lines 1874-1907) with zero core infrastructure.

**Recommendation:** Defer the deduplication/dependency-graph optimization layer. The plugin-based approach works for v1. Add the optimization when real-world perf data proves it's needed.

### 1.4 Signals-Based HMR (MEDIUM)

The dev mode HMR (lines 2006-2048) requires: `data-alloy` binding markers injected during Liquid rendering (template engine must be dev-mode-aware), a Preact Signals client, granular DOM patching for content changes, and component reconstruction via `cloneNode`. This is more sophisticated than what Hugo, 11ty, or Astro offer and is fragile across different component implementations.

**Recommendation:** Start with WebSocket full-page reload (already in the spec as a fallback). Add HMR as a v2 feature.

### 1.5 i18n System (LOW)

Well-designed and opt-in, but significant surface area (per-language collections, taxonomy generation, translation linking). Most early adopters won't need it.

**Recommendation:** Defer to post-v1. Architecture supports clean addition later.

### 1.6 Built-in REST/GraphQL Fetchers (LOW)

The plugin source type already handles the complex case. Built-in fetchers save ~10 lines of plugin code for simple unauthenticated GET requests.

**Recommendation:** Ship the plugin source interface. Defer built-in fetchers.

---

## 2. SPECIFICATION HOLES

### 2.1 TrackedDrop Cannot Be Implemented as Specified (CRITICAL)

The spec's core incremental rebuild strategy (lines 882-894, 1051-1091) relies on wrapping template context in a `TrackedDrop` implementing liquidgo's `LiquidMethodMissing` to record data key access.

**Neither `Notifuse/liquidgo` nor `osteele/liquid` supports this.** Both libraries resolve properties via direct reflection — no interceptable interface exists. There is no `Drop` interface in liquidgo, no `MethodMissing` in either library. The entire "track actual data reads" differentiator is unimplementable with any existing Go Liquid library without forking and modifying the library.

**Options:**
1. Fork liquidgo or osteele/liquid to add property access interception (substantial library work)
2. Wrap context maps in Go proxy types that log access before delegating (requires the library to resolve properties through an interface, not direct map indexing)
3. Accept that shared data changes trigger full rebuilds (as Go templates already do) and remove TrackedDrop from the spec
4. Use `osteele/liquid` which at least has a `Drop` interface — closer starting point for future interception

**Recommendation:** Option 3 for v1. At Alloy's target speed (1000 pages < 5s), full rebuild on shared data changes is fast enough. Content-hash invalidation (per-page) still works. TrackedDrop can be pursued as a v2 optimization if profiling proves shared data changes are a bottleneck.

### 2.2 No Syntax Highlighting (HIGH)

The spec mentions goldmark extensions for tables, footnotes, task lists, and typographer but omits syntax highlighting. For a developer-focused SSG, this is table stakes. `yuin/goldmark-highlighting` with Chroma (same as Hugo) is a zero-friction addition.

**Recommendation:** Add as Tier 1 built-in, enabled by default with configurable theme:
```yaml
content:
  markdown:
    goldmark:
      highlight:
        style: "monokai"
```

### 2.3 No Table of Contents Generation (HIGH)

No mention of generating ToC from Markdown headings. Hugo provides `.TableOfContents`, most SSGs have this. Critical for documentation sites.

**Recommendation:** Add `page.tableOfContents` as built-in, generated during goldmark AST walking.

### 2.4 `alloy init` Is Unspecified (MEDIUM)

Line 2113: "Create default alloy.config.yaml (fails if one already exists)" — that's it. No mention of directory scaffolding, starter templates, `.gitignore`, or flags.

**Recommendation:** Specify full scaffolding: config file, directory structure (`content/`, `layouts/`, `data/`, `static/`), minimal `layouts/_default/base.liquid`, and `.gitignore` for `_site/` and `.alloy/`.

### 2.5 `_index.md` Behavior Undefined (MEDIUM)

Line 52 mentions `_index.md` as "Section index (like Hugo)" but never defines what it does:
- Does it provide content for section list pages?
- Which layout does it use?
- Is it included in its own collection?
- How does its data merge with `_data.yaml`?

**Recommendation:** Add a dedicated subsection defining `_index.md` semantics.

### 2.6 Error Handling Strategy Is Thin (MEDIUM)

The spec shows error messages for specific cases but doesn't address:
- **Partial build failures:** If 3/1000 pages fail to render, does the build produce 997 pages or abort?
- **Plugin crashes in dev mode:** Does the server stop? Restart the plugin? Skip the hook?
- **Source location mapping:** After content+layout merge (step 14), line numbers in error messages point to the merged template, not the source file.
- **Graceful degradation:** What if an external data source is unreachable but stale cached data exists?

**Recommendation:** Define error severity (fatal/warning/info) and behavior per pipeline stage. Dev mode principle: "keep serving what you can, show errors clearly."

### 2.7 Plugin Conflict Resolution (MEDIUM)

No specification for:
- What happens when two plugins register the same filter/shortcode name
- Hook execution order (alphabetical? priority?)
- Plugin load order
- Plugin API versioning

**Recommendation:** Specify: last-registered wins for filters/shortcodes (with a warning), hooks execute in alphabetical filename order, add a plugin API version check.

### 2.8 Node Bridge Security Model (MEDIUM)

Tier 2 plugins are explicitly sandboxed. Tier 3 has full system access with no documentation of the trust model. A malicious plugin could `process.exit()`, `rm -rf /`, or exfiltrate data.

**Recommendation:** Document the trust model explicitly. Add `--no-node` flag. Recommend users audit third-party plugins.

### 2.9 Collection Sorting/Filtering Underspecified (LOW-MEDIUM)

Collections are available as `collections.blog` but:
- Default sort order not specified (by date? filename?)
- No documentation of sort-by-arbitrary-field
- Complex queries not addressed

**Recommendation:** Default sort by date descending. Document which array filters work on collections.

### 2.10 Passthrough Delimiter `{{% raw %}}` Needs Specification (LOW)

Not clear whether this requires a custom goldmark extension (it almost certainly does). The delimiter collides conceptually with Hugo's shortcode syntax and Liquid's built-in `{% raw %}` tag. The spec doesn't explain how the extension interacts with goldmark's HTML block parsing.

---

## 3. MISSED OPPORTUNITIES

### 3.1 Git-Derived Metadata (HIGH VALUE)

Hugo provides `.GitInfo` (last modified, commit hash, author) on every page. Trivial to implement via `git log --format=... -- <filepath>`. Essential for documentation sites.

**Recommendation:** Add `page.git.lastModified`, `page.git.hash`, `page.git.author`. Gate behind `git: { info: true }` config.

### 3.2 Built-in `page.wordCount` and `page.readingTime` (HIGH VALUE)

The spec shows `wordCount` as an example plugin (line 1443) when it should be a built-in. Hugo provides `.WordCount` and `.ReadingTime` natively. These are ~5 lines of Go each.

**Recommendation:** Add as built-in computed fields. Eliminates the most common "first plugin."

### 3.3 Content Excerpts via `<!-- more -->` (MEDIUM VALUE)

The spec says "no auto-generated summaries" but misses the middle ground: a `<!-- more -->` separator that splits rendered HTML into excerpt and remainder. No NLP, no heuristics — just a string split. Hugo and Jekyll support this.

**Recommendation:** Support optional `<!-- more -->`. If present, populate `page.excerpt`. If absent, nil.

### 3.4 Per-Collection Sitemap/RSS (MEDIUM VALUE)

Only global `sitemap.xml` and `feed.xml` are specified. Many sites need per-section feeds (`/blog/feed.xml`, `/docs/feed.xml`).

### 3.5 Built-in Asset Fingerprinting (MEDIUM VALUE)

Content-hash fingerprinting is pure Go computation. It's the single most impactful deployment optimization (infinite cache headers). Delegating to plugins is unnecessary.

**Recommendation:** Built-in `fingerprint` filter: `{{ 'css/main.css' | fingerprint }}` → `/css/main.abc123.css`.

### 3.6 `alloy check` Command (MEDIUM VALUE)

No validation command. Could validate front matter schemas, broken internal links, missing layouts, unused data files — reusing Phase 0 logic.

### 3.7 Embed Starter Template (LOW VALUE)

Go's `//go:embed` can bundle a minimal starter site in the binary, making `alloy init` self-contained.

---

## 4. ARCHITECTURAL CONCERNS

### 4.1 TrackedDrop Tracks Access, Not Control Flow (HIGH)

Even if TrackedDrop were implementable, it records ALL data keys resolved during rendering, including inside branches that don't execute:

```liquid
{% if page.showRelated %}
  {% for post in collections.blog %}...{% endfor %}
{% endif %}
```

Most Liquid engines resolve `collections.blog` before checking the `if` condition. This causes over-invalidation — pages marked as dependent on data they never use.

**Recommendation:** Document as known limitation. Add Phase 1 output hash comparison (not just Phase 2) to skip writing unchanged HTML.

### 4.2 Single-Pass Render Creates Scope Leakage (HIGH)

Content body is merged into layout as raw Liquid, then rendered in one pass (step 14). This means:
- `{% assign x = "foo" %}` in content is visible in layout after `{{ content }}`
- Layout variables are visible in content
- Shortcode tags share the namespace with layout tags
- Error line numbers map to merged template, not source files

**Recommendation:** Consider the alternative used by Hugo and 11ty: render content first to HTML string, inject into layout via `{{ content }}`. Prevents scope leakage, simplifies error reporting.

### 4.3 JSON-RPC Framing Will Break (MEDIUM)

"Newline-delimited JSON" (line 1489) fails when messages contain HTML with literal newlines (which they always will). Node's `console.log` writes to stdout will corrupt the JSON-RPC stream.

**Recommendation:** Use length-prefixed framing (like LSP's `Content-Length: N\r\n\r\n`). Redirect plugin stderr to a log file.

### 4.4 Plugin Hooks as Synchronous Barriers (MEDIUM)

Line 879: all pages batch through each hook before the next stage. With 3 Node plugins on `onContentTransformed` and 1000 pages at 5ms IPC overhead each, that's 15 seconds of plugin processing — exceeding the 5-second build target.

**Recommendation:** Allow parallel-safe hooks. Consider pipelining — start next stage for pages that have passed all hooks.

### 4.5 Dev Server Writes to Disk (MEDIUM)

Both dev and preview modes write to `_site/`. Modern dev servers (Vite, Next) serve from memory. Disk writes add latency and unnecessary SSD I/O.

**Recommendation:** Dev mode should serve from an in-memory map. Reserve disk writes for `build` and `--preview`.

### 4.6 liquidgo Library Risk (LOW-MEDIUM)

liquidgo has 63 commits from a single organization (Notifuse, for their email platform). No documentation of Liquid feature parity. Unknown handling of: error modes, filter chaining edge cases, nested `for` loops, `forloop.parentloop`. No fallback plan if insufficient.

**Recommendation:** Build a Liquid compatibility test suite before committing. Run against both liquidgo and `osteele/liquid` (513 commits, broader adoption, existing Drop interface). Choose based on results.

### 4.7 "Arrays Are Replaced" Merge Rule (LOW)

This will surprise users who expect concatenation (e.g., `_data.yaml` defines `scripts: ["analytics.js"]`, front matter defines `scripts: ["custom.js"]` — user wants both, gets only the latter).

**Recommendation:** Keep "replace" as default (safer), but document prominently with clear examples.

### 4.8 Performance Targets Are Undefined Per-Stage (LOW)

"1000 pages < 5s" is labeled "aspirational" but speed is the entire pitch. No per-stage budgets means no way to diagnose where time is spent.

**Recommendation:** Add stage budgets: discovery < 200ms, markdown < 500ms, templates < 1s, plugins < 2s, output < 500ms.

---

## Priority Summary

**Must resolve before implementation:**
1. TrackedDrop/liquidgo incompatibility (2.1) — core incremental rebuild strategy is unimplementable
2. Single-pass render scope rules (4.2) — undefined behavior
3. JSON-RPC framing (4.3) — will break on real HTML content
4. liquidgo library evaluation (4.6) — may need a different library entirely

**Should simplify for v1:**
5. Drop Tier 2 plugins; ship Tier 1 + Tier 3 (1.1)
6. Drop Go templates; ship Liquid only (1.2)
7. Drop signals-based HMR; ship full-page reload (1.4)
8. Defer Phase 2 SSR optimization layer (1.3)
9. Defer i18n (1.5)

**Should add:**
10. Syntax highlighting via goldmark-highlighting (2.2)
11. Table of contents generation (2.3)
12. `page.wordCount` and `page.readingTime` (3.2)
13. Git-derived metadata (3.1)
14. `alloy init` scaffolding spec (2.4)
15. `_index.md` semantics (2.5)
16. Built-in asset fingerprinting (3.5)
