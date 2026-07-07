---
type: minor
---

Feed templates placed in `layouts/` are now discovered and rendered through the template engine during builds. Template placement determines the output path.

```
layouts/feed.xml       → /feed.xml        (site-wide feed)
layouts/blog/feed.xml  → /blog/feed.xml   (section feed)
```

```xml
<!-- layouts/feed.xml -->
<?xml version="1.0"?>
<rss><channel><title>{{ site.title }}</title></channel></rss>
```
