---
type: minor
---

**Breaking:** On pages generated with `perPage: 1`, a front-matter field that fails to parse or render now fails the build. The error names the page, the field and the Liquid error:

```yaml
# content/team.md
title: "{{ member.name | upcse }}"
```

```text
Error: front matter interpolation for team.md field "title": liquid render error in _fm_title: Liquid error: undefined filter upcse
```

Previously the build logged a warning, kept the field as written, and exited 0, so every generated page shipped with the raw `{{ member.name | upcse }}` in its title.

Fix the filter or the template in that field. `alloy dev` shows the error in its overlay when it rebuilds.
