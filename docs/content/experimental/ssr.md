---
layout: doc
title: Server-Side Rendering
nav_weight: 20
---

Alloy can expand custom elements into Declarative Shadow DOM by piping rendered HTML through an external command. SSR is opt-in -- without the `ssr:` config block, Phase 1 output is final HTML.

```yaml
# alloy.config.yaml
ssr:
  command: "node ssr-worker.js"
  mode: "exec"
  timeout: 10000
```

With this config, every page containing custom element tags (any tag with a hyphen, like `<my-header>` or `<app-nav>`) is sent through `ssr-worker.js` for server-side rendering.

## How it works

Alloy's build runs in two phases:

1. **Phase 1** -- Content rendering. Markdown is parsed, templates are evaluated, layouts are applied. The result is complete HTML.
2. **Phase 2** -- SSR. Alloy scans each Phase 1 output for custom element tags. Pages with custom elements have their full HTML piped to the configured `ssr.command` via **stdin**. The command writes transformed HTML to **stdout**. Alloy replaces the page output with the result.

Pages without custom elements skip Phase 2 entirely.

## Exec vs stream modes

The `ssr.mode` setting controls how the SSR command is invoked:

| Mode | Behavior | Best for |
|---|---|---|
| `exec` | Spawns a new process for each page | Stateless workers, maximum isolation |
| `stream` | Starts one persistent subprocess, pages piped sequentially | Faster builds, shared state between pages |

```yaml
ssr:
  command: "node ssr-worker.js"
  mode: "stream"    # one long-lived process
  timeout: 10000
```

In `exec` mode, the command starts and stops for every page that contains custom elements. In `stream` mode, Alloy starts the command once and pipes pages through it one at a time. Stream mode avoids repeated startup costs but requires the worker to handle sequential input correctly.

## Timeout

The `ssr.timeout` value is specified in milliseconds. If the SSR command does not return output within the timeout, the build fails with an error identifying the page that timed out.

```yaml
ssr:
  command: "node ssr-worker.js"
  mode: "exec"
  timeout: 5000    # 5 seconds per page
```

## Custom element detection

Alloy identifies custom elements by scanning the Phase 1 HTML for tags whose names contain a hyphen. This follows the HTML spec requirement that custom element names must contain a hyphen character. Standard HTML tags (`<div>`, `<header>`, `<section>`) are ignored.

If a page has no custom element tags, it is never sent to the SSR command.

## Component tracking

Alloy tracks which custom elements appear on which pages. This mapping is stored in `.alloy/components.json`:

```json
{
  "my-header": ["index.html", "about/index.html"],
  "app-nav": ["index.html", "blog/index.html"]
}
```

When a component definition changes, Alloy uses this mapping to invalidate only the pages that use that component. Combined with Phase 2 output hashing (which skips writing pages whose SSR output has not changed), this keeps incremental rebuilds fast.

## Compatible engines

The SSR command can be any executable that reads HTML from stdin and writes transformed HTML to stdout. Two engines are tested and recommended:

- **golit** (recommended) -- A Go-native Lit SSR renderer. No Node dependency, fast startup, works well in both exec and stream modes.
- **lit-ssr-wasm** -- Lit's official SSR running in a WASM context. Compatible with Alloy's stdin/stdout protocol.

## Writing an SSR worker

An SSR worker reads HTML from stdin, renders custom elements, and writes the result to stdout. Here is a minimal Node.js example:

```js
// ssr-worker.js
import { render } from '@lit-labs/ssr';
import { html } from 'lit';

let input = '';
process.stdin.on('data', (chunk) => { input += chunk; });
process.stdin.on('end', async () => {
  const result = await render(input);
  process.stdout.write(result);
});
```

The output should contain Declarative Shadow DOM markup -- `<template shadowrootmode="open">` blocks inside the custom elements -- so browsers can hydrate them without JavaScript on first paint.

## Enabling SSR from the CLI

SSR can also be enabled with the `--ssr` flag, which is useful for toggling SSR in CI without changing the config file:

```bash
alloy build --ssr
```

The flag works alongside the config. If both are present, the config values are used for `command`, `mode`, and `timeout`.
