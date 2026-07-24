---
type: minor
---

Plugins declare external file dependencies via `addDependencies` in `onPageRendered` and `onContentTransformed` return values. Alloy tracks a reverse index in the build cache and rebuilds only the pages that declared a dependency when that file changes during `alloy dev`.

```javascript
alloy.hook("onPageRendered", {}, (page) => {
  const result = renderSSR(page.html);
  return {
    html: result,
    addDependencies: [
      "elements/rh-card/rh-card.js",
      "elements/rh-icon/rh-icon.js",
    ],
  };
});
```

`onFileChanged` hooks return `{ invalidateByDependency: [...] }` to trigger targeted rebuilds via the reverse index, or `{ restart: true }` to restart Node bridge subprocesses before the rebuild. Restart clears Node's ESM module cache so the plugin re-imports fresh component definitions.

```javascript
alloy.hook("onFileChanged", {}, (events) => {
  const changed = events
    .filter(ev => ev.path.startsWith("elements/") && ev.path.endsWith(".js"))
    .map(ev => ev.path);
  if (changed.length > 0) {
    return { invalidateByDependency: changed, restart: true };
  }
});
```

Dependencies from both hooks accumulate per page per build. Removing a component tag from a page drops that dependency on the next rebuild. Non-array `addDependencies` values produce a warning. Alloy normalizes dependency paths with `filepath.Clean`, so `./data.json` and `data/../data.json` match the canonical `data.json` cache key.

Previously, changes to files outside `content/` and `layouts/` either triggered no rebuild or forced a full rebuild of every page. A site with 720 pages and an SSR plugin that reads component definitions would rebuild all 720 pages when a single component file changed.
