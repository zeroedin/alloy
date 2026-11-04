---
type: minor
---

The `json` filter takes an optional indent, so a nested data file can be read without pasting it into another tool. A number is that many spaces, and a string is used as written:

```liquid
<pre>{{ site.data.nav | json: 2 }}</pre>
```

```html
<pre>{
  "home": "/",
  "products": "/products/"
}</pre>
```

The rules follow the `space` argument of JavaScript's `JSON.stringify`. A number is cut toward zero (`2.9` is 2) and capped at 10, and a string is cut to its first 10 characters. A number below 1, an empty string, or any other argument means no indent. Data file objects keep their file order at every level, and `<`, `>` and `&` inside strings stay escaped.

Previously `json` always printed one line. With no argument it still does, so existing templates print exactly what they did before.
