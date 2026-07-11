---
type: minor
---

Front matter permalinks can now use full template syntax with `{{ }}` expressions. When a permalink contains `{{`, it is rendered through the configured template engine (Liquid or Go templates) with a `page.*` context containing all front matter fields, date, slug, summary, and collection.

```yaml
# content/blog/hello.md
---
title: Hello World
slug: hello-world
lang: en
permalink: "/{{ page.lang }}/{{ page.slug }}/"
---
```

```yaml
# Go template engine (templates.engine: "gotemplate")
---
permalink: "/posts/{{ .page.title | slugify }}/"
---
```

Template and token syntax are separate modes. When `{{` is detected, token syntax (`:year`, `:slug`) is not resolved — the entire string is a template expression. A template permalink that renders to an empty or whitespace-only string is a fatal build error, distinct from `permalink: false` which is an intentional opt-out.

Pagination template permalinks now respect the configured engine. Previously, `engine: "gotemplate"` pagination pages fell back to Liquid for permalink rendering.
