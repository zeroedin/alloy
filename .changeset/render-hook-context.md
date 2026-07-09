---
type: minor
---

Render hook templates now receive richer context for links and headings.

Link hooks receive `markup.title` from the Markdown link title syntax `[text](url "title")`.

```liquid
<a href="{{ markup.destination }}" title="{{ markup.title }}">{{ markup.text }}</a>
```

Heading hooks receive `markup.inner` as rendered HTML (preserving inline formatting like `<strong>`), `markup.text` as plain text, and `markup.attributes` as a map of goldmark attributes from `{.class #id key=value}` syntax. `markup.id` uses the explicit `{#custom-id}` attribute when present, falling back to the auto-generated slug.

```liquid
<h{{ markup.level }} id="{{ markup.id }}" class="{{ markup.attributes.class }}">
  {{ markup.inner }}
</h{{ markup.level }}>
```
