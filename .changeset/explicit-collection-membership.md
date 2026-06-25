---
type: minor
---

Sections listed in the `collections:` config now declare collection membership without requiring date-token permalink patterns. Non-blog sections like releases or changelogs can participate in `collections.*` pagination and template loops.

```yaml
# alloy.config.yaml
collections:
  releases:             # declares releases/ as a collection — no date tokens needed
    sortBy: date
    order: desc
```

```yaml
# content/releases/_data.yaml
permalink: "/releases/:title/"
```

```liquid
{% for release in collections.releases %}
  <a href="{{ release.url }}">{{ release.title }}</a>
{% endfor %}
```
