---
type: minor
---

**Breaking:** `onPageRendered` sends a page object `{ html, frontMatter, url, path }` instead of a raw HTML string. Only `html` in the return is applied back — `frontMatter`, `url`, and `path` are read-only context.

```javascript
alloy.hook("onPageRendered", {}, (page) => {
  if (page.frontMatter.layout === "demo") return page;
  page.html = page.html.replace(/<h2/g, '<h2 class="styled"');
  return page;
});
```

Plugins that conditionally process pages can read `page.frontMatter` to decide whether to transform. Both `Build()` and `BuildIncremental()` send the same payload shape.

Previously, the hook received a raw HTML string. Plugins that needed to skip certain pages had to embed `<meta>` markers in layout HTML and strip them downstream.
