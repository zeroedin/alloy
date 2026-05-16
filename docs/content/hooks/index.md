---
layout: doc
title: Lifecycle Events
---

Hooks let plugins respond to events during the build pipeline. Register a hook with `alloy.hook(event, options, fn)` or the alias `alloy.on(event, options, fn)`:

```javascript
// plugins/lazy-images.js
alloy.hook("onContentTransformed", { priority: 50, pages: true }, (page) => {
  page.body = page.body.replace(
    /<img(?!\s+loading) /g,
    '<img loading="lazy" '
  );
  return page;
});
```

The options object is required. At minimum, pass `{ priority: 50 }`.

## Event reference

| Event | Fires | Payload | Return | Per-page |
|---|---|---|---|---|
| `onConfig` | After config is loaded | `{ config }` | Modified config | No |
| `onDataFetched` | After data files are loaded | `{ data }` | Modified data | No |
| `onContentLoaded` | After all content files are parsed | `{ pages }` | Modified pages array | No |
| `onDataCascadeReady` | After data cascade is resolved | `{ pages }` | Modified pages array | No |
| `onPagesReady` | After pages are built, before taxonomy | `{ pages }` | `{ addPages: [...] }` | No |
| `onBeforeValidation` | Before content validation | `{ pages }` | Modified pages array | No |
| `onAfterValidation` | After content validation | `{ pages, errors }` | Modified pages/errors | No |
| `onContentTransformed` | After Markdown rendering | `{ page }` | Modified page | Yes |
| `onPageRendered` | After template rendering | `{ page }` | Modified page | Yes |
| `onAssetProcess` | Per asset file | `{ asset }` | Modified asset | No |
| `onBuildComplete` | After all output is written | `{ stats }` | None | No |
| `onDevServerStart` | When dev server starts | `{ server }` | None | No |
| `onFileChanged` | On file change in dev mode | `{ path, event }` | None | No |

## When each event fires

Events fire in this order during a build:

```
onConfig
  → onDataFetched
    → onContentLoaded
      → onDataCascadeReady
        → onPagesReady
          → onBeforeValidation
            → onAfterValidation
              → onContentTransformed (per page)
                → onPageRendered (per page)
                  → onAssetProcess (per asset)
                    → onBuildComplete
```

`onDevServerStart` and `onFileChanged` only fire in dev mode.

## Per-page vs batch hooks

**Batch hooks** (`onContentLoaded`, `onPagesReady`, etc.) receive the full page set and fire once per build. Use them for cross-page operations like injecting virtual pages or computing global data.

**Per-page hooks** (`onContentTransformed`, `onPageRendered`) fire once for every page with a single-page payload. They run through the Node worker pool when using Node plugins, enabling parallel processing. Use them for content transforms that operate on one page at a time.

## Registration

```javascript
alloy.hook(event, options, fn);
alloy.on(event, options, fn);    // alias, identical behavior
```

The options object controls priority, scoping, and data access. See [Hook Scoping](/hooks/scoping/) for details on `pages`, `pageFields`, and `data` options.

| Option | Type | Default | Description |
|---|---|---|---|
| `priority` | number | 50 | Lower runs first |
| `pages` | boolean/string | false | Page scoping mode |
| `pageFields` | array | all fields | Subset of page fields to receive |
| `data` | array | none | Site data keys to include |

## Priority ordering

Hooks on the same event run in priority order (lower numbers first). Hooks with equal priority run in plugin load order (alphabetical by filename, Tier 1 before Tier 2 before Tier 3):

```javascript
// plugins/a-meta.js — runs first (priority 10)
alloy.hook("onContentTransformed", { priority: 10, pages: true }, (page) => {
  page.meta = computeMeta(page);
  return page;
});

// plugins/b-transform.js — runs second (priority 50)
alloy.hook("onContentTransformed", { priority: 50, pages: true }, (page) => {
  // page.meta is available from the previous hook
  return page;
});
```

## Hook chaining

Each hook receives the output of the previous hook on the same event. The return value of one hook becomes the input to the next:

```javascript
// First hook: adds lazy loading
alloy.hook("onContentTransformed", { priority: 10, pages: true }, (page) => {
  page.body = page.body.replace(/<img /g, '<img loading="lazy" ');
  return page;
});

// Second hook: wraps images in figures
alloy.hook("onContentTransformed", { priority: 20, pages: true }, (page) => {
  // page.body already has loading="lazy" from the first hook
  page.body = page.body.replace(
    /<img([^>]+)>/g,
    '<figure><img$1></figure>'
  );
  return page;
});
```

If a hook does not return a value (returns `undefined`), the original input is passed to the next hook unchanged.

## Example: lazy loading images

```javascript
// plugins/lazy-images.js
alloy.hook("onContentTransformed", { priority: 50, pages: true }, (page) => {
  // Skip pages that opt out
  if (page.lazyImages === false) return page;

  page.body = page.body.replace(
    /<img(?!\s+loading)([^>]*)>/g,
    '<img loading="lazy"$1>'
  );
  return page;
});
```

## Example: HTML minification

```javascript
// plugins/minify.js — Node plugin for npm access
export const runtime = "node";
import { minify } from "html-minifier-terser";

export default function(alloy) {
  alloy.hook("onPageRendered", { priority: 90, pages: true }, async (page) => {
    if (!page.url.endsWith("/")) return page;

    page.body = await minify(page.body, {
      collapseWhitespace: true,
      removeComments: true,
    });
    return page;
  });
}
```

## Example: virtual page injection with onPagesReady

`onPagesReady` is unique: instead of modifying existing pages, it can inject new virtual pages by returning an `addPages` array:

```javascript
// plugins/archives.js
alloy.hook("onPagesReady", { priority: 50 }, (data) => {
  const years = new Set(
    data.pages.map(p => new Date(p.date).getFullYear())
  );

  const archivePages = [...years].map(year => ({
    title: `Archive: ${year}`,
    permalink: `/archive/${year}/`,
    layout: "archive",
    year: year,
    body: "",
  }));

  return { addPages: archivePages };
});
```

Virtual pages injected by `onPagesReady` are added before taxonomy processing. They appear in collections and can have taxonomy terms assigned.

Each page in the `addPages` array must include at minimum a `permalink` and a `title`. Other fields (`layout`, `body`, custom front matter) are optional and follow the same rules as regular page front matter.
