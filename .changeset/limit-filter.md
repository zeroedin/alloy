---
type: minor
---

Add `limit` array filter that returns the first N elements, giving Go templates parity with Liquid.

Go template:

```gotemplate
{{ range limit .collections.blog 5 }}
  <h2>{{ .data.title }}</h2>
{{ end }}
```

Liquid:

```liquid
{% for post in collections.blog | limit: 5 %}
  <h2>{{ post.data.title }}</h2>
{% endfor %}
```
