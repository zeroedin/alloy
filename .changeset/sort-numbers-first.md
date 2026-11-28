---
type: patch
---

The `sort` filter and a collection's `sortBy` now put every number before every text value, so the result no longer depends on the order the items come in. Numbers include integers, floats and strings of digits only; anything else sorts as text, after the numbers. Missing values still go last. With `order: "desc"`, a collection puts text first, Z to A, then numbers, largest first, and missing values stay last.

With three pages in a `guides` collection whose `weight` is `"2"`, `10` and `"1a"`, `collections.guides | sort: "weight"` gives:

| Pages, in path order | Before | Now |
|---|---|---|
| `"2"`, `10`, `"1a"` | `2 10 1a` | `2 10 1a` |
| `"1a"`, `10`, `"2"` | `10 1a 2` | `2 10 1a` |
| `"2"`, `"1a"`, `10` | `1a 2 10` | `2 10 1a` |

A collection with `sortBy: "weight"` and `order: "asc"` gives the same order as the filter. A collection's `order` defaults to `"desc"`, which gives `1a 10 2`.
