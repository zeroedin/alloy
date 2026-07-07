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
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{ site.title }}</title>
    <link>{{ site.baseURL }}</link>
    <atom:link href="{{ site.baseURL }}/feed.xml" rel="self" type="application/rss+xml"/>
    <description>{{ site.title }}</description>
    {% for post in collections.blog %}
    <item>
      <title>{{ post.title }}</title>
      <link>{{ site.baseURL }}{{ post.url }}</link>
      <guid>{{ site.baseURL }}{{ post.url }}</guid>
      <pubDate>{{ post.date | date: "%a, %d %b %Y 00:00:00 +0000" }}</pubDate>
    </item>
    {% endfor %}
  </channel>
</rss>
```
