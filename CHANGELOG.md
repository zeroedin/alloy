## v0.1.1 (2026-06-24)

Reduce internal memory footprint by removing unused cascade data layers.

## v0.1.0 (2026-06-23)

Initial release of Alloy — a fast, extensible static site generator written in Go.

- **Config**: Customize your project structure, build output, content formats, and plugin settings in YAML, TOML, or JSON

  ```yaml
  title: "My Site"
  baseURL: "https://example.com"
  structure:
    content: "src/content"
    layouts: "src/layouts"
  templates:
    engine: "liquid"
  ```

- **Content**: Write pages in Markdown or plain HTML with YAML frontmatter

- **Data**: Load YAML, JSON, and CSV data files — available globally in templates as `site.data`

  ```yaml
  data:
    files:
      authors: "data/authors.json"
  ```

- **Cascade**: Inherit layout, metadata, and configuration down the directory tree via `_data.yaml` files with deep merge

- **Permalinks**: Control output URLs per-collection with token-based patterns

  ```yaml
  permalinks:
    blog: "/:year/:month/:slug/"
  ```

- **Collections**: Group content and generate taxonomy pages

  ```yaml
  taxonomies:
    tags:
      permalink: "/tags/:slug/"
  collections:
    blog:
      sortBy: "date"
      order: "desc"
  ```

- **Templates**: Liquid and Go `html/template` engines with shortcodes, filters, and composable layouts

- **Output**: Generate sitemaps, feeds, and multiple output formats per page

- **Assets**: Process assets through the build pipeline with built-in cache-busting support

- **Static**: Copy static files with passthrough mappings and glob-based exclude patterns

  ```yaml
  passthrough:
    - from: "node_modules/@rhds/elements"
      to: "assets/vendor/rhds"
      exclude: ["*.map"]
  ```

- **Pagination**: Paginate collections with configurable page size and custom permalink patterns

- **i18n**: Build multilingual sites with per-language content directories, URL prefixing, and translation strings

  ```yaml
  languages:
    en:
      title: "English Site"
      root: true
    fr:
      title: "Site Français"
  ```

- **Pipeline**: Incremental rebuilds that only reprocess changed files

- **Plugins (QuickJS)**: Drop a JS file in `plugins/` for in-process filters, hooks, and shortcodes — no Node.js required

  ```js
  export default function(alloy) {
      alloy.shortcode("greeting", (args) => {
          return `<p>Hello, ${args[0]}!</p>`;
      });
  }
  ```

- **Plugins (WASM)**: Compile filters from Rust, TinyGo, or AssemblyScript for near-native performance

- **Plugins (Node)**: Opt into a full Node.js subprocess runtime for plugins that need npm packages or filesystem access

- **Hooks**: React to build lifecycle events and inject virtual pages

  ```js
  alloy.hook("onContentLoaded", { pages: true }, (pages) => {
      // inject virtual pages, transform content, etc.
  });
  ```

- **CLI**: `alloy build`, `alloy dev` (development server with file watcher and live reload), `alloy serve`, `alloy init`, and `alloy version`
