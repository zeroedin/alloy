---
type: minor
---

Block shortcodes can take their content raw. Add `>` after the opening delimiter and Alloy hands the body to your shortcode exactly as you wrote it, skipping Markdown entirely.

```liquid
{%> helmet %}
<script type="application/ld+json">
  { "@context": "https://schema.org", "name": "Alloy -- fast & extensible" }
</script>
{% endhelmet %}
```

Go templates use `{{%>`:

```html
{{%> helmet %}}
<script type="application/ld+json">
  { "@context": "https://schema.org", "name": "Alloy -- fast & extensible" }
</script>
{{% /helmet %}}
```

Without the `>`, Markdown rewrites that content before the shortcode ever sees it — `<` and `&` become entities, `--` becomes an en dash, indented lines turn into code blocks, and `unsafe: false` strips the `<script>` altogether.

Close tags are unchanged: `{% endname %}` and `{{% /name %}}` as always. Only the open tag takes the `>`, so the same shortcode can take raw content on one page and Markdown-formatted content on another with no change to how it's registered and no plugin API to learn.

A raw body passes through even when `goldmark.unsafe` is `false`. That is the point — script and structured data are what the feature is for — but it does mean you are opting that one block out of HTML sanitizing, so use it with content you control.

Raw blocks nest, so a matching `{% name %}` / `{% endname %}` pair inside the body will not close the block early. A block you forget to close fails the build and tells you where to look:

```text
content transformation: blog/post.md: unterminated raw block shortcode {%> helmet %} opened at line 12: expected {% endhelmet %}
```

This applies to Markdown files. Shortcode content in `.html` files already reaches your shortcode untouched, so there is nothing to opt out of there.
