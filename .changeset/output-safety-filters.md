---
type: minor
---

Go templates can now print trusted content as written, wherever it lands on the page. `safeHTML` works in Go templates, and four new filters cover the other places Go escapes values: `safeCSS`, `safeJS`, `safeURL` and `safeHTMLAttr`.

```yaml
# data/promo.yaml
banner: "<b>Sale today</b>"
style: "background: url(/img/sale.png)"
```

```go-html-template
<style>.promo { {{ .site.data.promo.style | safeCSS }} }</style>
<p class="promo">{{ .site.data.promo.banner | safeHTML }}</p>
<p>{{ .site.data.promo.banner }}</p>
```

```html
<style>.promo { background: url(/img/sale.png) }</style>
<p class="promo"><b>Sale today</b></p>
<p>&lt;b&gt;Sale today&lt;/b&gt;</p>
```

Previously `safeHTML` had no effect in Go templates, so the banner above was printed as `&lt;b&gt;Sale today&lt;/b&gt;`, and the other four filters didn't exist.

- Each filter works only for its own place. `safeHTML` in an `href` is still checked as a link; use `safeURL` there.
- Put the `safe*` filter last. A filter that changes the value after it, like `upcase`, gives back plain text, which Go escapes again. `default` keeps the value trusted when it returns it unchanged.
- In Liquid, which doesn't escape, all five filters return the value unchanged, so templates that use them work in either engine.
- Only use them on content you wrote. They turn off Go's protection for that value.

See [Output safety](/templates/output-safety/) for how the two engines differ and what to check before switching between them.
