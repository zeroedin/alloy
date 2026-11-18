---
type: minor
---

Alloy now uses liquidgo v5, a port of Shopify Liquid 5.14, so templates give the results Shopify's Liquid gives where the old library differed:

| Template | Before | Now |
|---|---|---|
| `{{ 0.1 \| plus: 0.2 }}` | `0.30000000000000004` | `0.3` |
| `{{ 14 \| divided_by: 3 }}` | `4.666666666666667` | `4` |
| `{{ "héllo" \| size }}` | `6` | `5` |
| `{% if n == 100 %}` with `n` read from JSON | false | true |
| `{{ page.tags \| join }}` with tags `a`, `b` | `a, b` | `a b` |
| `{% if list contains "vip" %}` with `list` = `["vip_gold", "x"]` | true | false |
| `{{ obj.size }}` on an object with 2 keys and no `size` key | empty | `2` |
| `{% case k %}{% when "a" %}A{% when "a", "b" %}AB{% endcase %}` with `k` = `"a"` | `A` | `AAB` |

Dividing two integers now gives an integer: write `14.0 | divided_by: 3` for a decimal result. `size`, `truncate`, `truncatewords`, `slice` and `capitalize` count characters, not bytes, so accented and non-Latin text isn't cut in the middle of a character. `date` follows Ruby's `strftime`.

A `for` loop over a long list is much faster. The old library copied everything a template had written so far each time a loop added to it. A 1,000-page site whose layout lists every page in its nav now builds in under a second instead of about 15 seconds, with the same output.

A data file object answers key lookups, `.size` and `.first`, as before. `{{ data.keys }}` and `{{ data.len }}` now render nothing: the old library read them from Alloy's internal Go methods, and they were never documented. Use `.size` for the number of keys, and a loop for the keys themselves, in file order:

```liquid
{% for pair in site.data.nav %}{{ pair[0] }}{% unless forloop.last %}, {% endunless %}{% endfor %}
```

Markup that doesn't parse, such as `{{ | broken_syntax }}`, still fails the build. An unknown tag is now reported as `Unknown tag 'name'`.
