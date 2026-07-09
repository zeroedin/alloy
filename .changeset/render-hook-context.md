---
type: minor
---

Render hook context is now enriched with additional properties:

- **render-link**: `markup.title` passes the link title from `[text](url "title")` syntax
- **render-heading**: `markup.inner` now contains rendered HTML (preserving inline formatting like `<strong>`) instead of plain text
- **render-heading**: `markup.text` provides the plain-text version of the heading content
- **render-heading**: `markup.attributes` exposes a map of goldmark attributes from `{.class #id key=value}` syntax
- **render-heading**: `markup.id` uses the explicit `{#custom-id}` attribute when present, falling back to the auto-generated slug
