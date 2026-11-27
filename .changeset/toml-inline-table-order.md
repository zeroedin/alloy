---
type: patch
---

Inline tables in a TOML array now keep their own key order. Before, every table after the first came out in the first table's order, and a key the first table didn't have went last.

With `data/t.toml` holding:

```toml
items = [{z = 1, a = 2}, {a = 3, z = 4}]
shift = [{b = 1, c = 2}, {a = 3, b = 4}]
```

and a layout with `{% for it in site.data.t.items %}({% for p in it %}{{ p[0] }} {% endfor %}){% endfor %}`:

| Array | Before | Now |
|---|---|---|
| `items` | `(z a )(z a )` | `(z a )(a z )` |
| `shift` | `(b c )(b a )` | `(b c )(a b )` |

This holds at any depth: inline tables inside them, dotted keys, and inline arrays inside a `[[table]]` element. Arrays of tables (`[[rows]]`) already kept each table's order.
