---
type: minor
---

YAML and TOML data files now keep the key order you wrote them in, the way JSON already did. Renaming a file no longer reorders it:

```yaml
# data/nav.yaml
home: /
products: /products/
pricing: /pricing/
docs: /docs/
```

```liquid
{% for pair in site.data.nav %}
  <a href="{{ pair[1] }}">{{ pair[0] }}</a>
{% endfor %}
```

That renders `home, products, pricing, docs`. Previously it rendered `docs, home, pricing, products` — alphabetical — while the identical file saved as `nav.json` rendered in file order. Saving the same content as `.yaml`, `.toml` or `.json` now gives the same result.

Plugins see the same order. `Object.keys(alloy.data.nav)` returns the keys as written, for every format:

```javascript
export default function (alloy) {
  alloy.shortcode("nav", () => Object.keys(alloy.data.nav).join(", "));
}
```

**Go templates are unaffected** — they still sort map keys alphabetically, on every build, whatever the format. Ordering is visible in Liquid and to plugins. If order matters in a Go template, model the data as a list; a list iterates in file order in both engines.

Two things deliberately stay as they are. The `_data.yaml` directory cascade still sorts, because it merges each file into the one above it. CSV keeps its row order and has no column order to preserve.

Everything else about loading is unchanged: dates still load as dates, numbers as numbers, duplicate keys and merge keys (`<<: *anchor`) behave exactly as before.
