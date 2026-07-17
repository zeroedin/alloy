---
type: minor
---

`onPagesReady` virtual pages accept a `dependencies` array of project-root-relative file paths. On incremental rebuilds, Alloy re-renders only virtual pages whose dependencies appear in `changedFiles`, skipping the rest.

```javascript
alloy.hook('onPagesReady', { pages: false }, function({ siteData }) {
  return {
    addPages: [{
      path: '_virtual/demos/button.html',
      url: '/demos/button/',
      dependencies: ['elements/button/demo.html'],
      frontMatter: { title: 'Button Demo', layout: 'demo', markdown: false },
      content: '<p>Button demo</p>'
    }]
  };
});
```

Three dependency states control incremental rebuild behavior:

- `dependencies: ['a.html', 'b.css']` — re-render when any listed file changes, skip otherwise
- `dependencies: []` — never re-render from file changes (no local file deps)
- no `dependencies` field — always re-render (safe fallback, matches pre-#1058 behavior)

Without this, a site with 400 file-derived virtual pages re-rendered all 400 on every incremental rebuild. With dependency tracking, only pages whose source files changed are re-rendered.
