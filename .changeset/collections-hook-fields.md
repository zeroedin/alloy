---
type: patch
---

Collections, taxonomy terms and pagination over a collection now show the fields `onContentLoaded` and `onDataCascadeReady` add. Before, they held each page as it was before those hooks, so a field a hook added was empty in `collections.<name>`, `taxonomies.<name>.<term>` and `pagination.items`, though `site.pages` showed it. In `alloy dev`, collections and taxonomy terms show them after each rebuild too.

With posts A, B and C in a `blog` collection tagged `x`, a plugin whose `onContentLoaded` sets `loaded: 'L' + title` and whose `onDataCascadeReady` sets `cascaded: 'D' + title`, and a layout printing `{{ p.title }}={{ p.loaded }},{{ p.cascaded }};` for each page:

| List | Before | Now |
|---|---|---|
| `collections.blog` | `A=,;B=,;C=,;` | `A=LA,DA;B=LB,DB;C=LC,DC;` |
| `taxonomies.tags.x` | `A=,;B=,;C=,;` | `A=LA,DA;B=LB,DB;C=LC,DC;` |
| `pagination.items` over `collections.blog`, two to a page | `A=,;B=,;` then `C=,;` | `A=LA,DA;B=LB,DB;` then `C=LC,DC;` |

Which pages each list holds, and their order, are still decided before the hooks: a hook that changes a page's `tags` or `date` doesn't move it.
