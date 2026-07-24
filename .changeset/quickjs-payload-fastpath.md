---
type: patch
---

QuickJS per-page hooks (`onPageRendered`, `onFormatRendered`, `onContentTransformed`) pass string payload fields between Go and JavaScript without JSON serialization. `html`, `content`, `url`, `path`, and `format` bypass JSON on both the send and return paths. Only structured fields like `frontMatter` and `toc` still go through JSON.

Previously, each hook call serialized the entire payload — including rendered HTML — through a full JSON round-trip. On an 820-page site averaging 800 KB per page, this added ~29 seconds to post-render hooks.
