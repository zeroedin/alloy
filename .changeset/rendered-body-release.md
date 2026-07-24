---
type: patch
---

Free each page's rendered HTML after writing it to disk. Alloy held every page's `RenderedBody`, cached HTML string, and alternate format bodies in memory for the full build lifetime. A 3,000-page site averaging 500KB of output carried ~1.5GB in page bodies. Now only the largest single page sits in memory at once: O(total site HTML) becomes O(largest page).

`CaptureRenderedContent` (used by `BuildWithContent` tests) snapshots the HTML map before the output writing loop, so release does not affect test infrastructure. Sitemap generation and cache building read page metadata, not rendered bodies.
