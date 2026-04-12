# Alloy SSG Specification Critique — v2

## Context

Second review of PLAN.md after revisions addressing the original critique. The spec has grown from 2,393 to ~2,492 lines. Many critical issues from v1 were resolved. This review focuses on what remains open, what was partially addressed, and new issues introduced by the revisions.

---

## Resolved From v1 Critique

The following items were fully addressed and are no longer concerns:

| # | Issue | Resolution |
|---|---|---|
| 2.1 | TrackedDrop unimplementable | Moved to Future Features (post-v1). Incremental builds now use content-hash + full rebuild for shared data. Honest and correct. |
| 4.2 | Single-pass render scope leakage | Now two-pass: content rendered first, injected as `{{ content }}` into layout. Explicit scope isolation (lines 853-855). |
| 4.3 | JSON-RPC framing will break | Now uses length-prefixed LSP-style framing. Plugin stderr redirected to `.alloy/plugin.log` (lines 1539, 1566). |
| 2.6 | Error handling thin | Comprehensive new section (lines 883-916): build fails completely, dev keeps running, clear error messages with file/line/stage. |
| 2.7 | Plugin conflict resolution | New section (lines 1414-1425): alphabetical load order, last-wins with warnings, hooks chain in filename order. |
| 2.9 | Collection sorting underspecified | New section (lines 996-1021): date descending default, config-based custom ordering, inline filter examples. |
| 2.10 | `{{% raw %}}` delimiter confusing | Replaced with template tag auto-detection — `{{ }}`/`{% %}` pass through goldmark automatically. No special delimiters. |
| 4.5 | Dev server writes to disk | Dev mode now serves from in-memory map (line 2115). Preview/build write to disk. |
| 4.7 | Array merge rule undocumented | Now documented with clear example (lines 958-969). |
| 4.8 | No per-stage performance budgets | Added diagnostic target table (lines 2186-2198). |
| 3.5 | No built-in asset fingerprinting | Added `fingerprint` as Tier 1 built-in filter (line 1441). |
| 1.4 | Signals HMR too complex for v1 | Deferred to Future Features. Dev mode now uses full page reload. |
| 4.4 | Synchronous hook barriers | Acknowledged as future improvement (line 2485). Acceptable for v1. |
| 3.6 | No `alloy check` command | Added to Future Features (line 2489). |
| — | Tier 2 plugin error surfacing | New section (lines 1522-1529) on translating WASM/QuickJS errors to actionable messages. |

These were good changes. The spec is materially stronger.

---

## 1. STILL OPEN — Unresolved From v1

### 1.1 No Syntax Highlighting (HIGH)

Still no mention of code block syntax highlighting. For a developer-focused SSG, this is the single most expected Markdown feature after basic formatting. Every competitor has it: Hugo (Chroma), 11ty (Prism/Shiki via plugin), Jekyll (Rouge), Astro (Shiki).

`yuin/goldmark-highlighting` integrates with goldmark in ~10 lines of Go and uses Chroma (same highlighter as Hugo). It's a pure Go dependency with zero CGo.

**Recommendation:** Add to goldmark extensions in config and implementation Phase 1:
```yaml
content:
  markdown:
    goldmark:
      highlight:
        style: "monokai"
        lineNumbers: false
        guessLanguage: true
```

### 1.2 No Table of Contents Generation (HIGH)

Still no `page.tableOfContents`. Documentation sites depend on this. Goldmark already parses headings with auto-generated IDs — extracting a ToC is an AST walk during Markdown rendering, not a separate pipeline stage.

**Recommendation:** Add `page.tableOfContents` as a built-in computed field. Generate during step 11 (content transformation) by walking the goldmark AST.

### 1.3 No `page.wordCount` or `page.readingTime` (HIGH VALUE)

Still not mentioned as built-in computed fields. The spec still shows `wordCount` as a plugin example (line 1485). Hugo provides `.WordCount` and `.ReadingTime` on every page natively. These are ~5 lines of Go each and are the most commonly used page metadata on blog sites.

