---
layout: doc
title: Colocated Assets
nav_weight: 40
description: "Non-content files placed alongside Markdown and HTML pages are copied to the output automatically, preserving their path."
---

Files in the `content/` directory whose extension does not match `content.formats` (default: `md`, `html`) are automatically copied to the output, preserving their path:

```
content/about/
  index.md              -> processed as content
  diagram.svg           -> _site/about/diagram.svg
  hero.png              -> _site/about/hero.png
```

This enables colocating assets with the content that uses them:

```markdown
---
title: "About Us"
---

# About Us

![Team photo](hero.png)

{% inline "./diagram.svg" %}
```

## What Gets Copied

All files in the content directory are copied except:

- Files matching `content.formats` (those are content pages)
- `_data.yaml` / `_data.yml` (cascade data files)
- Dot-prefixed files (`.DS_Store`, `.gitkeep`, etc.)