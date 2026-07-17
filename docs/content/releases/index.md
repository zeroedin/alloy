---
layout: doc
title: Release Notes
nav_section: "Release Notes"
nav_section_weight: 1000
nav_weight: 10
description: "Alloy release notes and changelog"
---

{% assign releases = collections.releases | reverse %}

<section class="release-tiles">
  <a class="release-tile" href="https://github.com/zeroedin/alloy" target="_blank" rel="noopener">
    <svg viewBox="0 0 24 24" aria-hidden="true"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
    <span class="release-tile-label">Repository</span>
  </a>
  <a class="release-tile" href="https://github.com/zeroedin/alloy/releases" target="_blank" rel="noopener">
    <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"/><line x1="7" y1="7" x2="7.01" y2="7"/></svg>
    <span class="release-tile-label">Releases on GitHub</span>
  </a>
</section>

{% for release in releases %}
<article class="release-entry">
  <div class="release-entry-head">
    <h2><a href="{{ release.url }}">{{ release.title }}</a></h2>
    <span class="release-badge release-badge-{{ release.type }}">{{ release.type | capitalize }}</span>
    {% if forloop.first %}<span class="release-badge release-badge-latest">Latest</span>{% endif %}
  </div>
  <time datetime="{{ release.date | date: '%Y-%m-%d' }}">{{ release.date | date: '%B %d, %Y' }}</time>
  <p>{{ release.description }}</p>
  <a class="release-github-link" href="https://github.com/zeroedin/alloy/releases/tag/{{ release.title }}" target="_blank" rel="noopener">
    <svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></svg>
    View on GitHub
  </a>
</article>
{% endfor %}
