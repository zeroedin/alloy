---
type: minor
---

Format layout resolution (JSON, XML, etc.) now uses the same lookup chain as HTML layouts. The output format is infixed before the engine extension — one algorithm, one candidate order.

For a blog child page with `outputs: ["html", "json"]`:

1. `layouts/<front-matter-layout>.json.liquid`
2. `layouts/post.json.liquid` (date-based section child)
3. `layouts/<filename>.json.liquid`
4. `layouts/default.json.liquid`

The legacy `single` concept has been removed from format resolution. The duplicate `ResolveFormatLayout` helper in `output/formats.go` has been deleted.
