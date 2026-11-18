---
type: patch
---

QuickJS plugin filters now receive and return lists and objects as JavaScript values, as Node filters do. Before, a QuickJS filter, the default runtime, got anything but text, a number or `true`/`false` as Go's debug text, and an array or object it returned came back as text, with no error.

```javascript
// plugins/filters.js
export default function(alloy) {
  alloy.filter("keys", (obj) => Object.keys(obj));
  alloy.filter("summary", (tags) => ({ count: tags.length, first: tags[0] }));
}
```

With `data/nav.json` holding `{"home": "/", "blog": "/blog/", "about": "/about/"}` and `tags: [go, liquid]` in front matter:

| Template | Before | Now |
|---|---|---|
| `{{ site.data.nav \| keys \| join: ", " }}` | `0,1,2,…,57` | `home, blog, about` |
| `{% assign s = page.tags \| summary %}{{ s.count }} tags, first {{ s.first }}` | ` tags, first [` | `2 tags, first go` |

Also:
- A date arrives as text in RFC 3339 form, `2024-01-02T00:00:00Z`.
- An integer too big for 32 bits keeps its value: `3000000000` used to arrive as `-1294967296`.
- Nothing arrives as `null`, not the text `<nil>`.
- A data file object's keys arrive in file order, and on QuickJS a returned object keeps its key order.
- A returned object's own `size` and `first` keys win over the object's size and first pair, as on Node.

The plugin docs now describe what a filter receives and returns.
