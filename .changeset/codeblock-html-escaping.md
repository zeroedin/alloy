---
type: patch
---

Fix render hook templates receiving unescaped HTML in context fields. Alloy HTML-escapes all `markup.*` values before the hook template runs, covering codeblock, link, and image hooks. A `<script>` tag inside a fenced code block, an `&` in a link URL, or `"` in image alt text display as code text instead of executing or rendering as HTML.
