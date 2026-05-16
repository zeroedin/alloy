---
layout: doc
title: Quickstart
---

Build a blog with Alloy in 5 minutes.

## Create the project

```bash
mkdir my-blog && cd my-blog
```

Create `alloy.config.yaml`:

```yaml
title: "My Blog"
baseURL: "https://example.com"
build:
  output: "_site"
```

## Add a home page

Create `content/index.md`:

```markdown
---
title: Home
layout: default
---

# Welcome to my blog

This is the home page.
```

## Create a layout

Layouts wrap your content in HTML. Create `layouts/default.liquid`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{ page.title }}</title>
</head>
<body>
  <nav>
    <a href="/">Home</a>
    <a href="/blog/">Blog</a>
  </nav>
  <main>
    {{ content }}
  </main>
</body>
</html>
```

The `{{ content }}` tag is replaced with the rendered page body.

## Write a blog post

Create `content/blog/hello-world.md`:

```markdown
---
title: Hello World
date: 2024-01-15
layout: default
---

This is my first blog post built with Alloy.
```

## Set directory defaults

Instead of repeating `permalink` in every post, create `content/blog/_data.yaml`:

```yaml
permalink: "/blog/:slug/"
```

Every page in `content/blog/` inherits this permalink pattern. The `:slug` token is derived from the filename.

## Build the site

```bash
alloy build
```

Alloy writes the output to `_site/`:

```
_site/
├── index.html
└── blog/
    └── hello-world/
        └── index.html
```

## Live preview

Start the development server for live rebuilds:

```bash
alloy serve
```

Open `http://localhost:8080` in your browser. Edits to content and templates trigger automatic rebuilds.

## Next steps

- [Project Structure](/getting-started/project-structure/) — understand the directory layout
- [Data Cascade](/content/data-cascade/) — directory-level defaults and deep merge
- [Plugins](/plugins/) — extend Alloy with QuickJS, WASM, or Node
