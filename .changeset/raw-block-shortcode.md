---
type: minor
---

Block shortcodes can opt their body out of Markdown processing with a `>` immediately after the opening delimiter: `{%> name %}` for Liquid, `{{%> name %}}` for Go templates. The body is passed to the shortcode function byte-for-byte — no entity escaping, no typographer substitution, no block or inline Markdown, indentation and blank lines preserved. Close tags keep each engine's native syntax (`{% endname %}`, `{{% /name %}}`).

The marker belongs to the call site, not the shortcode registration, so the same shortcode can take a raw body on one page and a Markdown-processed body on another. It is stripped before the template engine sees the tag, so no plugin changes are needed.

A raw body is emitted verbatim regardless of `goldmark.unsafe`, which makes it possible to pass `<script>` and structured-data content to a shortcode on sites that otherwise strip raw HTML. This is an explicit per-invocation opt-out of HTML sanitization.

The matching close tag is found by counting nesting depth over own-line, same-name, same-delimiter-family tags. A raw block that never closes fails the build with the open tag, its line number, and the expected close tag rather than silently consuming the rest of the file.

This is a Markdown-only feature — `.html` content files never reach Goldmark, so their shortcode bodies are already raw.
