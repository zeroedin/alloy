---
type: patch
---

Reduce peak memory on large sites. Alloy kept a duplicate of every page's rendered HTML in memory after writing it to disk. A 3,000-page site averaging 500KB of output carried ~1.5GB in the duplicate alone. Production builds and `alloy dev` skip the duplicate.

Trim the `onBuildComplete` hook payload to `{ pageCount, duration, errors }`. Alloy previously piped the full rendered HTML of every page to plugins over IPC on each build. Plugins that need output content can read `_site/` from disk.
