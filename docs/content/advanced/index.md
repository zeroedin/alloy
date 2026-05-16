---
layout: doc
title: Advanced Features
---

Alloy handles most sites with content, templates, and plugins alone. These features cover specialized needs: rendering web components on the server, building multilingual sites, and controlling how static assets reach the output directory.

## Server-Side Rendering

Alloy can pipe your HTML through an external SSR command to expand custom elements into Declarative Shadow DOM. This runs as a second phase after content rendering, so your templates produce standard HTML first, then SSR transforms it. See [Server-Side Rendering](/advanced/ssr/).

## Internationalization

The `languages:` config key activates multi-language support. Each language gets its own content tree, output directory, and translation strings accessible in templates. Collections and taxonomies are scoped per-language automatically. See [Internationalization](/advanced/i18n/).

## Static Files and Passthrough

Files in `static/` are copied to the output root as-is. For assets outside your project tree, passthrough copy maps external directories into the output with glob pattern support. In dev mode, these files are served directly from source without copying. See [Static Files and Passthrough](/advanced/static/).
