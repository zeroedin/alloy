---
type: patch
---

Fix raw HTML blocks inside blockquotes and tables rendering as live markup in `markup.inner` when render hooks are active. Alloy entity-encodes `<script>`, `<div>`, and other HTML blocks before the hook template runs. Goldmark's own formatting (`<strong>`, `<em>`, `<p>`) renders normally.
