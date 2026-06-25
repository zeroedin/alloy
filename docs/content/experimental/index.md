---
layout: doc
title: Experimental Features
nav_weight: 10
description: "Functional but evolving features in Alloy, including config-driven SSR, with APIs that may change between releases."
---

These features are functional but their APIs may change in future releases. Use them in production with the understanding that configuration keys, CLI flags, or behavior details could shift between versions.

## Server-Side Rendering

Alloy can pipe your HTML through an external SSR command to expand custom elements into Declarative Shadow DOM. This runs as a second phase after content rendering, so your templates produce standard HTML first, then SSR transforms it. See [Server-Side Rendering](/experimental/ssr/).
