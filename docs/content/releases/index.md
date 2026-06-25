---
layout: doc
title: Release Notes
nav_section: "Release Notes"
nav_section_weight: 9
nav_weight: 10
description: "Alloy release notes and changelog"
---

{% assign releases = collections.releases | reverse %}
{% for release in releases %}
<article class="release-entry">
<h2><a href="{{ release.url }}">{{ release.title }}</a></h2>
<time datetime="{{ release.date | date: '%Y-%m-%d' }}">{{ release.date | date: '%B %d, %Y' }}</time>
<p>{{ release.description }}</p>
</article>
{% endfor %}
