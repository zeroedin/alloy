---
type: minor
---

Plugins declare external dependencies in `onPageRendered` and `onContentTransformed` by returning an `addDependencies` array of file paths. Alloy tracks a reverse index in the build cache — when a dependency changes, only pages that declared it are rebuilt, even if the page content itself is unchanged.

```javascript
alloy.hook("onContentTransformed", {}, (page) => {
  return { html: page.html, addDependencies: ["data/nav.json"] };
});
```

`onFileChanged` hooks can return `{ invalidateByDependency: ["data/nav.json"] }` to target rebuilds to pages that depend on the changed file, or `{ restart: true }` to request a full dev-server restart. The hook contract and result parser are implemented; dev-server integration is pending.

Previously, any change to a non-content file triggered a full rebuild. Sites with expensive plugins that read external data files (design tokens, API caches, shared partials) will now rebuild only the affected pages.
