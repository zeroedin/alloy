---
type: patch
---

After an `onDataFetched` or `onAfterValidation` hook, `.size` and `.first` now read each entry of `site.data` by where it came from. A data file's entry gives its key count and first pair, as without a hook. Every other entry, a data source or a key the hook adds, lets its own `size` and `first` keys win, as a source does without a hook. Before, `onDataFetched` made every entry read like a data file, and `onAfterValidation` made every entry, data files included, let its own keys win.

```javascript
// plugins/hook.js
export default function(alloy) {
  alloy.hook("onDataFetched", { data: ["*"] }, (data) => ({ ...data, extra: { size: "XL" } }));
}
```

With `data/nav.json` holding `{"home": "/", "blog": "/blog/", "docs": "/docs/"}`, and a plugin source `plan` returning `{ size: "large", first: "monthly", price: 9 }`:

| Template | Before | Now |
|---|---|---|
| `{{ site.data.nav.size }}` | `3` | `3` |
| `{{ site.data.plan.size }} {{ site.data.plan.first }}` | `3 sizelarge` | `large monthly` |
| `{{ site.data.extra.size }}` | `1` | `XL` |

The same hook as `onAfterValidation`, returning `{ cascade: { ...payload.cascade, extra: { size: "XL" } } }`, gave `{{ site.data.nav.first }}` nothing before. It now gives a pair from the file, `blog/blog/`. That's the first key in sorted order, since this hook doesn't keep a data file's key order yet (#1401).
