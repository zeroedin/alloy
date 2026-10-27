---
type: minor
---

**Breaking:** Go templates now escape a `content` or `summary` field in a data file, like every other field. Previously any value under a key with one of those names printed as raw HTML, so a data file could put a live `<script>` on the page while its other fields were escaped.

```yaml
# data/review.yaml
author: "<b>Sam</b>"
content: "<b>Great</b>"
```

```go-html-template
<p>{{ .site.data.review.author }}</p>
<p>{{ .site.data.review.content }}</p>
<p>{{ safeHTML .site.data.review.content }}</p>
```

```html
<p>&lt;b&gt;Sam&lt;/b&gt;</p>
<p>&lt;b&gt;Great&lt;/b&gt;</p>
<p><b>Great</b></p>
```

If your site printed HTML from a data file's `content` or `summary` field, or from a `summary` your pages inherit, wrap it in `safeHTML`, as in the last line.

- What prints as HTML now depends on where a value came from, not on what it's called. Your pages' rendered Markdown (`{{ .content }}`, `{{ .page.content }}`) and your pages' front-matter `summary` still print as HTML everywhere they appear, including `site.pages`, collections, taxonomy pages and pagination.
- A front-matter `content` field on a page with no body is escaped. It's text you wrote, not rendered Markdown.
- Only a `summary` written in the page's own front matter is trusted. A `summary` a page inherits as a default, from a `_data.yaml` or a data file named `summary`, is escaped. Summaries that pagination fills in per item, and summaries on pages a plugin adds, are still trusted.
- `where` now matches a page's summary in Go templates: `{{ where .site.pages "summary" "<b>sum</b>" }}` used to find nothing. `sort` by `summary` also now orders number-like summaries as numbers (`9` before `10`), as Liquid does.
- Liquid output is unchanged.

See [Output safety](/templates/output-safety/#what-go-prints-as-html-without-a-filter) for the full list.
