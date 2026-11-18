---
type: patch
---

Large sites build much faster. Alloy used to copy every page into `site.pages` for every page it rendered, once per layout, whether or not a template read it. It now builds the list only when a template reads `site.pages`, once per render pass.

On a plain blog with a two-layout chain, with the same output:

| Pages | Before | Now |
|---|---|---|
| 1,000 | 1.21s | 0.12s |
| 4,000 | 11.3s | 0.37s |
| 4,000, with a layout that reads `site.pages` | 11.0s | 0.64s |

Templates read `site` and `site.pages` as before, with three exceptions that count or test the keys of `site` itself:

| Template | Before | Now |
|---|---|---|
| `{{ site \| size }}` | `5` | `0` |
| `{% if site contains "title" %}`, or any other key | true | false |
| `{{ site["size"] }}` | nothing | `5` |

`{{ site.size }}` still gives `5`.
