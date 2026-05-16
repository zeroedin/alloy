---
layout: doc
title: Content Overview
---

Alloy processes Markdown and HTML files from the `content/` directory into output pages. Every content file needs front matter — even if it's empty.

```markdown
---
title: About Us
layout: default
---

This is the about page. Write **Markdown** here.
```

## Content file requirements

A file in `content/` is treated as content when its extension matches `content.formats` (default: `md` and `html`) and it has front matter. Front matter is the metadata block at the top of the file, delimited by `---` (YAML), `+++` (TOML), or `{` (JSON).

Empty front matter is valid — a file with just `---` / `---` and a body is a content page with default metadata. But a Markdown file with no delimiters at all is a build error:

```markdown
---
---

No metadata needed, but the delimiters are required.
```

## HTML content handling

HTML files have three classification paths based on their content:

| Condition | Treatment |
|---|---|
| Has front matter (`---`, `+++`, `{`) | Content page, processed normally |
| No front matter + starts with `<!DOCTYPE` or `<html>` | Passthrough — copied to output as-is |
| No front matter + HTML fragment (no DOCTYPE) | Content page with empty front matter, wrapped by cascade layout |

HTML fragments inherit their layout from the `_data.yaml` cascade. A `_data.yaml` with `layout: element` wraps every fragment in that directory with the element layout. Set `layout: false` to skip layout wrapping and pass fragments through unwrapped.

```
content/patterns/card/
├── _data.yaml           # layout: "element"
├── index.html           # has front matter → content page
└── patterns/
    ├── themes.html      # fragment → wrapped in element layout
    └── image.html       # fragment → wrapped in element layout
```

HTML fragments go through template processing — Liquid or Go template tags in the fragment body are evaluated.

## Content-colocated assets

Files in `content/` whose extension does not match `content.formats` are copied to the output directory as-is, preserving their path relative to `content/`. No template processing, no Markdown rendering — raw file copy.

```
content/about/
├── index.md        # content file → processed through pipeline
├── diagram.svg     # passthrough → copied to _site/about/diagram.svg
├── hero.png        # passthrough → copied to _site/about/hero.png
└── data-sheet.pdf  # passthrough → copied to _site/about/data-sheet.pdf
```

This lets you keep images, diagrams, and downloads next to the content that references them. In templates, reference colocated assets with relative paths:

```markdown
---
title: About Us
layout: default
---

![Architecture diagram](diagram.svg)

Download the [data sheet](data-sheet.pdf).
```

Excluded from passthrough: `_data.yaml` / `_data.yml` (cascade data files), dot-prefixed files (`.DS_Store`, `.gitkeep`), and directories.

## Supported content formats

The `content.formats` config controls which file extensions are eligible as content:

```yaml
# alloy.config.yaml
content:
  formats: ["md", "html"]   # default
```

Adding `liquid` to the list processes `.liquid` files as content — but only when the template engine is set to `liquid`. The Go template engine cannot render Liquid syntax, so `.liquid` files are always passthrough when `templates.engine: "gotemplate"`.
