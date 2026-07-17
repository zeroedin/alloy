---
type: patch
---

`onContentLoaded` now applies `html` mutations back to page state via `SetRenderedBody`. Previously only `frontMatter` changes were merged back; `html` mutations were silently dropped.

```javascript
alloy.hook("onContentLoaded", { pages: true, pageFields: ["*"] }, (pages) => {
  for (const page of pages) {
    page.html = page.html + "<footer>Injected</footer>";
  }
  return pages;
});
```

Both `html` and `frontMatter` mutations work independently or together in the same hook call. The fix applies to both `Build()` and `BuildIncremental()`.
