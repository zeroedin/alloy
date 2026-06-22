---
layout: doc
title: Static Files
nav_weight: 20
---

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

## Custom Directory

Override the default static directory in your config:

```yaml
structure:
  static: "./docs/static/"
```

The path is relative to the project root.

## Build vs Dev Mode

**`alloy build`**: static files are copied to `_site/`.

**`alloy dev`** and **`alloy serve`**: static files are copied to `_site/` during the initial build. The file watcher monitors the static directory and recopies modified files.
