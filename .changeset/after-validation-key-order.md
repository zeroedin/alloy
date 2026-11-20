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

As in any JavaScript object, integer-like keys such as `"2"` and `"10"` come first, in numeric order, whatever order the hook set them in.
