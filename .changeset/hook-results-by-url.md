---
type: patch
---

What an `onContentLoaded` or `onDataCascadeReady` hook returns now reaches each page made from a paginated file. Before, Alloy matched each returned entry to a page by its source file's path alone. Every page from one paginated file has that path, so every entry landed on the last of them, and the other pages got nothing. Each entry now goes to the page with its `path` and `url`, in `alloy build` and `alloy dev` alike.

```javascript
// plugins/badge.js
export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true }, (pages) =>
    pages.map(p => ({ ...p, frontMatter: { ...p.frontMatter, badge: 'for ' + p.url } })));
}
```

With `content/team.md` paginating Ann, Bob and Cy, one page each, and a layout printing `badge=[{{ page.badge }}]`:

| Page | Before | Now |
|---|---|---|
| `/team/ann/` | `badge=[]` | `badge=[for /team/ann/]` |
| `/team/bob/` | `badge=[]` | `badge=[for /team/bob/]` |
| `/team/cy/` | `badge=[for /team/cy/]` | `badge=[for /team/cy/]` |

`onDataCascadeReady` entries now have `url` too. A hook that builds its entries from the ones it gets, as above, needs no change. An entry with no `url`, or one no page has, is still matched by `path` when only one page has that path. Each page must now get exactly one entry. When an entry can't be placed on one page, because several pages share its path, no page has its path, its `url` is another page's, or another entry already went to that page, or when an entry isn't an object, such as `null`, `alloy build` now fails with an error. Before, such an entry went to the last page with that path, or was dropped:

```text
Error: plugin hook onContentLoaded: an entry for team.md has no url, and 3 pages have that path: give each entry the url of the page it is for
```
