---
layout: doc
title: Static Files and Passthrough
---

Files that should reach the output without template processing or Markdown rendering are handled through three mechanisms: the `static/` directory, passthrough copy, and content-colocated assets.

## The static/ directory

Everything in `static/` is copied to the output root as-is, preserving directory structure:

```
static/
├── css/
│   └── main.css
├── js/
│   └── app.js
└── favicon.ico
```

Produces:

```
_site/
├── css/
│   └── main.css
├── js/
│   └── app.js
└── favicon.ico
```

No processing, no renaming. Reference these files with root-relative paths in your templates: `/css/main.css`, `/js/app.js`.

## Passthrough copy

Passthrough copy maps directories or file patterns from outside (or inside) your project into the output. Configure it in `alloy.config.yaml`:

```yaml
passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
```

This copies the contents of `../design-system/dist/elements/` into `_site/elements/`. Use it for monorepo dependencies, build artifacts from other tools, or any files that live outside the standard `static/` directory.

### Glob patterns

The `from` field supports glob patterns for selective copying:

```yaml
passthrough:
  - from: "elements/**/*.{js,css}"
    to: "elements"
```

This copies only `.js` and `.css` files from the `elements/` tree, ignoring everything else.

### Exclude patterns

Use gitignore-style exclusion patterns to skip files:

```yaml
passthrough:
  - from: "../design-system/dist"
    to: "vendor"
    exclude:
      - "*.map"
      - "test/**"
      - "*.d.ts"
```

Exclude patterns follow gitignore syntax: `*` matches within a path segment, `**` matches across segments, and leading `/` anchors to the `from` root.

## Content-colocated assets

Files in `content/` whose extension does not match the configured content formats (`md` and `html` by default) are copied to the output alongside their content pages:

```
content/about/
├── index.md         # content — processed through the pipeline
├── team-photo.jpg   # passthrough — copied to _site/about/team-photo.jpg
└── org-chart.pdf    # passthrough — copied to _site/about/org-chart.pdf
```

Reference colocated assets with relative paths in your Markdown:

```markdown
![Team photo](team-photo.jpg)

Download the [org chart](org-chart.pdf).
```

This keeps assets next to the content that uses them, avoiding a separate `static/images/` directory for page-specific files.

## Build vs dev behavior

Static files and passthrough copy behave differently depending on the command:

| Command | Behavior |
|---|---|
| `alloy build` | Files are copied into `_site/` |
| `alloy serve` | Files are copied into `_site/` |
| `alloy dev` | Files are served directly from their source location (no copy) |

In dev mode, Alloy serves static and passthrough files from their original paths without writing them to the output directory. This avoids redundant I/O and keeps the dev cycle fast. The production build (`alloy build`) always performs a full copy.

## Watch directories

By default, Alloy watches `content/`, `layouts/`, `static/`, and `plugins/` for changes during `serve` and `dev`. If you have passthrough sources or other external directories that should trigger rebuilds, add them to `watch:`:

```yaml
watch:
  - "../design-system/dist"
  - "../shared-components/src"
```

When a file changes in a watched directory, Alloy triggers a rebuild. This is useful for monorepo setups where component libraries or shared assets are built by a separate process.
