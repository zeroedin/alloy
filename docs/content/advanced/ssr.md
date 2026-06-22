---
layout: doc
title: Server-Side Rendering
---

Web components can be server-rendered into Declarative Shadow DOM at build time using Alloy's plugin system. The `onPageRendered` hook gives plugins access to each page's final HTML, making it possible to transform custom elements before they're written to disk.

## How it works

1. Alloy completes its normal build -- Markdown is parsed, templates are evaluated, layouts are applied. The result is complete HTML.
2. The SSR plugin receives each page's HTML via the `onPageRendered` hook.
3. The plugin checks for custom element tags (any tag with a hyphen). Pages without custom elements are returned unchanged.
4. Pages with custom elements are rendered through your SSR engine. The plugin returns the transformed HTML with Declarative Shadow DOM markup.

## Writing an SSR plugin

An SSR plugin is a Node runtime plugin that hooks into `onPageRendered`. It lazy-loads your component definitions and SSR engine on first use, then transforms each page's HTML.

```js
// plugins/lit-ssr.js
export const runtime = "node";

export default function(alloy) {
  let renderLit, litHtml, collectResult;

  async function ensureLoaded() {
    if (renderLit) return;

    // Load SSR dependencies
    const ssrMod = await import('@lit-labs/ssr');
    renderLit = ssrMod.render;
    const collectMod = await import('@lit-labs/ssr/lib/render-result.js');
    collectResult = collectMod.collectResult;
    const litMod = await import('lit');
    litHtml = litMod.html;

    // Load your component definitions
    await import('./components/my-header.js');
    await import('./components/my-nav.js');
  }

  // UnsafeHTMLStringsArray lets Lit treat raw HTML as a tagged template
  class UnsafeHTMLStringsArray extends Array {
    raw;
    constructor(string) {
      super();
      this.push(string);
      this.raw = [string];
    }
  }

  alloy.hook("onPageRendered", async (html) => {
    if (typeof html !== 'string') return html;
    if (!/<[a-z]+-[a-z]/.test(html)) return html;

    await ensureLoaded();

    try {
      const tpl = litHtml(new UnsafeHTMLStringsArray(html));
      const result = renderLit(tpl);
      return await collectResult(result);
    } catch (e) {
      console.error(`[lit-ssr] SSR failed: ${e.message}`);
      return html;
    }
  });
}
```

The plugin:

- **Uses `runtime: "node"`** -- SSR engines like `@lit-labs/ssr` require a full Node environment.
- **Lazy-loads dependencies** -- Component definitions and the SSR engine are loaded once on the first page that needs them, not at startup.
- **Skips pages without custom elements** -- The regex check avoids unnecessary SSR overhead.
- **Falls back gracefully** -- If SSR fails on a page, the original HTML is returned and the error is logged.

The output should contain Declarative Shadow DOM markup -- `<template shadowrootmode="open">` blocks inside the custom elements -- so browsers can hydrate them without JavaScript on first paint.

## Experimental: config-driven SSR

Alloy also has an experimental `ssr:` config block that pipes rendered HTML through an external command via stdin/stdout, without requiring a plugin. See [Config-Driven SSR](/experimental/ssr/) for details.
