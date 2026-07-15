---
type: minor
---

`onPagesReady` hooks accept a second return shape, `{ addPages: [...] }`, for injecting virtual pages without round-tripping the entire pages array through the plugin bridge.

```javascript
alloy.hook('onPagesReady', { data: ["elements"], pages: false }, function(payload) {
  var newPages = payload.siteData.elements.map(function(el) {
    return {
      path: 'demos/' + el.slug + '.md',
      url: '/demos/' + el.slug + '/',
      frontMatter: { title: el.name, layout: 'default' },
      content: '# ' + el.name
    };
  });
  return { addPages: newPages };
});
```

With `pages: false`, the plugin receives only `siteData`. Alloy skips serialization of existing pages and appends the `addPages` entries Go-side, cutting the O(N) cost of returning all pages.

Virtual pages from `addPages` flow through the remaining pipeline: taxonomy collection, content rendering, layout resolution, and output writing. They appear in `taxonomies.*` template variables and count toward `PageCount`.

The `{ pages: [...] }` return shape still works for plugins that mutate existing pages. The two shapes are mutually exclusive: returning both produces a build error. Returning an unrecognized key (e.g., `{ newPages: [...] }`) now errors instead of silently dropping pages.
