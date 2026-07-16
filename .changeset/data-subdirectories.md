---
type: minor
---

Alloy loads data files from subdirectories of `data/` into nested template namespaces. `data/nav/main.yaml` becomes `site.data.nav.main`. Nesting goes to any depth: `data/api/v2/endpoints.yaml` becomes `site.data.api.v2.endpoints`.

```yaml
# data/nav/main.yaml
items:
  - label: Home
    url: /
  - label: About
    url: /about/
```

```liquid
{% for item in site.data.nav.main.items %}
  <a href="{{ item.url }}">{{ item.label }}</a>
{% endfor %}
```

Place root-level data files alongside subdirectory namespaces in the same `data/` directory. A file and directory sharing the same stem (`nav.yaml` alongside a `nav/` directory) produces a build error, matching the collision behavior for same-stem files in different formats. Alloy skips empty subdirectories.

Alloy skipped subdirectories of `data/` without warning in prior releases. Organizing files into folders produced no template output despite the documented "any structure" guidance.
