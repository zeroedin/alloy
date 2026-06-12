---
layout: landing
body_class: landing-page
title: Fast, Extensible Static Site Generator
meta_title: Alloy — Fast, Extensible Static Site Generator
---

<div class="hero">
  <svg class="hero-logo" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 540.23 560">
    <path d="m442.3 204.86 79.34-39.34L272.78 42.13l-2.67-1.32L18.59 165.52l79.34 39.34-79.34 39.34 79.34 39.34-79.34 39.34 79.34 39.34-79.34 39.34 101.08 50.12L239 132.18h62.23l119.33 319.5 101.08-50.12-79.34-39.34 79.34-39.34-79.34-39.34 79.34-39.34zm52.33 39.34-117.07 58.05-20.57-55.08 71.81-35.6 65.83 32.64Zm-383.2-32.64 71.81 35.6-20.57 55.08L45.6 244.19l65.83-32.64Zm0 78.68 47.01 23.31-20.57 55.09-92.27-45.75 65.83-32.64Zm1.65 144.78L45.6 401.56l65.83-32.64 22.22 11.02zm117.59-314.84-43.21 115.68L45.6 165.52 270.11 54.2l224.51 111.32-141.86 70.34-43.21-115.68h-78.89Zm263.96 281.38-67.48 33.46-20.57-55.08 22.22-11.02zm0-78.68-92.27 45.75-20.57-55.09 47.01-23.31 65.83 32.64Z"/>
    <path d="m318.93 344.72-48.81-160.47-48.81 160.47 48.81 24.2zm-35.24-74.61-13.58 6.73-13.58-6.73 13.58-44.63zM253 281.75l17.12 8.49 17.12-8.49 17.26 56.73-34.37 17.04-34.37-17.04 17.26-56.73Zm17.11 152.45-66.07-32.76-24.34 80.01 90.4 44.83 90.4-44.83-24.34-80.01-66.07 32.76Zm0 78.68-75.96-37.67 17.26-56.73 58.71 29.11 58.71-29.11 17.26 56.73-75.96 37.67Z"/>
  </svg>
  <h1>Alloy</h1>
  <p class="hero-subtitle">A Go-powered static site generator built for speed and extensibility. Liquid templates, a tiered plugin system, and builds that finish in seconds — not minutes.</p>
  <div class="hero-actions">
    <a href="/getting-started/" class="btn btn-primary">Get Started</a>
    <a href="https://github.com/zeroedin/alloy" class="btn btn-secondary" target="_blank" rel="noopener">GitHub</a>
  </div>
</div>

<div class="features-grid">
  <div class="feature">
    <h3>Go-Speed Builds</h3>
    <p>Single binary, no runtime dependencies. Thousands of pages render in seconds using parallel worker pools.</p>
  </div>
  <div class="feature">
    <h3>Liquid Templates</h3>
    <p>Industry-standard template language with full support for includes, filters, and whitespace control.</p>
  </div>
  <div class="feature">
    <h3>Tiered Plugins</h3>
    <p>Three tiers of extensibility: embedded QuickJS, compiled WASM (Rust, TinyGo), and Node.js via IPC bridge.</p>
  </div>
  <div class="feature">
    <h3>Output Formats</h3>
    <p>Generate HTML, JSON, XML, or any text format from the same content with matching layout templates.</p>
  </div>
  <div class="feature">
    <h3>Data Cascade</h3>
    <p>Six-level data merge from global to directory to front matter. Shared data is pointer-backed for zero-copy efficiency.</p>
  </div>
  <div class="feature">
    <h3>Taxonomies</h3>
    <p>Declare taxonomy keys in config and Alloy auto-generates index and per-term pages with full template control.</p>
  </div>
</div>

<div class="install-block">
  <h2>Quick Install</h2>

```bash
go install github.com/zeroedin/alloy@latest
```

  <p>Requires Go 1.21 or later. See the <a href="/getting-started/">installation guide</a> for more options.</p>
</div>
