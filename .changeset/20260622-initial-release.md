---
type: minor
---

Initial release of Alloy — a static site generator combining Hugo's speed with 11ty's extensibility.

- **Config**: YAML/TOML/JSON configuration loading with `--root` flag override
- **Content**: Frontmatter parsing, Markdown rendering via goldmark, and lifecycle filtering
- **Data**: YAML/JSON/CSV data file loading with external data file mappings and ordered key preservation
- **Cascade**: 5-level data cascade with deep merge
- **Permalinks**: Token resolution, section-based lookup, and cascade-driven permalinks
- **Collections**: Collection building and taxonomy system with URL conflict detection
- **Templates**: Liquid engine (liquidgo), Go html/template engine, layout resolution, shortcodes, filters, render hooks, and `oget`/`orange` helpers
- **Output**: File writing, sitemap, feeds, and multiple output format support
- **Assets**: Asset copying with optional hook processing, `cachebust` and `get_hash` fingerprinting filters
- **Static**: Concurrent static file copies with passthrough support and exclude patterns
- **Pagination**: Full pagination support with template permalink resolution
- **i18n**: Multilingual build pipeline with i18n URL prefixing
- **Pipeline**: Multi-stage build pipeline with incremental rebuilds, site data reload, template usage tracking, and `--profile`/`--profile-dir` flags
- **Dev server**: HTTP server with port auto-increment, file watcher, live reload, and static/asset/passthrough recopy on change
- **SSR**: Per-page CLI rendering model with streaming, timeouts, and body extraction
- **Plugins (QuickJS)**: Filter bridging, hook parsing, shortcode support, `alloy.data` access, and `CallFilter` execution
- **Plugins (Node)**: Full Node.js runtime with auto-scaling worker pool, process group isolation, PID file cleanup, and SIGKILL escalation
- **Plugins (WASM)**: wazero-based runtime with alloc/filter ABI, multi-arg `CallExport`, per-hook priority and scope metadata
- **Hooks**: Declarative hook payload scoping, `onContentLoaded` and `onPagesReady` virtual page injection, hook return value application, and lifecycle hook dispatch in incremental rebuilds
- **CLI**: `alloy build`, `alloy serve`, `alloy init` (full project scaffolding), `alloy version`, and startup banner with progress bar
- **Release pipeline**: Custom changeset-based workflow with automated release PR and cross-compiled binary distribution
