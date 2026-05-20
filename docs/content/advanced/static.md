---
layout: doc
title: Static Files and Passthrough
nav_weight: 30
---

Alloy provides three mechanisms for getting non-content files into your output directory: the `static/` directory, passthrough mappings, and content-colocated assets. Each serves a different use case.

```yaml
# alloy.config.yaml
passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
  - from: "../shared-assets/fonts"
    to: "assets/fonts"
```

## Static Directory

Files in `static/` are copied to the output root as-is. No template rendering, no fingerprinting, no transformation.

```
static/
  favicon.ico          -> _site/favicon.ico
  robots.txt           -> _site/robots.txt
  images/
    logo.png           -> _site/images/logo.png
  downloads/
    whitepaper.pdf     -> _site/downloads/whitepaper.pdf
```

Use `static/` for files that must appear at exact paths: favicons, `robots.txt`, verification files, and downloadable assets.

## Passthrough Copy

Passthrough mappings copy files from outside the project (or from non-standard locations inside it) into the output directory:

```yaml
passthrough:
  # Design system dist files
  - from: "../design-system/dist/elements"
    to: "elements"                           # -> _site/elements/

  # Fonts from a shared repo
  - from: "../shared-assets/fonts"
    to: "assets/fonts"                       # -> _site/assets/fonts/

  # Internal directory
  - from: "vendor/js"
    to: "js/vendor"                          # -> _site/js/vendor/
```

`from` paths are resolved relative to the project root. Absolute paths are also supported.

### Glob Patterns

The `from` field accepts glob patterns (`**`, `{a,b}`, `[chars]`) for fine-grained file selection:

```yaml
passthrough:
  # Only .js and .css files
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/elements"

  # Only .woff2 font files
  - from: "../shared-assets/fonts/**/*.woff2"
    to: "assets/fonts"
```

The output path preserves directory structure relative to the glob root (the longest static prefix before any metacharacter). For `elements/**/*.js`, the glob root is `elements`. A file at `elements/rh-button/rh-button.js` copies to `_site/assets/packages/elements/rh-button/rh-button.js`.

### Exclude Patterns

The `exclude` array filters out files using gitignore-style patterns:

```yaml
passthrough:
  # Copy everything except demos and sourcemaps
  - from: "elements"
    to: "assets/packages/elements"
    exclude:
      - "*.html"         # any .html file at any depth
      - "demo/"          # entire demo/ directory tree
      - "**/*.map"       # any .map file at any depth
```

Exclude patterns follow gitignore normalization rules:

| Pattern | Normalized Form | Matches |
|---|---|---|
| `*.html` | `**/*.html` | `foo.html`, `sub/bar.html` |
| `demo/` | `demo/**` | `demo/index.html`, `demo/sub/file.js` |
| `demo/*.html` | `demo/*.html` | `demo/foo.html` (not `demo/sub/bar.html`) |
| `**/*.map` | `**/*.map` | `foo.map`, `sub/bar.map` |

Patterns without `/` match filenames at any depth. Patterns ending with `/` match entire directory trees. Patterns containing `/` match against the relative path as-is.

### Combining Glob and Exclude

```yaml
passthrough:
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/elements"
    exclude:
      - "*.min.js"       # skip minified JS
      - "*.test.js"      # skip test files
```

### Managed Directory Protection

If a passthrough `from:` path resolves to a managed directory (`content`, `layouts`, `assets`, `static`, `data`), it is silently ignored. Those directories are already processed by the pipeline.

## Content-Colocated Assets

Files in the `content/` directory whose extension does not match `content.formats` (default: `md`, `html`) are automatically copied to the output, preserving their path:

```
content/about/
  index.md              -> processed as content
  diagram.svg           -> _site/about/diagram.svg
  hero.png              -> _site/about/hero.png
```

This enables colocating assets with the content that uses them:

```markdown
---
title: "About Us"
---

# About Us

![Team photo](hero.png)

{% inline "./diagram.svg" %}
```

### What Gets Copied

Files excluded from content-colocated passthrough:

- `_data.yaml` / `_data.yml` (cascade data files)
- Dot-prefixed files (`.DS_Store`, `.gitkeep`, etc.)
- Directories
- Files matching `content.formats` (those are content pages)

### HTML File Classification

`.html` files in the content directory follow a three-way classification:

| Condition | Treatment |
|---|---|
| Has front matter (`---`, `+++`, `{`) | Content page, processed normally |
| No front matter + `<!DOCTYPE` or `<html>` | Passthrough, copied as-is |
| No front matter + no DOCTYPE | HTML fragment, treated as content with empty front matter |

## Build vs Dev Mode

**`alloy build`**: static and passthrough files are copied to `_site/`. File copies use a bounded worker pool for performance.

**`alloy dev`**: static and passthrough files are served directly from their source locations. No copy needed -- the Go HTTP server maps URL paths to source directories. Changes are reflected instantly.

**`alloy serve`**: files are copied to `_site/` (like build mode). The file watcher monitors passthrough source directories. On change, only the modified file is recopied -- not the entire directory.

## Watch Directories

Plugin filters may read files from directories outside the standard content tree. Register extra directories for pipeline-triggering watches:

```yaml
watch:
  - from: "elements"
    type: content
  - from: "shared-layouts"
    type: layout
  - from: "external-data"
    type: data
```

Changes in watch directories trigger pipeline rebuilds (not just file recopies like passthrough). The `type` field determines what kind of rebuild occurs:

| Type | Effect |
|---|---|
| `content` | Content change -- rebuild affected pages |
| `layout` | Layout change -- rebuild pages using affected layouts |
| `data` | Data change -- rebuild pages that could be affected |

Watch directories are checked before passthrough directories during change classification. A directory in both `watch:` and `passthrough:` uses the watch classification.

## Output Path Conflicts

If two sources target the same output path, the build fails immediately:

```
[alloy] ERROR Output path conflict detected:
        _site/elements/button.js is claimed by:
          1. static/elements/button.js
          2. passthrough "../design-system/dist/elements" -> "elements"

        Resolve by renaming one source or adjusting the passthrough "to" path.
        Build aborted.
```

There is no priority system. Conflicts must be resolved explicitly.

## Custom Directory Structure

Override the default directories in your config:

```yaml
structure:
  content: "./docs/pages/"
  layouts: "./docs/layouts/"
  assets: "./docs/assets/"
  static: "./docs/static/"
  data: "./data/"
```

All paths are relative to the project root. The pipeline, file watcher, and dev server all use the configured paths.

## Related

- [Internationalization](/advanced/i18n/) -- per-language content trees
- [Plugin System](/plugins/) -- `onAssetProcess` hook for asset transformation
- [Lifecycle Events](/hooks/) -- `onBeforeValidation` for registering additional output paths
