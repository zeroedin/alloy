---
type: minor
---

Liquid shortcode arguments resolve variables from the template context. Unquoted arguments like `{% youtube page.videoId %}` look up `page.videoId` in the template context and pass the resolved value to the shortcode callback. Quoted arguments remain literal strings. Dotted paths traverse nested maps: `page.videoId` resolves to the `videoId` key inside the `page` map. Non-string values are converted to strings. When an unquoted argument does not match any context variable, it falls back to its literal token string.

```liquid
{% assign vid = "dQw4w9WgXcQ" %}
{% youtube vid %}          <!-- resolves vid to "dQw4w9WgXcQ" -->
{% youtube "hardcoded" %}  <!-- stays literal "hardcoded" -->
{% youtube page.videoId %} <!-- resolves nested path -->
```

Mixed quoted and unquoted arguments work in the same tag: `{% card "primary" page.size %}` passes `"primary"` as a literal and resolves `page.size` from context.

Shortcodes returning an empty string now produce no output in Liquid, matching Go template behavior. Previously, empty-returning shortcodes emitted an `<alloy-shortcode>` placeholder element into production HTML.
