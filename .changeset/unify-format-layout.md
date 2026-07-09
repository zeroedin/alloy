---
type: minor
---

Format layouts (JSON, XML, etc.) now follow the same predictable lookup order as HTML layouts. The format name is inserted before the template extension, so a single bare layout name drives all output formats.

The `layout` front-matter value must be a bare name with no extension:

```yaml
layout: article        # correct — resolves to article.json.liquid, article.xml.liquid, etc.
# layout: article.liquid  # build error — extension-bearing names cannot be used with format outputs
outputs: [html, json]
```

Using an extension-bearing layout name (e.g., `article.liquid`, `feed.xml`) with format outputs is now a build error. The error message tells you what to fix:

> extension-bearing layout "article.liquid" cannot be used with format outputs; use `layout: article` instead

### With `layout` set in front matter

Alloy looks for the named layout with the format infixed. For `layout: article` with JSON output:

1. `layouts/article.json.liquid`
2. `layouts/article.json` (bare-extension fallback)

If neither exists, the build errors with the layout name, page, and format.

### Without `layout` in front matter

Alloy walks the auto candidate chain. For a blog post with JSON output:

1. `layouts/post.json.liquid` — date-based section child
2. `layouts/post.json` — bare-extension fallback
3. `layouts/my-post.json.liquid` — matches the content filename
4. `layouts/my-post.json` — bare-extension fallback
5. `layouts/default.json.liquid` — final fallback
6. `layouts/default.json` — bare-extension fallback

Each candidate tries `.format.liquid` first, then the bare format extension. Higher-priority candidates (including their bare fallback) are checked before lower-priority ones.

Cascade layouts from `_data.yaml` also apply to format outputs, with front-matter taking priority as expected.
