---
layout: doc
title: Shortcodes
nav_weight: 40
description: "Reusable content snippets that accept arguments and output HTML, embeddable directly in Markdown."
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

<wa-tab-group>
<wa-tab slot="nav" panel="inline-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="inline-go">Go templates</wa-tab>

<wa-tab-panel name="inline-liquid" active>

```liquid
{% youtube "dQw4w9WgXcQ" %}
{% github_star "alloy-ssg/alloy" %}
```

</wa-tab-panel>
<wa-tab-panel name="inline-go">

```html
{{ youtube "dQw4w9WgXcQ" }}
{{ github_star "alloy-ssg/alloy" }}
```

</wa-tab-panel>
</wa-tab-group>

### Block shortcodes

Block shortcodes wrap inner content, letting you add markup around authored text:

<wa-tab-group>
<wa-tab slot="nav" panel="block-liquid" active>Liquid</wa-tab>
<wa-tab slot="nav" panel="block-go">Go templates</wa-tab>

<wa-tab-panel name="block-liquid" active>

```liquid
{% callout "warning" %}
  Do not deploy to production without running the test suite first.
{% endcallout %}
```

Liquid block shortcodes close with `{% end<name> %}`.

</wa-tab-panel>
<wa-tab-panel name="block-go">

```html
{{% callout "warning" %}}
  Do not deploy to production without running the test suite first.
{{% /callout %}}
```

Go template block shortcodes use `{{% %}}` delimiters (double braces) and close with `{{% /<name> %}}`.

</wa-tab-panel>
</wa-tab-group>

```html
<!-- Output (both engines) -->
<div class="callout callout--warning">
  Do not deploy to production without running the test suite first.
</div>
```

### Engine differences

| | Liquid | Go templates |
|---|---|---|
| Inline | `{% tag "arg" %}` | `{{ tag "arg" }}` |
| Block open | `{% tag "arg" %}` | `{{% tag "arg" %}}` |
| Block close | `{% endtag %}` | `{{% /tag %}}` |
| Inner content | Rendered HTML | Rendered HTML |
| Plugin callback | `(args, content)` | `(args, content)` |

Both engines pass rendered HTML to the plugin callback — the same `alloy.shortcode()` plugin works for both engines with no engine-specific code.

### Code block escaping

`{{% %}}` delimiters inside fenced code blocks and inline `<code>` elements are treated as literal text, not shortcode invocations.

## Variable arguments (Liquid)

In Liquid templates, unquoted shortcode arguments resolve from the template context instead of being passed as literal strings:

```liquid
{% assign vid = "dQw4w9WgXcQ" %}
{% youtube vid %}              <!-- resolves vid to "dQw4w9WgXcQ" -->
{% youtube "hardcoded" %}      <!-- stays literal "hardcoded" -->
{% youtube page.videoId %}     <!-- resolves nested path -->
```

Dotted paths like `page.videoId` traverse nested maps in the template context.

### Mixed arguments

Quoted and unquoted arguments work in the same tag:

```liquid
{% card "primary" page.size %}
```

The first argument is the literal string `"primary"`. The second resolves `page.size` from the context.

### Fallback behavior

When an unquoted argument does not match any context variable, it falls back to its literal token string. `{% youtube nonexistent %}` passes `"nonexistent"` to the shortcode callback. This preserves backward compatibility — existing shortcodes that used unquoted literal strings continue to work.

### Empty return

Shortcodes that return an empty string produce no output. Previous versions emitted an `<alloy-shortcode>` placeholder element — this is no longer the case.

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