**Recommendation:** Compute during step 11 (content transformation) after Markdown rendering. Add to every page's context:
- `page.wordCount` — integer, words in rendered content (strip HTML, split on whitespace)
- `page.readingTime` — integer, minutes, assuming 200 WPM

### 1.4 No Git-Derived Metadata (MEDIUM VALUE)

Still no `page.git.*`. Hugo provides `.GitInfo` (last modified date, commit hash, author) per page. Essential for documentation sites showing "Last updated: April 3, 2026" without manually maintaining dates in front matter.

**Recommendation:** Add as opt-in via config:
```yaml
git:
  info: true
```
Populates `page.git.lastModified`, `page.git.hash`, `page.git.author` via `git log --format=... -- <filepath>` during content discovery.

### 1.5 `alloy init` Still Unspecified (MEDIUM)

Line 2134 still says only: "Create default alloy.config.yaml (fails if one already exists)." No specification of what it actually creates.

**Recommendation:** Specify the full scaffolding:
- `alloy.config.yaml` with sensible defaults
- Directory structure: `content/`, `layouts/_default/`, `data/`, `static/`, `assets/`
- Minimal `layouts/_default/base.liquid` and `layouts/_default/single.liquid`
- `.gitignore` for `_site/` and `.alloy/`
- Optional: use Go's `//go:embed` to bundle the starter in the binary (no network needed)

### 1.6 `index.md` Semantics Need a Dedicated Section (MEDIUM)

The spec changed `_index.md` to `index.md` (lines 54, 58) but didn't add a section explaining the semantics. This introduces a new ambiguity — see New Issue 2.1 below.

### 1.7 Node Bridge Security Model (LOW-MEDIUM)

Still no explicit trust model documentation for Tier 3. The spec says "Bring your own Node" and "Alloy spawns whatever `node` is in PATH" but doesn't address what a malicious or buggy plugin can do (full system access, `process.exit()`, arbitrary file I/O).

**Recommendation:** Add a brief security note to Section 5: "Tier 3 plugins run with the same permissions as the user. They can access the filesystem, network, and environment variables. Only install plugins you trust — treat them like npm scripts in `package.json`." Consider adding `--no-node` flag to disable the Node bridge entirely.

### 1.8 liquidgo Library Risk (LOW-MEDIUM)

Still using `Notifuse/liquidgo` with no mention of compatibility testing or fallback plan. The library has limited community adoption (single org, limited commits). Unknown Liquid feature coverage for: `forloop.parentloop`, `tablerow`, error modes (strict/lax/warn), whitespace control (`{%-`, `-%}`), `{% render %}` vs `{% include %}` scoping.

**Recommendation:** Before implementation begins, build a Liquid compatibility test suite covering the features Alloy needs. Run it against both liquidgo and `osteele/liquid`. Choose based on results. Document the chosen library's known limitations.

### 1.9 Content Excerpts (LOW VALUE)

Still no `<!-- more -->` separator support. Not critical but a cheap win.

### 1.10 Per-Collection Sitemap/RSS (LOW VALUE)

Still only global `sitemap.xml` and `feed.xml`.

---

## 2. NEW ISSUES — Introduced by Revisions

### 2.1 `index.md` vs `index.md` Ambiguity (HIGH)

The spec now uses `index.md` (not `_index.md`) for both section indexes and page bundle indexes:

```
content/
├── index.md                    # Site root page (line 54)
├── blog/
│   ├── index.md                # Blog landing page (line 58)
│   └── second-post/
│       └── index.md            # Page bundle index (line 61)
```

Hugo uses `_index.md` for section indexes and `index.md` for page bundles specifically to distinguish them. The distinction matters because:

- **Section index** (`blog/index.md` line 58) should use the `list.liquid` layout and represent the blog landing page
- **Page bundle index** (`second-post/index.md` line 61) should use the `single.liquid` layout and represent a single post with co-located assets

