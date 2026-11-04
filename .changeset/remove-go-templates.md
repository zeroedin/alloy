---
type: minor
---

**Breaking:** Go templates are no longer supported. Liquid is the only template engine.

- The `templates.engine` setting is gone. A `templates:` block left in your config is ignored, whatever its value, and the site builds with Liquid.
- Layouts and partials are Liquid. Alloy still looks for `name.liquid` first, then `name.html`, and reads a `.html` file as Liquid.
- Render hooks are `.liquid` files only. A `layouts/_markup/render-link.html` is no longer a hook.
- The `safeHTML`, `safeCSS`, `safeJS`, `safeURL` and `safeHTMLAttr` filters are removed. A template that still uses one fails the build with `undefined filter safeHTML`. Liquid prints values as written, so the filter isn't needed.
- `{{% name %}}` block shortcodes and the `{{%> name %}}` raw marker are removed. Use `{% name %}` … `{% endname %}` and `{%> name %}`. A `{{% name %}}` line in Markdown is no longer a shortcode: Liquid reads it as an output tag and prints nothing.
