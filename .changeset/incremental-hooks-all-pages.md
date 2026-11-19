---
type: patch
---

In `alloy dev`, what an `onContentLoaded` or `onDataCascadeReady` hook adds to pages now stays on every page after a rebuild. Before, a rebuild ran these hooks only on the pages it re-rendered, so every other page lost what they added, for example in a layout that lists `site.pages`.

```javascript
// plugins/badge.js
export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true }, (pages) =>
    pages.map(p => { p.frontMatter.badge = 'hooked'; return p; }));
}
```

With three pages, A, B and C, and a layout printing `{% for p in site.pages %}{{ p.title }}={{ p.badge }} {% endfor %}`:

| In `alloy dev` | Before | Now |
|---|---|---|
| First build | `A=hooked B=hooked C=hooked` | `A=hooked B=hooked C=hooked` |
| After editing `a.md` | `A=hooked B= C=` | `A=hooked B=hooked C=hooked` |

Each rebuild runs both hooks again on every page, as a full build does, and `onContentLoaded` gets every page's rendered body. A rebuild still lays out and writes only the pages it re-renders.
