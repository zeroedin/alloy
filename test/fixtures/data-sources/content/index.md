---
title: "Data Source Test"
---
# Posts from API

{% for post in site.data.api_posts %}
- {{ post.title }}
{% endfor %}
