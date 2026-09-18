---
type: minor
---

Go templates can now reach JSON data with ordinary dot notation. Previously every access needed nested `oget` calls, which made Go templates impractical for any site with real data:

```html
<!-- before -->
{{ oget (oget .site.data.tokens "color") "brand" }}

<!-- now -->
{{ .site.data.tokens.color.brand }}
```

It works at any depth and through arrays, so JSON design tokens and similar nested structures are reachable the same way YAML already was:

```html
{{ .site.data.config.nested.deep.leaf }}
{{ (index .site.data.nav.items 0).name }}
```

`{{ range }}` works directly on that data too — no `orange` needed:

```html
{{ range $key, $value := .site.data.sections }}
  <h2>{{ $key }}</h2>
{{ end }}
```

**Map keys now come out sorted alphabetically in Go templates.** JSON files used to iterate in the order you wrote them there; making dot notation work means handing Go templates a plain map, and a Go map has nowhere to store order. Liquid is unaffected and still shows file order for JSON.

**If order matters, use a list.** A list iterates in file order in both engines with no special handling, and it is the better fit for navigation menus — the usual reason people wanted map order:

```json
{ "sections": [ { "id": "intro", "title": "Introduction" } ] }
```

`orange` and `oget` still work. `orange` is now sorted rather than random for YAML and TOML data: it used to range a Go map directly, so three builds of unchanged input could produce three different orders.

Reach for `index` when a key is not a valid template identifier — one containing a hyphen, say: `{{ index .site.data.tokens "color-brand" }}`
