---
type: patch
---

On a page that paginates a collection, the name set by `pagination.as` (`item` by default) now shows the fields `onContentLoaded` and `onDataCascadeReady` add, as `pagination.items` does. Before, it held a copy of the items made before those hooks, so a field a hook added was empty in it. This works with `perPage: 1` too, where the name holds one item, and in `alloy dev` rebuilds.

With posts A, B and C in a `blog` collection, a plugin whose `onContentLoaded` sets `loaded: 'L' + title` and whose `onDataCascadeReady` sets `cascaded: 'D' + title`, and a layout printing `{{ p.title }}={{ p.loaded }},{{ p.cascaded }};` for each item:

| Page | Before | Now |
|---|---|---|
| `as: posts`, two to a page | `A=,;B=,;` then `C=,;` | `A=LA,DA;B=LB,DB;` then `C=LC,DC;` |
| `as: member`, `perPage: 1` | `A=,` | `A=LA,DA` |
| No `as:` (`item`), `perPage: 3` | `A=,;B=,;C=,;` | `A=LA,DA;B=LB,DB;C=LC,DC;` |

Front matter filled in from the item, such as `title: "One {{ member.title }}"` or a permalink, still uses the item as it was before the hooks.
