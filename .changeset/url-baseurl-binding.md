---
type: patch
---

`absolute_url` now prepends the site's configured `baseURL` automatically when no explicit argument is passed. The `url` filter prepends the path portion of `baseURL` to relative paths.
