---
type: patch
---

Objects that a Node plugin gives back now keep their key order, as they do on QuickJS. Before, a Node hook or data source scrambled the order, differently on each build, and a Node filter's result came back sorted A to Z.

```javascript
// plugins/p.js
export const runtime = "node";
export default function(alloy) {
  alloy.hook("onDataFetched", { data: ["*"] }, (data) => data);
  alloy.filter("sizes", () => ({ small: "S", medium: "M", large: "L" }));
}
```

With `data/nav.json` holding `{"home": "/", "blog": "/blog/", "docs": "/docs/", "about": "/about/", "contact": "/contact/"}`:

| Template | Before, three builds | Now |
|---|---|---|
| `{% for pair in site.data.nav %}{{ pair[0] }} {% endfor %}` | `about contact home blog docs`, `home blog docs about contact`, `blog docs about contact home` | `home blog docs about contact`, every build |
| `{% assign s = "x" \| sizes %}{% for pair in s %}{{ pair[0] }} {% endfor %}` | `large medium small` | `small medium large` |

This covers objects at every level, from hooks (including those run in parallel), data sources (`alloy.source`) and filters. A filter result's own `size` and `first` keys still win.
