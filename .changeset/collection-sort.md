---
type: patch
---

Collections and taxonomy terms are now sorted as the docs describe. Before, the build never sorted them: every list came out in file-name order, and `sortBy` and `order` under `collections:` did nothing. Lists on a site that relied on file order will reorder.

- A collection sorts by its `sortBy` and `order`, `date` and `desc` by default. Pages without a date, or without the `sortBy` field, go last in both orders. Ties go by file path.
- Another `sortBy` field compares as the `sort` filter compares, so `2` comes before `10`.
- A taxonomy term's pages (`taxonomies.<name>.<term>` and `taxonomy.pages`) sort by date, newest first.
- A page `onPagesReady` adds sorts by the `date` in its `frontMatter`.
- `order` must be `"asc"` or `"desc"`, or the build fails.

With posts A (2026-01-01), B (2026-03-01) and C (2026-02-01) tagged `x`, and guides G1 (`weight: 10`), G2 (`weight: 2`) and G3 (no weight) with `sortBy: weight` and `order: asc`:

| List | Before | Now |
|---|---|---|
| `collections.posts` | `A;B;C;` | `B;C;A;` |
| `collections.guides` | `G1;G2;G3;` | `G2;G1;G3;` |
| `taxonomies.tags.x` | `A;B;C;` | `B;C;A;` |

```
Error: validation error: collections.guides.order must be "asc" or "desc" (got "newest")
```
