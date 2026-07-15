---
type: minor
---

Go template block shortcodes use Hugo-style `{{% tag "args" %}}...{{% /tag %}}` delimiters. A preprocessor runs after Goldmark and before Go template rendering — it pairs opening and closing tags, extracts quoted arguments, passes inner HTML to the shortcode callback, and replaces the block with the callback's output.

```markdown
{{% callout "warning" %}}
This is **important** content with [links](/).
{{% /callout %}}
```

Nesting resolves innermost-first. Same-name nesting (`{{% box %}}{{% box %}}...{{% /box %}}{{% /box %}}`) uses depth tracking. Delimiters inside `<pre>` and `<code>` elements are treated as literal text. Unclosed tags, mismatched names, and callback errors produce build errors.

Goldmark now treats standalone `{{% tag %}}` lines as block-level nodes, preventing `<p>` wrapping. Inner content between paired tags is Markdown-processed before reaching the preprocessor.

Previously, Go template block shortcodes always received empty `content` because `goEngine.AddTag` hard-coded it to `""`.
