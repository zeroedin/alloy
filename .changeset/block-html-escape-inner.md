---
type: patch
---

Fix block-level raw HTML inside blockquotes and tables passing through unescaped in `markup.inner` when render hooks are active. Tags like `<script>` and `<div>` in block HTML are entity-encoded, matching the inline raw HTML escaping already applied to `<beta>` and similar tags. Goldmark's own formatting (`<strong>`, `<em>`, `<p>`) renders normally.
