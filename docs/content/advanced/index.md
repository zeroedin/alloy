---
layout: doc
title: Advanced Features
nav_weight: 10
---

Alloy includes several advanced features for complex site architectures. This section covers capabilities beyond the core content-to-HTML pipeline.

## Internationalization

Build multilingual sites with per-language content trees, shared layouts, and automatic translation linking:

```yaml
# alloy.config.yaml
languages:
  en:
    title: "My Site"
    weight: 1
    root: true
  fr:
    title: "Mon Site"
    weight: 2
```

Each language gets its own content directory and output prefix. Layouts are shared. See [Internationalization](/advanced/i18n/) for the full guide.

## Static Files and Passthrough

Control how non-content files reach the output directory. Static files are copied as-is. Passthrough mappings bring in files from outside your project (monorepo packages, shared assets). Glob patterns and exclude filters control what gets copied.

```yaml
# alloy.config.yaml
passthrough:
  - from: "../design-system/dist/elements"
    to: "elements"
  - from: "../shared-assets/fonts/**/*.woff2"
    to: "assets/fonts"
    exclude:
      - "*.map"
```

See [Static Files and Passthrough](/advanced/static/) for details.
