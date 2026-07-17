---
layout: doc
title: Plugin System
nav_weight: 10
description: "Alloy's tiered plugin system: Go built-in filters, in-process QuickJS/WASM, and Node subprocess plugins for full ecosystem access."
---

Alloy's plugin system lets you extend the build pipeline with custom filters, shortcodes, and lifecycle hooks. Plugins are loaded automatically from the `plugins/` directory -- drop a file in and it is active.

```
plugins/
  word-count.js          # JS on embedded QuickJS (Tier 2)
  custom-slugify.wasm    # Compiled WASM (Tier 2)
  css-minifier.js        # Node bridge (Tier 3, exports runtime: "node")
```

No configuration required. To disable a plugin, remove or rename the file.

## Three-Tier Runtime

Alloy routes plugin execution to the appropriate tier based on what the plugin needs:

| Tier | Runtime | Latency | Use When |
|---|---|---|---|
| **Tier 1** | Go built-in | ~nanoseconds | Built-in filters (`slugify`, `date`, `upcase`, etc.) |
| **Tier 2** | In-process (QuickJS or WASM) | ~microseconds | Custom filters, shortcodes, data transforms -- pure computation |
| **Tier 3** | Node subprocess | ~milliseconds | npm packages, filesystem access, native addons |

### When to Use Each Tier

**Tier 2 -- QuickJS (JS plugins)** is the default for `.js` files. Write plain JavaScript, drop it in `plugins/`, and it works immediately. No build step, no Node dependency. Best for:

- Custom template filters
- Shortcodes
- Data transforms
- Anything that is pure computation

**Tier 2 -- WASM (compiled plugins)** gives maximum performance for hot-path operations. Compile from Rust, TinyGo, or AssemblyScript. Best for:

- Filters applied to every page (5-10x faster than QuickJS)
- Performance-critical transforms

**Tier 3 -- Node (subprocess plugins)** runs in a Node.js subprocess with full system access. Best for:

- PostCSS, Lightning CSS, cssnano (npm packages)
- Image optimization with Sharp (native addons)
- Lit SSR (Node VM module)
- External API calls, database queries

## Plugin API

All tiers share the same registration API:

```javascript
// plugins/my-plugin.js
export default function(alloy) {
  // Register a filter
  alloy.filter("wordCount", (content) => {
    return content.split(/\s+/).filter(w => w.length > 0).length;
  });

  // Register a shortcode
  alloy.shortcode("youtube", (args) => {
    const id = args[0];
    return `<iframe src="https://www.youtube.com/embed/${id}"
            frameborder="0" allowfullscreen></iframe>`;
  });

  // Register a lifecycle hook
  alloy.hook("onPageRendered", {}, (html) => {
    return html.replace(/\s+/g, ' ').trim();
  });
}
```

After registration, use your filter and shortcode in templates:

{% raw %}
<wa-tab-group>
<wa-tab slot="nav" panel="pluguse-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="pluguse-go">Go templates</wa-tab>

<wa-tab-panel name="pluguse-liquid" active>
<alloy-code lang="liquid">{{ page.content | wordCount }} words

{% youtube "dQw4w9WgXcQ" %}</alloy-code>
</wa-tab-panel>
<wa-tab-panel name="pluguse-go">
<alloy-code lang="html">{{ wordCount .page.content }} words

{{ youtube "dQw4w9WgXcQ" }}</alloy-code>
</wa-tab-panel>
</wa-tab-group>
{% endraw %}

## Site Data Access

Plugins can access global data files via `alloy.data`:

```javascript
export default function(alloy) {
  alloy.shortcode("statusTag", (args) => {
    const key = args[0];
    const legend = alloy.data.statusLegend;
    const entry = legend[key];
    return `<rh-tag color="${entry.color}">${entry.pretty}</rh-tag>`;
  });
}
```

`alloy.data` is a read-only snapshot of `site.data`, available inside filter, shortcode, and hook functions. Access it inside those functions, not at the top level of your plugin file (it is `undefined` during plugin evaluation).

To modify data that templates see, use hooks like `onDataFetched` or `onAfterValidation` -- see [Lifecycle Events](/hooks/).

## Load Order and Conflicts

Plugins load in alphabetical filename order. Tier 1 (Go built-in) filters register first, then Tier 2, then Tier 3.

If two plugins register the same filter or shortcode name, the last one loaded wins:

```
[alloy] WARN Filter "slugify" registered by plugins/custom-slugify.wasm
        overwrites built-in filter "slugify"
```

## Sandboxing

Tier 2 plugins (both QuickJS and WASM) run in isolated memory spaces via wazero. They cannot access the filesystem, network, or system resources. Safe to run community plugins.

Tier 3 plugins run with the same permissions as the user — full filesystem and network access. This is the same trust model as npm packages.

## Error Handling

Plugin errors surface clearly with the plugin name, filter/shortcode name, and input that caused the failure:

- **`alloy build`**: any plugin error aborts the build. Plugin timeouts produce a warning and continue with unmodified data.
- **`alloy dev`**: plugin errors show in the browser error overlay. Plugin crashes stop the server.

## Learn More

- [QuickJS Plugins](/plugins/quickjs/) -- embedded JS plugins with no build step
- [WASM Plugins](/plugins/wasm/) -- compiled plugins for maximum performance
- [Node Plugins](/plugins/node/) -- Node subprocess plugins with npm access
- [Lifecycle Events](/hooks/) -- all available hook events
