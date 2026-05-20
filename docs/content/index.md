---
layout: landing
body_class: landing-page
title: Fast, Extensible Static Site Generator
meta_title: Alloy — Fast, Extensible Static Site Generator
---

<div class="hero">
  <svg class="hero-logo" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 492.5 540.6" width="100" height="109" aria-hidden="true"><defs><style>.hero-mark{color-scheme:light dark;fill:none;stroke:light-dark(#151515,#FFFFFF);stroke-miterlimit:10;stroke-width:10px}</style></defs><polygon class="hero-mark" points="128.4 300.4 127.4 302.8 108.8 348.8 72.2 330.4 13.3 301 71.4 271.9 128.4 300.4"/><polygon class="hero-mark" points="108.8 348.8 97.2 377.4 89 397.7 73 389.7 13.3 359.9 72.2 330.4 108.8 348.8"/><polygon class="hero-mark" points="149 249.6 128.4 300.4 71.4 271.9 13.3 242.8 74.4 212.3 132.5 241.4 149 249.6"/><polygon class="hero-mark" points="168.5 201.3 157.3 229 149 249.6 132.5 241.4 74.4 212.3 11.2 180.7 69.3 151.7 132.5 183.2 168.5 201.3"/><polygon class="hero-mark" points="479.3 180.7 418.2 211.2 360.1 240.3 339.1 250.8 329 224.8 320.1 202.1 360 182.2 421.2 151.7 479.3 180.7"/><polygon class="hero-mark" points="481.4 242.8 359.7 303.7 357.5 297.9 339.1 250.8 360.1 240.3 418.2 211.2 481.4 242.8"/><polygon class="hero-mark" points="481.4 359.9 423.3 388.9 398 401.6 385.7 370.2 378.7 352.3 422.5 330.4 481.4 359.9"/><polygon class="hero-mark" points="89 397.7 70.2 444.3 17.4 418 73 389.7 89 397.7"/><polygon class="hero-mark" points="349.1 485.3 251.4 535 139.9 479.2 158.6 432.6 247.3 476.9 330.7 435.2 349.1 485.3"/><polygon class="hero-mark" points="481.4 418 417.1 450.7 398 401.6 423.3 388.9 481.4 418"/><polygon class="hero-mark" points="481.4 301 422.5 330.4 378.7 352.3 359.7 303.7 423.2 271.9 481.4 301"/><polygon class="hero-mark" points="479.3 122.6 421.2 151.7 360 182.2 320.1 202.1 300.8 152.6 274.9 86.1 215.1 86.1 186.8 156.1 168.5 201.3 132.5 183.2 69.3 151.7 11.2 122.6 245.2 5.6 479.3 122.6"/><polygon class="hero-mark" points="330.7 435.2 247.3 476.9 158.6 432.6 178.3 383.5 247.3 418 312.5 385.4 330.7 435.2"/><polygon class="hero-mark" points="245.2 239.6 229.8 231.9 245.2 192.8 259.6 232.4 245.2 239.6"/><polygon class="hero-mark" points="297 335 247.3 359.9 190.4 331.4 191.1 329.6 210.5 280.4 245.2 297.8 277.6 281.6 292.9 323.8 297 335"/><polygon class="hero-mark" points="277.6 281.6 245.2 297.8 210.5 280.4 220 256.5 229.8 231.9 245.2 239.6 259.6 232.4 267 252.7 277.6 281.6"/></svg>
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
