---
layout: doc
title: Assets
nav_weight: 10
description: "How Alloy moves non-content files into your build output: static assets, passthrough copies, and colocated files."
---

Alloy provides several mechanisms for getting non-content files into your output directory. This section covers static files, passthrough mappings, and content-colocated assets.

## Static Directory

Files in `static/` are copied to the output root as-is. No template rendering, no fingerprinting, no transformation. Use it for files that must appear at exact paths: favicons, `robots.txt`, verification files, and downloadable assets.

See [Static Files](/assets/static/) for details.

## Passthrough Copy

Passthrough mappings copy files from outside the project (or from non-standard locations inside it) into the output directory. Glob patterns and exclude filters control what gets copied.

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

See [Passthrough Copy](/assets/passthrough/) for details.

## Content-Colocated Assets

Files in the `content/` directory whose extension does not match `content.formats` are automatically copied to the output, preserving their path. This enables colocating images and other assets with the content that uses them.

See [Colocated Assets](/assets/colocated/) for details.
