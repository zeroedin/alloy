---
type: patch
---

`absolute_url` now prepends the site's configured `baseURL` automatically when no explicit argument is passed, matching Hugo and Jekyll behavior. The `url` filter prepends the path portion of `baseURL` to relative paths.
