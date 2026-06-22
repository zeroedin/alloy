---
layout: doc
title: Quickstart
nav_weight: 20
---

Build a working blog with Alloy in 5 minutes. By the end, you will have a site with a homepage, two blog posts, a shared layout, and a local dev server.

## 1. Create the project

```bash
alloy init my-blog && cd my-blog
```

This scaffolds a complete starter project with a config file, directories, a default layout, an index page, and a stylesheet. Open `alloy.config.yaml` and update the title, then add a taxonomies block for tags:

```yaml
# alloy.config.yaml
title: "My Blog"
baseURL: "http://localhost:3000"

taxonomies:
  tags:
```

## 2. Customize the layout

`alloy init` creates a `layouts/default.liquid` with a minimal HTML5 boilerplate. Open it and replace it with a layout that includes a header and navigation:

```liquid
<!-- layouts/default.liquid -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{ page.title }} | {{ site.title }}</title>
</head>
<body>
  <header>
    <a href="/">{{ site.title }}</a>
  </header>
  <main>
    {{ content }}
  </main>
</body>
</html>
```

Create a layout for blog posts. Children of a section with date-based permalinks automatically resolve to `post.liquid`:

```liquid
<!-- layouts/post.liquid -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{ page.title }} | {{ site.title }}</title>
</head>
<body>
  <header>
    <a href="/">{{ site.title }}</a>
  </header>
  <main>
    <article>
      <h1>{{ page.title }}</h1>
      <time>{{ page.date | date: "%B %d, %Y" }}</time>
      {% if page.tags %}
        <ul class="tags">
          {% for tag in page.tags %}
            <li>{{ tag }}</li>
          {% endfor %}
        </ul>
      {% endif %}
      {{ content }}
    </article>
  </main>
</body>
</html>
```

## 3. Add content

Create the content directory and a homepage:

```bash
mkdir -p content/blog
```

```markdown
<!-- content/index.md -->
---
title: "Home"
---

# Welcome to my blog

Check out my latest posts.
```

Create a `_data.yaml` file in the blog directory to set a date-based permalink pattern. This turns the blog directory into a collection:

```yaml
# content/blog/_data.yaml
permalink: "/blog/:year/:month/:slug/"
```

Now add two blog posts:

```markdown
<!-- content/blog/hello-world.md -->
---
title: "Hello World"
date: 2026-01-15
tags: ["introduction"]
---

This is my first post on Alloy. The build is fast and the templates are familiar.
```

```markdown
<!-- content/blog/liquid-templates.md -->
---
title: "Working with Liquid Templates"
date: 2026-01-20
tags: ["tutorials", "liquid"]
---

Alloy uses Liquid for templates. If you've worked with Liquid before, the syntax is identical.

## Filters

Liquid filters transform output:

- `{{ "hello world" | upcase }}` outputs `HELLO WORLD`
- `{{ page.date | date: "%Y-%m-%d" }}` formats dates
- `{{ page.title | slugify }}` creates URL-safe slugs
```

## 4. Build the site

```bash
alloy build
```

```
[alloy] Built 3 pages in 24ms
```

Alloy writes output to `_site/`:

```
_site/
├── index.html
└── blog/
    └── 2026/
        └── 01/
            ├── hello-world/
            │   └── index.html
            └── liquid-templates/
                └── index.html
```

## 5. Start the dev server

```bash
alloy dev
```

```
[alloy] Built 3 pages in 24ms
Serving at http://localhost:3000
```

Open `http://localhost:3000` in your browser. Edit any content or template file -- Alloy rebuilds incrementally and reloads the page.

The dev server includes draft content by default. Add `draft: true` to a post's front matter to hide it from production builds while keeping it visible during development.

## What you built

Your project now looks like this:

```
my-blog/
├── alloy.config.yaml
├── content/
│   ├── index.md
│   └── blog/
│       ├── _data.yaml
│       ├── hello-world.md
│       └── liquid-templates.md
├── layouts/
│   ├── default.liquid
│   └── post.liquid
└── _site/                 # generated output
```

## Next steps

- [Project Structure](/getting-started/project-structure/) -- Full directory layout reference
- [CLI Reference](/cli/) -- All commands and flags
- [Content](/content/) -- Front matter formats, drafts, summaries, and table of contents
- [Templates](/templates/) -- Partials, shortcodes, layout chaining, and filters
- [Collections](/collections/) -- Taxonomies and section collections
