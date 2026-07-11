---
type: patch
---

Fix HTML tags inside fenced code blocks rendering as actual markup when a codeblock render hook replaces the default `<pre><code>` output. Tags like `<h2>` and `<script>` in `markup.inner` are HTML-escaped before the hook template runs, so they display as code text instead of being interpreted by the browser.
