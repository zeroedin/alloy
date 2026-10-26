---
layout: doc
title: Output Safety
nav_weight: 35
description: "How Liquid and Go templates escape the values you print, how to print trusted HTML in each, and what to watch for when switching engines."
---

Liquid and Go templates treat the values you print in opposite ways. Liquid prints them exactly as written. Go templates escape them. The same data file, printed the same way, gives different HTML depending on the engine:

```yaml
# data/promo.yaml
banner: "<b>Sale today</b>"
```

{% raw %}
| Engine | Template | Prints | On the page |
|---|---|---|---|
| Liquid | `{{ site.data.promo.banner }}` | `<b>Sale today</b>` | **Sale today**, in bold |
| Go templates | `{{ .site.data.promo.banner }}` | `&lt;b&gt;Sale today&lt;/b&gt;` | the text `<b>Sale today</b>` |
{% endraw %}

## Each engine, and how to change it

{% raw %}
| | Liquid | Go templates |
|---|---|---|
| By default | prints values **as written** | **escapes** values |
| To escape a value | `{{ v \| escape }}` | already escaped |
| To print trusted HTML as written | already printed as written | the [`safe*` filters](/templates/filters/#output-safety-safehtml-safecss-safejs-safeurl-safehtmlattr) |
{% endraw %}

Liquid's behavior matches Jekyll and Shopify. Go's comes from its `html/template` package, which escapes each value by the rules of where it lands — the page body, `<style>`, `<script>`, a link, or an attribute. That's why Go needs one filter per place: `safeHTML`, `safeCSS`, `safeJS`, `safeURL` and `safeHTMLAttr`. The [filter reference](/templates/filters/#output-safety-safehtml-safecss-safejs-safeurl-safehtmlattr) has an example of each.

In Liquid the `safe*` filters do nothing and return the value unchanged, so a template that uses them works in either engine.

## Switching from Go templates to Liquid

**Values that Go escaped will start printing as written, with no error and no change to your templates.** If a Go site relied on escaping to show text like `<b>` literally, or to keep markup in data from rendering, that stops when you switch.

Before switching, look for values that aren't meant to be HTML — names, titles, anything that comes from outside your own files — and add `| escape` to each in the Liquid templates:

{% raw %}
```liquid
<h2>{{ review.author | escape }}</h2>
```
{% endraw %}

Going the other way, from Liquid to Go, is the safe direction: values that printed as HTML start printing escaped. Anything that needs to stay HTML gets the matching `safe*` filter.

## Trusted means you checked it

The `safe*` filters are you telling Go a value is already safe for where it lands. Alloy doesn't check that. Use them on content the site's authors wrote. On data you don't control — a third-party feed, user submissions, a remote data source — they remove exactly the protection Go's escaping gives you: `safeJS` and `safeURL` on someone else's data let them run script on your page.

Liquid gives that data no protection by default, so in Liquid templates, escape values like these with `| escape`.
