---
type: patch
---

`alloy dev` keeps list pages and tag pages current. On a site with a collection or a taxonomy, changing a page's front matter, or adding or removing a page, re-renders the whole site and regenerates the taxonomy pages:

```yaml
# content/blog/one.md
title: One edited   # was: One
tags: [fresh]       # was: [a]
```

```text
GET /             LIST:[One edited]
GET /tags/fresh/  200  TAG fresh:[One edited]
```

Previously only the edited page was re-rendered. A page that lists others through `collections` or a taxonomy kept the old list, and a new tag got no page (`/tags/fresh/` returned 404) until you restarted `alloy dev`.

An edit that changes only a page's body still re-renders just that page, and a site with no collection or taxonomy stays incremental. A tag page whose last page leaves is deleted, as a full build would leave it out.
