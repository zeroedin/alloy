---
layout: doc
title: Passthrough Copy
nav_weight: 30
---

Passthrough mappings copy directories and files into the output directory without processing them:

```yaml
passthrough:
  # Internal directory
  - from: "vendor/js"
    to: "js/vendor"                          # -> _site/js/vendor/

  # Monorepo sibling package
  - from: "../design-system/dist/elements"
    to: "elements"                           # -> _site/elements/
```

`from` paths are resolved relative to the project root. Relative paths that reach outside the project (like `../`) and absolute paths are supported but not recommended -- the referenced files must exist in the build environment, and paths that work locally can break in CI where the repo is checked out in isolation.

## Glob Patterns

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

## Exclude Patterns

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

## Combining Glob and Exclude

```yaml
passthrough:
  - from: "elements/**/*.{js,css}"
    to: "assets/packages/elements"
    exclude:
      - "*.min.js"       # skip minified JS
      - "*.test.js"      # skip test files
```

## Managed Directory Protection

If a passthrough `from:` path resolves to a managed directory (`content`, `layouts`, `assets`, `static`, `data`, `plugins`, `.alloy`), it is silently ignored. Those directories are already processed by the pipeline. This check respects custom paths set in `structure:`.

## Watch Directories

Watch directories register external directories for pipeline-triggering rebuilds during dev and serve modes. Use `watch:` when a directory contains files your plugins or templates depend on, where a change should trigger a **page rebuild** rather than a simple file copy:

```yaml
watch:
  - from: "elements"
    type: content
  - from: "shared-layouts"
    type: layout
  - from: "external-data"
    type: data
```

The `type` field determines what kind of rebuild occurs:

| Type | Effect |
|---|---|
| `content` | Content change -- rebuild affected pages |
| `layout` | Layout change -- rebuild pages using affected layouts |
| `data` | Data change -- rebuild pages that could be affected |

A directory should not appear in both `watch:` and `passthrough:`. If it does, the watch classification takes precedence and the passthrough recopy behavior is skipped.

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

## Build vs Dev Mode

**`alloy build`**: passthrough files are copied to `_site/`. File copies use a bounded worker pool for performance.

**`alloy dev`** and **`alloy serve`**: passthrough files are copied to `_site/` during the initial build. The file watcher monitors passthrough source directories. On change, only the modified file is recopied -- not the entire directory.
