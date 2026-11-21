---
type: patch
---

Objects in `site.data` now keep their key order after an `onAfterValidation` hook, as they do after `onDataFetched`. Before, any `onAfterValidation` hook, even one that returned the cascade unchanged, made every object loop in sorted order: data files, plugin sources, and objects the hook built.

```javascript
// plugins/hook.js
export default function(alloy) {
  alloy.hook("onAfterValidation", {}, (payload) => ({ cascade: payload.cascade }));
}
```

With `data/nav.json` holding `{"home": "/", "blog": "/blog/", "docs": "/docs/", "about": "/about/"}`:

| Template | Before | Now |
|---|---|---|
| `{% for pair in site.data.nav %}{{ pair[0] }} {% endfor %}` | `about blog docs home` | `home blog docs about` |

As in any JavaScript object, array-index keys, which are whole numbers from `"0"` to `"4294967294"` without leading zeros, come first, in numeric order, whatever order the hook set them in. Every other key keeps the hook's order, including `"4294967295"` and above, `"07"`, `"-1"` and `"1.5"`.