With the same filename for both, how does Alloy distinguish them? The only signal is directory structure — `second-post/` contains non-Markdown files (hero.jpg), so it's a page bundle. But what about a directory that contains only `index.md`? Is that a section with no posts, or a page bundle with no assets?

**Recommendation:** Either:
1. Restore Hugo's `_index.md` convention for section indexes (clearest, least surprising for Hugo migrants)
2. Define an explicit rule: `index.md` in a directory with other `.md` files is a section index; `index.md` in a directory with no other `.md` files is a page bundle. Document this clearly.
3. Require explicit `type: "section"` or `type: "page"` in front matter to disambiguate (most flexible but adds friction)

### 2.2 Template Tag Auto-Detection Edge Cases (MEDIUM)

The goldmark template tag extension (lines 1643-1666) auto-detects `{{ }}` and `{% %}` patterns and emits them as `ast.RawHTML` nodes. This is elegant but has edge cases:

**False positives in prose:** Text like "use the `{{ variable }}` syntax" in a paragraph (not inside a code span) would be treated as a real template tag and rendered by Liquid. The user likely meant to show the literal syntax. With the old `{{% raw %}}` approach, the user had explicit control. With auto-detection, there's no way to escape a template tag in prose without using Liquid's `{% raw %}...{% endraw %}`.

**Interaction with fenced code blocks:** The spec says the extension "registers an inline parser." Goldmark's fenced code blocks should already protect their contents from inline parsing, so ` ```{{ foo }}``` ` should be safe. But the spec should confirm this explicitly — it's the most common place template-like syntax appears in developer content.

**Interaction with inline code:** Similarly, `` `{{ foo }}` `` (backtick inline code) should not trigger the extension. Goldmark's inline code parser has higher precedence than custom inline parsers, but this should be stated.

**Ambiguity with Go template syntax in content:** If the Go template engine is configured (`engine: "go"`), the content file might contain `{{ .page.title }}` which looks like both a Go template tag and a potential Liquid tag. The goldmark extension preserves it, but which engine processes it?

