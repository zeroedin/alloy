---
type: patch
---

Fix HTML injection in render hook context fields. All raw AST values passed to hook templates (`markup.inner`, `markup.language`, `markup.destination`, `markup.text`, `markup.title`, `markup.src`, `markup.alt`) are HTML-escaped before the hook template runs. Tags like `<h2>` and `<script>` in code blocks, URLs with `&` characters, and quotes in alt text display as intended instead of being interpreted as markup by the browser.
