---
type: minor
---

**Breaking:** Two template mistakes that used to log a warning now fail the build, like an error in a layout or in page content already does. Before, the build reported success and wrote broken pages.

- **A layout that doesn't exist.** If a page names a layout that isn't there, in front matter, in a `_data.yaml` cascade, or as an extension-bearing name like `layout: "missing.html"`, the build stops:

  ```text
  Error: layout "nonexistent" not found for page "about.md"
  ```

  Previously the page was written without its layout. `layout: false` is unaffected.

- **A pagination front-matter field that doesn't render.** On pages generated with `perPage: 1`, a field such as `title: "{{ member.name | upcse }}"` with a typo in the filter name now fails:

  ```text
  Error: front matter interpolation for team.md field "title": liquid render error in _fm_title: Liquid error: undefined filter upcse
  ```

  Previously every generated page shipped with the raw `{{ member.name | upcse }}` in its title.

`alloy dev` shows both errors in its error overlay when it rebuilds.