**Recommendation:** Add a subsection to the goldmark extension spec that explicitly addresses:
1. Fenced code blocks and inline code are not affected (goldmark's built-in parsers take precedence)
2. To show literal `{{ }}` in prose, use Liquid's `{% raw %}...{% endraw %}` tag
3. The extension only fires for patterns that match `{{ ... }}` or `{% ... %}` with balanced delimiters

### 2.3 Layout Inheritance Chain Unspecified (MEDIUM)

The two-pass rendering (lines 1647-1648) cleanly separates content from layout. But the spec doesn't address layout inheritance — when one layout wraps another:

```liquid
<!-- layouts/_default/single.liquid -->
{% layout "base" %}
<article>{{ content }}</article>
```

```liquid
<!-- layouts/_default/base.liquid -->
<html><body>{{ content }}</body></html>
```

Questions:
- Does Alloy support layout chaining (single extends base)?
- If so, how many passes? Content → single → base = 3 renders?
- Or does Alloy use a different mechanism (e.g., `{% include %}` for partials, no layout inheritance)?
- The layout lookup order (lines 1229-1243) lists 5 fallback steps but doesn't describe chaining.

Hugo uses `{{ block "main" . }}...{{ end }}` / `{{ define "main" }}` for layout inheritance. Jekyll uses `layout: base` in the layout's own front matter. 11ty uses `layout` in data cascade.

**Recommendation:** Specify whether layouts can extend other layouts, and if so, the mechanism. The simplest v1 approach: layouts use `{% include %}` for shared partials, no layout-extends-layout. If layout inheritance is supported, specify the mechanism (front matter `layout` in the layout file, or a Liquid tag).

### 2.4 Duplicate Test File in Spec (TRIVIAL)

Line 2241 lists `merge_test.go` twice under `internal/cascade/`:
```
├── cascade/
│   ├── merge.go
│   ├── merge_test.go
│   └── merge_test.go     ← duplicate, should be tracked_drop_test.go or similar
```

---

## 3. RETAINED COMPLEXITY — Conscious Decisions

The following items from the v1 critique were not changed. They represent deliberate choices to keep scope. I note them here for completeness — they're risks to manage, not blockers.

### 3.1 Three-Tier Plugin System Still in v1

Tier 2 (QuickJS-on-WASM-on-wazero) remains. The new error surfacing section (lines 1522-1529) helps, but the 3-layer abstraction is still the highest-risk implementation item. The marshaling overhead between Go ↔ WASM ↔ QuickJS will need real benchmarking against the claimed ~10-50us per-call target.

**Risk mitigation:** Implement Tier 1 and Tier 3 first. Add Tier 2 last in Phase 5. If it proves too complex or slow, it can be cut without affecting the rest of the system.

### 3.2 Dual Template Engine Still in v1

Both Liquid and Go `html/template` remain (lines 104-105, 1157-1158). This doubles the filter registration surface, layout lookup logic, shortcode syntax, and documentation. The Go engine path has no data read tracking (acknowledged), different shortcode syntax, and a different file extension convention.

**Risk mitigation:** Implement Liquid first. Add Go template support after Liquid is solid. The `TemplateEngine` interface makes this a clean separation.

### 3.3 Phase 2 SSR Infrastructure Still in v1

Full component scanning, deduplication, dependency graph, stamp-back markers, and 3 SSR engine integrations remain (lines 1672-1798). This is ~130 lines of implementation spec for a feature most users won't need.

**Risk mitigation:** Implementation Phase 3 is dedicated to SSR. It can be deferred or descoped without affecting Phases 1-2 or 4-7.

### 3.4 i18n Still in v1

Full multilingual support with per-language content trees, translation linking, and per-language collections/taxonomies remains (lines 695-828).

**Risk mitigation:** Implementation Phase 2 includes i18n last. It can be cut from v1 without architectural impact.

### 3.5 Plugin Hooks Remain Synchronous Barriers

All pages batch through each hook before the next stage (line 881). With multiple Tier 3 plugins, IPC overhead accumulates linearly. The per-stage budget allocates 2 seconds for plugins (line 2196), which is tight for 3+ Node plugins across 1,000 pages.

The spec acknowledges this and defers parallel-safe hooks to Future Features (line 2485). Acceptable for v1 — the batching optimization (line 1618) helps, and most projects won't have 3+ content-transforming Node plugins.

---

## 4. SUMMARY

### Scorecard

| Category | v1 Issues | Resolved | Still Open | New |
|---|---|---|---|---|
| Overcomplexities | 6 | 2 (HMR, hooks) | 4 (retained as conscious decisions) | 0 |
| Spec Holes | 10 | 6 | 4 | 3 |
| Missed Opportunities | 7 | 2 (fingerprint, check) | 5 | 0 |
| Architectural Concerns | 8 | 6 | 2 | 1 (trivial) |
| **Total** | **31** | **16** | **15** | **4** |

### Must Address Before Implementation

1. **`index.md` ambiguity (2.1)** — Alloy must define how it distinguishes section indexes from page bundles when both use `index.md`. This affects content discovery, layout resolution, and collection membership — foundational behaviors.
2. **Syntax highlighting (1.1)** — Table-stakes feature. Add `goldmark-highlighting` to the dependency list and config spec.
3. **Template tag auto-detection edge cases (2.2)** — Document behavior in code blocks, inline code, and prose to avoid user surprise.

### Should Address Before Implementation

4. Table of contents generation (1.2)
5. `page.wordCount` / `page.readingTime` (1.3)
6. `alloy init` scaffolding (1.5)
7. Layout inheritance mechanism (2.3)
8. liquidgo compatibility evaluation (1.8)

### Nice to Have

9. Git-derived metadata (1.4)
10. Content excerpts (1.9)
11. Per-collection sitemap/RSS (1.10)
12. Node bridge security documentation (1.7)
