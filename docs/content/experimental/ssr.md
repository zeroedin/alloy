---
layout: doc
title: "Config-Driven SSR"
nav_weight: 20
---

<aside>
This feature is experimental and may change. The <a href="/advanced/ssr/">plugin-based approach</a> is the recommended way to add SSR today.
</aside>

Alloy supports an `ssr:` config block that pipes rendered HTML through an external command via stdin/stdout:

```yaml
# alloy.config.yaml
ssr:
  command: "node ssr-worker.js"
  mode: "exec"
  timeout: 10000
```

With this config, every page containing custom element tags is sent through the configured command for server-side rendering.

## Exec vs stream modes

| Mode | Behavior | Best for |
|---|---|---|
| `exec` | Spawns a new process for each page | Stateless workers, maximum isolation |
| `stream` | Starts one persistent subprocess, pages piped sequentially | Faster builds, shared state between pages |

In `exec` mode, the command starts and stops for every page that contains custom elements. In `stream` mode, Alloy starts the command once and pipes pages through it one at a time. Stream mode avoids repeated startup costs but requires the worker to handle sequential input correctly.

## Timeout

The `ssr.timeout` value is in milliseconds. If the SSR command does not return output within the timeout, the build fails with an error identifying the page that timed out.

## SSR worker example

The SSR command can be any executable that reads HTML from stdin and writes transformed HTML to stdout:

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
