---
type: patch
---

Fix Go template engine format layouts resolving to `name.format.html` (e.g., `default.json.html`) instead of the correct bare format extension (`default.json`, `feed.xml`). The format extension is now used directly as the file extension, so `layouts/feed.xml` renders XML output and `layouts/api.json` renders JSON output without an `.html` suffix.
