---
type: minor
---

**Breaking:** Remove automatic filename-based layout matching from layout resolution. Previously, `content/docs/getting-started.md` would automatically match `layouts/getting-started.liquid` — creating ambiguity when multiple content directories had pages with the same filename.

Layouts are now resolved via three mechanisms only:

1. Explicit `layout:` in front matter or `_data.yaml` cascade
2. Date-based convention (`post.liquid` for section children, `<section>.liquid` for index pages)
3. `default.liquid` fallback

If you relied on filename matching, add an explicit `layout:` to your front matter or `_data.yaml`:

```yaml
# content/docs/getting-started.md
---
layout: "getting-started"
---
```

```yaml
# content/docs/_data.yaml — applies to all pages in docs/
layout: "docs-page"
```
