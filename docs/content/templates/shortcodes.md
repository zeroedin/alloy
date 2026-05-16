---
layout: doc
title: Shortcodes
---

Shortcodes are reusable template fragments that extend Liquid's tag syntax. They are defined by plugins and used in content files as custom Liquid tags.

```liquid
{% youtube "dQw4w9WgXcQ" %}

{% callout "warning" %}
  Back up your data before upgrading.
{% endcallout %}
```

Shortcodes bridge the gap between writing content in Markdown and embedding rich, structured components that Markdown alone cannot express.

## Using shortcodes

Shortcodes are invoked as Liquid tags in content files. There are two forms:

**Inline shortcodes** take arguments and produce output with no body:

```liquid
{% youtube "dQw4w9WgXcQ" %}
{% icon "check" %}
{% version %}
```

**Block shortcodes** wrap content between opening and closing tags:

```liquid
{% callout "info" %}
  This is an informational note. **Markdown** works inside.
{% endcallout %}

{% details "Click to expand" %}
  Hidden content goes here.
{% enddetails %}
```

The closing tag is always `end` + the tag name: `{% endcallout %}`, `{% enddetails %}`.

## Passing parameters

Arguments are passed as quoted strings after the tag name:

```liquid
{% image "hero.jpg" "A scenic mountain view" %}
```

The plugin receives these as an ordered list of string arguments. Unquoted words are also supported for simple single-word parameters:

```liquid
{% icon check %}
{% badge primary %}
```

## How shortcodes work

Shortcodes are Liquid tags registered by plugins. When a plugin calls `addTag`, it registers a new tag name and a handler function. The handler receives:

1. **Arguments** -- the list of string parameters passed to the tag
2. **Body** -- the inner content for block shortcodes (empty string for inline shortcodes)

The handler returns an HTML string that replaces the tag in the rendered output.

A plugin defining a callout shortcode:

```javascript
// plugins/callout.js
export default function(api) {
  api.addTag("callout", (args, body) => {
    const type = args[0] || "info";
    return `<div class="callout callout-${type}">${body}</div>`;
  });
}
```

When the content file uses `{% callout "warning" %}...{% endcallout %}`, the plugin receives `["warning"]` as args and the inner content as body.

## Shortcodes in Markdown

Shortcodes are evaluated during the template rendering phase, before Markdown processing. This means the HTML output of a shortcode is passed through the Markdown renderer. Block-level HTML (like `<div>`) is preserved by Markdown, while inline HTML is left as-is within surrounding text.

```markdown
Here is an important note:

{% callout "warning" %}
  Always **back up** your data.
{% endcallout %}

And the text continues.
```

## Built-in shortcodes

Alloy does not ship built-in shortcodes. All shortcodes are plugin-defined, keeping the core minimal and avoiding opinionated defaults. Common patterns like callouts, figures, and embeds are implemented as plugins you add to your project.

See [Plugins](/plugins/) for how to create plugins that register shortcodes with `addTag`.
