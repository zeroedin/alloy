---
type: patch
---

In `alloy dev`, data sources now stay in `site.data` after you edit a data file. Before, the rebuild reloaded `site.data` from the data files alone, so every plugin, REST and GraphQL source was missing until a restart. The rebuild now keeps each source as it was last fetched, at startup or on the last full rebuild, without fetching it again. `onDataFetched` runs again on the result, and `alloy.data` in QuickJS plugins sees the sources too.

With a plugin source `plan` returning `{ name: "pro", price: 9 }`, a data file `data/site.json`, and a layout printing `DATA[{{ site.data.site.title }}] PLAN[{{ site.data.plan.name }}]`:

| In `alloy dev` | Before | Now |
|---|---|---|
| At startup | `DATA[one] PLAN[pro]` | `DATA[one] PLAN[pro]` |
| After editing `data/site.json`, then the page | `DATA[two] PLAN[]` | `DATA[two] PLAN[pro]` |

Editing `data/site.json` alone doesn't re-render this page, before or now, so the second row edits the page too.
