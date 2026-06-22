---
layout: doc
title: Shortcodes
nav_weight: 40
---

Shortcodes are reusable content snippets that accept arguments and output HTML. They let you embed rich elements in Markdown content without writing raw HTML.

```liquid
{% youtube "dQw4w9WgXcQ" %}
```

```html
<!-- Output -->
<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ"
        frameborder="0" allowfullscreen></iframe>
```

Shortcodes are registered via [plugins](/plugins/) and used in content files as custom Liquid tags (or Go template functions).

## Using shortcodes in content

### Inline shortcodes

Inline shortcodes take positional arguments and produce self-contained output:

```liquid
{% youtube "dQw4w9WgXcQ" %}
{% github_star "alloy-ssg/alloy" %}
```

### Block shortcodes

Block shortcodes wrap inner content, letting you add markup around authored text:

```liquid
{% callout "warning" %}
  Do not deploy to production without running the test suite first.
{% endcallout %}
```

```html
<!-- Output -->
<div class="callout callout--warning">
  Do not deploy to production without running the test suite first.
</div>
```

### Go template syntax

With the Go template engine, shortcodes are registered as template functions:

```html
{{ youtube "dQw4w9WgXcQ" }}
{{ callout "warning" "Do not deploy to production without running the test suite first." }}
```

## Registering shortcodes

Shortcodes are defined in plugin files placed in the `plugins/` directory. No configuration is needed -- drop a file in `plugins/` and its shortcodes are immediately available in all content files.

### JS plugin (Tier 2 -- in-process)

The simplest way to define shortcodes. JS plugins run on embedded QuickJS with no build step:

```javascript
// plugins/shortcodes.js
export default function(alloy) {
  alloy.shortcode("youtube", (args) => {
    const id = args[0];
    return `<iframe src="https://www.youtube.com/embed/${id}"
            frameborder="0" allowfullscreen></iframe>`;
  });

  alloy.shortcode("callout", (args, content) => {
    const level = args[0];
    return `<div class="callout callout--${level}">${content}</div>`;
  });
}
```

The first argument to `alloy.shortcode()` is the tag name. The callback receives an `args` array of positional arguments. Block shortcodes receive a second `content` parameter containing the inner content.

### Node plugin (Tier 3 -- full Node.js access)

Use Tier 3 when your shortcode needs npm packages, filesystem access, or network calls:

```javascript
// plugins/code-highlight.js
export const runtime = "node";
import prism from 'prismjs';

export default function(alloy) {
  alloy.shortcode("highlight", (args, content) => {
    const language = args[0] || "text";
    const html = prism.highlight(content, prism.languages[language], language);
    return `<pre class="language-${language}"><code>${html}</code></pre>`;
  });
}
```

Tier 3 plugins must have `"type": "module"` in the project's `package.json`.

### WASM plugin (Tier 2 -- compiled)

For maximum performance, compile shortcodes to WASM from Rust, TinyGo, or AssemblyScript:

**Rust:**

```rust
// plugins/shortcodes.rs (compile with wasm-pack)
use alloy_plugin::*;

#[alloy_shortcode("youtube")]
fn youtube(args: Vec<&str>) -> String {
    let id = args[0];
    format!(
        r#"<iframe src="https://www.youtube.com/embed/{}"
        frameborder="0" allowfullscreen></iframe>"#,
        id
    )
}

#[alloy_shortcode("callout")]
fn callout(args: Vec<&str>, content: &str) -> String {
    let level = args[0];
    format!(r#"<div class="callout callout--{}">{}</div>"#, level, content)
}
```

**TinyGo:**

```go
// plugins/shortcodes.go (compile with TinyGo)
package main

import "fmt"

//export register
func register(alloy *Alloy) {
    alloy.Shortcode("youtube", func(args []string) string {
        return fmt.Sprintf(
            `<iframe src="https://www.youtube.com/embed/%s"
            frameborder="0" allowfullscreen></iframe>`,
            args[0],
        )
    })

    alloy.Shortcode("callout", func(args []string, content string) string {
        return fmt.Sprintf(
            `<div class="callout callout--%s">%s</div>`,
            args[0], content,
        )
    })
}
```

JS plugins run at ~10-50 microseconds per call. Compiled WASM runs at ~1-10 microseconds per call. Choose based on whether you need the simplicity of plain JS or the performance of compiled code.

## Accessing site data

Shortcode plugins can access global data from `data/` files via `alloy.data`:

```javascript
// plugins/status-tag.js
export default function(alloy) {
  alloy.shortcode("statusTag", (args) => {
    const key = args[0];
    const legend = alloy.data.statusLegend;  // from data/statusLegend.yaml
    const entry = legend[key];
    return `<rh-tag color="${entry.color}" icon="${entry.icon}">${entry.pretty}</rh-tag>`;
  });
}
```

```liquid
{% statusTag "beta" %}
```

`alloy.data` is a read-only snapshot of `site.data` injected after data files are loaded. Access it inside shortcode functions, not at the top level of the plugin file -- top-level access during evaluation returns `undefined`.

## Practical examples

### Responsive image shortcode

```javascript
// plugins/responsive-image.js
export default function(alloy) {
  alloy.shortcode("image", (args) => {
    const src = args[0];
    const alt = args[1] || "";
    return `
      <figure>
        <img src="${src}" alt="${alt}" loading="lazy" decoding="async">
        ${alt ? `<figcaption>${alt}</figcaption>` : ""}
      </figure>`;
  });
}
```

```liquid
{% image "/img/hero.jpg" "A sunset over the mountains" %}
```

### Admonition block shortcode

```javascript
// plugins/admonition.js
export default function(alloy) {
  alloy.shortcode("note", (args, content) => {
    return `<div class="admonition admonition--note">
      <p class="admonition-title">Note</p>
      <div>${content}</div>
    </div>`;
  });

  alloy.shortcode("tip", (args, content) => {
    return `<div class="admonition admonition--tip">
      <p class="admonition-title">Tip</p>
      <div>${content}</div>
    </div>`;
  });
}
```

```liquid
{% note %}
  Remember to run `alloy build` before deploying.
{% endnote %}

{% tip %}
  Use `alloy dev` during local development for live reloading.
{% endtip %}
```

## Name conflicts

If two plugins register the same shortcode name, the last one loaded wins. Plugins load in alphabetical filename order within `plugins/`: built-in Go functions first, then Tier 2 (`.js` and `.wasm`), then Tier 3 (`.js` with `runtime: "node"`).

Alloy logs a warning when a name collision occurs so you know which plugin took precedence.

## Related

- [Filters](/templates/filters/) -- transform values in template expressions
- [Plugins](/plugins/) -- plugin system overview and tiers
- [Templates Overview](/templates/) -- template engine basics and context
