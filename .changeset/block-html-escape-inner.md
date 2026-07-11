---
type: patch
---

Fix raw HTML blocks inside blockquotes and tables rendering as live markup when blockquote or table render hooks are active. Alloy entity-encodes `<script>`, `<div>`, and other HTML blocks before the hook template runs. Inline formatting like bold and emphasis renders normally.
