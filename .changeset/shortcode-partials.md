---
type: patch
---

A plugin shortcode used as a block no longer fails the build with `Unknown tag 'end<name>'` when a partial or the spacing of its end tag differs from what the page uses:

- **Partials decide for themselves.** A partial loaded with `{% render %}` or `{% include %}` can use a shortcode as a block while the page uses it inline, the other way round, or when the page doesn't use it at all. Before, a partial followed the page that included it.
- **Any spacing Liquid accepts closes a block:** `{%endbox%}`, `{%-endbox-%}` and `{%   endbox   %}`, not only `{% endbox %}` and `{%- endbox %}`.

An `{% endname %}` closes the nearest open use of the same name before it. In `{% box "a" %}x{% box "b" %}y{% endbox %}`, `box "a"` is inline and `box "b"` is a block, so the page renders both. Before, `box "a"` took everything up to `{% endbox %}` as its body, including `{% box "b" %}` as text.
