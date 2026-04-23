---
title: "Alloy"
layout: "landing"
---

<div class="hero">
  <h1>Fast. Extensible. Go-powered.</h1>
  <p class="hero-subtitle">Alloy is a fast, extensible static site generator built in Go. Thousands of pages in seconds, Liquid templates, and a tiered plugin system — from in-process WASM to a full Node.js bridge.</p>
  <div class="hero-actions">
    <a href="/getting-started/" class="btn btn-primary">Get Started</a>
    <a href="https://github.com/zeroedin/alloy" class="btn btn-secondary" target="_blank" rel="noopener">GitHub</a>
  </div>
</div>

<div class="features-grid">
  <div class="feature">
    <h3>Fast Builds</h3>
    <p>Go-powered concurrent pipeline targeting &lt; 5 seconds for 1,000 pages. Templates parsed once, data shared by pointer.</p>
  </div>
  <div class="feature">
    <h3>Liquid Templates</h3>
    <p>Liquid as the default template engine with familiar syntax. Go html/template available as an alternative.</p>
  </div>
  <div class="feature">
    <h3>Tiered Plugins</h3>
    <p>Built-in Go filters (ns), in-process JS/WASM plugins (us), Node subprocess plugins (ms). Drop a file in plugins/ and go.</p>
  </div>
  <div class="feature">
    <h3>Web Component SSR</h3>
    <p>Opt-in two-phase rendering. Pipe page HTML to an external SSR engine for Declarative Shadow DOM.</p>
  </div>
  <div class="feature">
    <h3>Data Cascade</h3>
    <p>5-level merge: global data, directory data, front matter, plugin hooks. Objects deep-merge, arrays replace.</p>
  </div>
  <div class="feature">
    <h3>Dev Server</h3>
    <p>File watching, WebSocket live reload, incremental rebuilds, error overlay, port auto-increment. Preview mode for production output.</p>
  </div>
</div>

<div class="install-block">
  <h2>Quick Start</h2>

```bash
# Initialize a new project
alloy init

# Start the dev server
alloy serve

# Build for production
alloy build
```

</div>
