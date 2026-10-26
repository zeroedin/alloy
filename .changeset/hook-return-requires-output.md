---
type: minor
---

**Breaking:** `onPageRendered` and `onContentTransformed` hooks must return the page object with `html` as a string. A hook that returns anything else now fails the build, and the error names that plugin and the page:

```text
Error: plugin hook onPageRendered: plugin "00-seo" returned no "html" for /about/ — return the page object (return page;)
```

Previously a hook that forgot to return the page didn't fail where the mistake was. It handed the next plugin a page with no `html`, and *that* plugin crashed and took the blame:

```text
Error: plugin hook onPageRendered: item 0: hook "onPageRendered": TypeError: cannot read property 'includes' of undefined
    at <anonymous> (01-transforms.js:2:36)
```

The same plugin running on its own built silently and kept the original HTML, so whether a plugin worked depended on which other plugins happened to be installed. Now it fails the same way wherever it runs.

The fix is to return the page:

```javascript
alloy.hook("onPageRendered", {}, (page) => {
  page.html = page.html.replace("<body>", '<body data-ready="true">');
  return page;
});
```

Returning the HTML as a bare string is no longer supported on either hook. Set `page.html` and return the page instead:

```javascript
// before
return page.html.replace(...);
// after
page.html = page.html.replace(...);
return page;
```

An empty string (`page.html = ""`) is still a valid result. On `onContentTransformed`, a hook may return `{ toc }` without `html` when every hook on the site is scoped to fields that leave out `html`, such as `pageFields: ["toc"]`; it must still return an object. In `alloy dev` the error is logged as a warning and the server keeps running, as for any hook error.

`onFormatRendered` is unchanged.
