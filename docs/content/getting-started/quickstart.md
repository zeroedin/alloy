---
title: "Quickstart"
layout: "doc"
weight: 2
section: "getting-started"
description: "Build a complete site with Alloy in 5 minutes."
---

## Create the Project

```bash
mkdir my-blog && cd my-blog
alloy init
```

## Add Content

Create `content/index.md`:

```markdown
---
title: "Home"
---

# Welcome to My Blog

This is my first Alloy site.
```

Create `content/blog/first-post.md`:

```markdown
---
title: "My First Post"
date: 2026-04-22
tags: ["hello", "alloy"]
---

# Hello World

This is my first blog post built with Alloy.
```

## Add a Layout

Create `layouts/default.liquid`:

```liquid
<!DOCTYPE html>
<html>
<head><title>{{ page.title }}</title></head>
<body>
  <nav><a href="/">Home</a></nav>
  {{ content }}
</body>
</html>
```

## Add Directory Data

Create `content/blog/_data.yaml` to set defaults for all blog posts:

```yaml
layout: "post"
```

Create `layouts/post.liquid`:

```liquid
<!DOCTYPE html>
<html>
<head><title>{{ page.title }}</title></head>
<body>
  <nav><a href="/">Home</a></nav>
  <article>
    <h1>{{ page.title }}</h1>
    <time>{{ page.date | date: "%B %d, %Y" }}</time>
    {{ content }}
  </article>
</body>
</html>
```

## Build and Serve

```bash
alloy serve
```

Open `http://localhost:3000`. Edit a file — the browser reloads automatically.

Build for production:

```bash
alloy build
```

Your site is in `_site/`, ready to deploy.

## Next Steps

- [Project Structure](/getting-started/project-structure/) — What each directory does
- [Configuration](/configuration/) — Customize permalinks, collections, and more
- [Templates](/templates/) — Liquid syntax, filters, and layouts
