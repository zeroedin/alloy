---
layout: doc
title: Advanced Features
nav_weight: 10
---

This section covers capabilities beyond the core content-to-HTML pipeline.

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

## Server-Side Rendering

Web components can be server-rendered into Declarative Shadow DOM at build time using Alloy's plugin system. See [SSR](/advanced/ssr/) for details.
