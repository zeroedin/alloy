---
type: patch
---

Fix render hook templates receiving unescaped HTML in context fields. Alloy HTML-escapes all `markup.*` string values before the hook template runs, covering codeblock, link, image, and heading hooks. A `<script>` tag inside a fenced code block, an `&` in a link URL, `"` in image alt text, or `<beta>` in a heading display as code text instead of executing or rendering as HTML.

Heading `markup.inner` preserves goldmark's own formatting (`<strong>`, `<em>`) while escaping user-supplied raw HTML. Heading `markup.id` is escaped unconditionally, covering both the auto-generated slug path and the `{id="..."}` attribute override which can contain raw `&`, `<`, and `"`.
