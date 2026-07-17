---
type: minor
---

`onPagesReady` virtual pages accept a `dependencies` array of project-root-relative file paths. On incremental rebuilds, Alloy re-renders virtual pages whose dependencies appear in `changedFiles` and skips the rest.

```javascript
alloy.hook('onPagesReady', { pages: false }, function() {
  const demoFiles = glob.sync('elements/*/demo/*.html');
  const pages = demoFiles.map(file => ({
    path: 'demos/' + path.basename(file),
    url: '/demos/' + path.basename(file, '.html') + '/',
    dependencies: [file],
    frontMatter: { layout: 'demo', markdown: false },
    content: fs.readFileSync(file, 'utf-8')
  }));
  return { addPages: pages };
});
```

- `dependencies: ['a.html', 'b.css']` — re-render when a listed file changes, skip otherwise
- `dependencies: []` — skip (no local file deps to invalidate)
- no `dependencies` field — re-render on all incremental rebuilds (pre-#1058 behavior)

A site with 400 file-derived virtual pages previously re-rendered all 400 per incremental rebuild. Declaring dependencies narrows that to the pages whose source files changed.
