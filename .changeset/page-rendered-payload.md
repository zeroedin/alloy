---
type: minor
---

`onPageRendered` now sends a page object instead of a raw HTML string. The payload shape is `{ html, frontMatter, url, path }`. Only `html` in the return is applied back — `frontMatter`, `url`, and `path` are read-only context for conditional processing.

```js
// Before (≤ v0.5.x):
alloy.hook("onPageRendered", {}, (html) => {
  return html.replace(/<h2/g, '<h2 class="styled"');
});

// After:
alloy.hook("onPageRendered", {}, (page) => {
  if (page.frontMatter.skipTransforms) return page;
  page.html = page.html.replace(/<h2/g, '<h2 class="styled"');
  return page;
});
```

Plugins that conditionally process pages (e.g., skip transforms on demo pages, apply different logic based on layout) no longer need to embed markers in the HTML itself. Check `page.frontMatter` directly instead.
