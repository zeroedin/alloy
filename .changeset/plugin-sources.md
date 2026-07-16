---
type: minor
---

`alloy.source(name, fn)` registers a data source handler in Node plugins. Configure `type: "plugin"` in `sources:` to route data acquisition through the handler instead of a REST or GraphQL endpoint.

```yaml
# alloy.config.yaml
sources:
  blog:
    type: "plugin"
    plugin: "cms-posts"
    cache: 3600
    as: "blog"
```

```javascript
// plugins/cms.js
export const runtime = "node";
export default function(alloy) {
  alloy.source("cms-posts", async (config) => {
    const resp = await fetch("https://api.example.com/posts");
    return resp.json();
  });
}
```

Returned data merges into `site.data` under the `as` key (or the source map key when `as` is omitted). Templates access it like any other data source: `site.data.blog.size`, `{% for post in site.data.blog %}`.

Alloy caches plugin source results to `.alloy/fetch-cache/` using the same TTL and `--refetch` semantics as REST sources. A source handler error aborts the build. Duplicate `alloy.source()` calls for the same name produce a warning; the last registration wins.

Source calls enforce a 5-second timeout matching `plugins.timeout`. Slow handlers produce a timeout error instead of blocking the build.
