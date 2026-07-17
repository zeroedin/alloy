---
type: patch
---

Fix `alloy dev` skipping virtual pages from `onPagesReady` plugins on incremental rebuilds. Alloy tracks virtual page paths across rebuilds, so plugin-generated pages (demos, API docs, CMS-driven content) re-render when source files change instead of serving stale content.
