---
type: patch
---

Fix nested `_data.yaml` permalink patterns being silently ignored. Previously, only top-level section patterns were applied — a `content/blog/posts/_data.yaml` with `permalink: "/blog/:year/:month/:slug/"` had no effect. Permalink resolution now uses the nearest `_data.yaml` in the directory tree, so subdirectories can override their parent's URL pattern.

```yaml
# content/blog/_data.yaml — simple slugs for static pages
permalink: "/blog/:slug/"

# content/blog/posts/_data.yaml — date-based URLs for posts
permalink: "/blog/:year/:month/:slug/"
```

A page at `content/blog/posts/first-post.md` now correctly resolves to `/blog/2026/04/first-post/` instead of falling back to the parent's `/blog/first-post/`.
