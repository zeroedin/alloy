## v0.6.0 (2026-07-24)

### Minor Changes

- `structure.components` controls where Alloy looks for SSR component source files. Defaults to `components/` when omitted.

  ```yaml
  # alloy.config.yaml
  structure:
    components: "elements"
  ```

  During `alloy serve`, Alloy watches this directory for changes and re-renders pages that use the affected components. Projects that keep component source outside `components/` previously got no SSR invalidation on file changes.

  Alloy's built-in SSR integration is experimental. This config option and its behavior may change in future releases.
- `onFormatRendered` fires once per non-HTML format body after layout rendering with `{ format, content, url, path, frontMatter }`. Return an object with a `content` key to replace the rendered body. The build ignores all other keys in the return value.

  ```javascript
  alloy.hook("onFormatRendered", {}, (payload) => {
    if (payload.format === "json") {
      return { content: JSON.stringify(JSON.parse(payload.content)) };
    }
  });
  ```

  `onPageRendered` skips pages whose `outputs` contains only non-HTML formats. A page with `outputs: ["json"]` routes through `onFormatRendered` instead. Both hooks fire independently when a page declares HTML and non-HTML outputs together.

  Return `null`, `undefined`, or an object without a `content` key to keep the original format body.
- **Breaking:** `onPageRendered` sends a page object `{ html, frontMatter, url, path }` instead of a raw HTML string. Only `html` in the return is applied back — `frontMatter`, `url`, and `path` are read-only context.

  ```javascript
  alloy.hook("onPageRendered", {}, (page) => {
    if (page.frontMatter.layout === "demo") return page;
    page.html = page.html.replace(/<h2/g, '<h2 class="styled"');
    return page;
  });
  ```

  Plugins that conditionally process pages can read `page.frontMatter` to decide whether to transform. Both `Build()` and `BuildIncremental()` send the same payload shape.

  Previously, the hook received a raw HTML string. Plugins that needed to skip certain pages had to embed `<meta>` markers in layout HTML and strip them downstream.
- Plugins declare external file dependencies via `addDependencies` in `onPageRendered` and `onContentTransformed` return values. Alloy tracks a reverse index in the build cache and rebuilds only the pages that declared a dependency when that file changes during `alloy dev`.

  ```javascript
  alloy.hook("onPageRendered", {}, (page) => {
    const result = renderSSR(page.html);
    return {
      html: result,
      addDependencies: [
        "elements/rh-card/rh-card.js",
        "elements/rh-icon/rh-icon.js",
      ],
    };
  });
  ```

  `onFileChanged` hooks return `{ invalidateByDependency: [...] }` to trigger targeted rebuilds via the reverse index, or `{ restart: true }` to restart Node bridge subprocesses before the rebuild. Restart clears Node's ESM module cache so the plugin re-imports fresh component definitions.

  ```javascript
  alloy.hook("onFileChanged", {}, (events) => {
    const changed = events
      .filter(ev => ev.path.startsWith("elements/") && ev.path.endsWith(".js"))
      .map(ev => ev.path);
    if (changed.length > 0) {
      return { invalidateByDependency: changed, restart: true };
    }
  });
  ```

  Dependencies from both hooks accumulate per page per build. Removing a component tag from a page drops that dependency on the next rebuild. Non-array `addDependencies` values produce a warning. Alloy normalizes dependency paths with `filepath.Clean`, so `./data.json` and `data/../data.json` match the canonical `data.json` cache key.

  Previously, changes to files outside `content/` and `layouts/` either triggered no rebuild or forced a full rebuild of every page. A site with 720 pages and an SSR plugin that reads component definitions would rebuild all 720 pages when a single component file changed.
- `alloy dev` and `alloy serve` write a lockfile at `.alloy/server.lock` on startup. If another alloy process is already watching the same project directory, a warning prints to stderr with the conflicting PID, port, mode, and a `kill` command. Startup continues without blocking.

  ```
  warning: another alloy process (PID 4659, alloy serve on port 3003, started 2026-07-14T13:00:00-04:00) is watching this directory
  warning: concurrent instances writing to _site/ will cause missing pages and 404s
  warning: kill the other process with: kill 4659
  ```

  Stale lockfiles from crashed processes (dead PID or corrupt JSON) are removed on the next startup. The lockfile is removed on clean shutdown via signal handler.

  Previously, a backgrounded `alloy serve` and a new `alloy dev` session would silently fight over `_site/`, with `clean: true` full rebuilds wiping incremental output. Pages vanished with no errors in either console.

### Patch Changes

- Free each page's rendered HTML after writing it to disk. Alloy held every page's `RenderedBody`, cached HTML string, and alternate format bodies in memory for the full build lifetime. A 3,000-page site averaging 500KB of output carried ~1.5GB in page bodies. Now only the largest single page sits in memory at once: O(total site HTML) becomes O(largest page).

  `CaptureRenderedContent` (used by `BuildWithContent` tests) snapshots the HTML map before the output writing loop, so release does not affect test infrastructure. Sitemap generation and cache building read page metadata, not rendered bodies.
- Reduce peak memory on large sites. Alloy kept a duplicate of every page's rendered HTML in memory after writing it to disk. A 3,000-page site averaging 500KB of output carried ~1.5GB in the duplicate alone. Production builds and `alloy dev` skip the duplicate.

  Trim the `onBuildComplete` hook payload to `{ pageCount, duration, errors, outputDir }`. Alloy previously piped the full rendered HTML of every page to plugins over IPC on each build. Plugins that need output content can read `_site/` from disk.

## v0.5.0 (2026-07-17)

### Minor Changes

- `onPagesReady` hooks accept a second return shape, `{ addPages: [...] }`, for injecting virtual pages without round-tripping the entire pages array through the plugin bridge.

  ```javascript
  alloy.hook('onPagesReady', { data: ["elements"], pages: false }, function(payload) {
    var newPages = payload.siteData.elements.map(function(el) {
      return {
        path: 'demos/' + el.slug + '.md',
        url: '/demos/' + el.slug + '/',
        frontMatter: { title: el.name, layout: 'default' },
        content: '# ' + el.name
      };
    });
    return { addPages: newPages };
  });
  ```

  With `pages: false`, the plugin receives only `siteData`. Alloy skips serialization of existing pages and appends the `addPages` entries Go-side, cutting the O(N) cost of returning all pages.

  Virtual pages from `addPages` flow through the remaining pipeline: taxonomy collection, content rendering, layout resolution, and output writing. They appear in `taxonomies.*` template variables and count toward `PageCount`.

  The `{ pages: [...] }` return shape still works for plugins that mutate existing pages. The two shapes are mutually exclusive: returning both produces a build error. Returning an unrecognized key (e.g., `{ newPages: [...] }`) now errors instead of silently dropping pages.
- Alloy loads data files from subdirectories of `data/` into nested template namespaces. `data/nav/main.yaml` becomes `site.data.nav.main`. Nesting goes to any depth: `data/api/v2/endpoints.yaml` becomes `site.data.api.v2.endpoints`.

  ```yaml
  # data/nav/main.yaml
  items:
    - label: Home
      url: /
    - label: About
      url: /about/
  ```

  ```liquid
  {% for item in site.data.nav.main.items %}
    <a href="{{ item.url }}">{{ item.label }}</a>
  {% endfor %}
  ```

  Place root-level data files alongside subdirectory namespaces in the same `data/` directory. A file and directory sharing the same stem (`nav.yaml` alongside a `nav/` directory) produces a build error, matching the collision behavior for same-stem files in different formats. Alloy skips empty subdirectories.

  Alloy skipped subdirectories of `data/` without warning in prior releases. Organizing files into folders produced no template output despite the documented "any structure" guidance.
- Go template block shortcodes use Hugo-style `{{% tag "args" %}}...{{% /tag %}}` delimiters. A preprocessor runs after Goldmark and before Go template rendering — it pairs opening and closing tags, extracts quoted arguments, passes inner HTML to the shortcode callback, and replaces the block with the callback's output.

  ```markdown
  {{% callout "warning" %}}
  This is **important** content with [links](/).
  {{% /callout %}}
  ```

  Nesting resolves innermost-first. Same-name nesting (`{{% box %}}{{% box %}}...{{% /box %}}{{% /box %}}`) uses depth tracking. Delimiters inside `<pre>` and `<code>` elements are treated as literal text. Unclosed tags, mismatched names, and callback errors produce build errors.

  Goldmark now treats standalone `{{% tag %}}` lines as block-level nodes, preventing `<p>` wrapping. Inner content between paired tags is Markdown-processed before reaching the preprocessor.

  Previously, Go template block shortcodes always received empty `content` because `goEngine.AddTag` hard-coded it to `""`.
- `onAssetProcess` fires once per asset file with `{path, content}` instead of once per build with directory paths. The returned `content` key replaces the file in the output directory.

  ```javascript
  alloy.hook("onAssetProcess", {}, (asset) => {
    if (asset.path.endsWith('.css')) {
      return { content: minifyCSS(asset.content) };
    }
    return asset;
  });
  ```

  Return `null`, `undefined`, or an object without a `content` key to keep the original file. The build ignores any `path` key you return. Hook errors stop the build.

  Before this change, the hook received `{assetsDir, outputDir}` directory paths and discarded the return value. Plugins that followed the docs were silent no-ops.
- `onConfig` hooks can mutate pipeline config. The return value is applied back to `cfg` for a mutable allowlist: `build.output`, `build.clean`, `structure.content`, `structure.layouts`, `structure.assets`, `structure.static`, `structure.data`, `passthrough`, `plugins.workers`, and `plugins.timeout`.

  ```javascript
  alloy.hook("onConfig", {}, (config) => {
    config.build.output = "dist";
    config.structure.content = "pages";
    return config;
  });
  ```

  Fields outside the allowlist (`title`, `baseURL`, `language`, etc.) are silently ignored. Returning a non-object produces a build error. Multiple `onConfig` hooks from separate plugins chain in priority order — each receives the previous hook's return value.

  Previously the return value was discarded and mutations had no effect.
- `alloy.source(name, fn)` registers a data source handler in Node plugins. Configure `type: "plugin"` in `sources:` to route data acquisition through the handler instead of a REST or GraphQL endpoint.

  ```yaml
  # alloy.config.yaml
  sources:
    blog:
      type: "plugin"
      plugin: "cms-posts"
      cache: 3600
      as: "blog"
  ```

  ```javascript
  // plugins/cms.js
  export const runtime = "node";
  export default function(alloy) {
    alloy.source("cms-posts", async (config) => {
      const resp = await fetch("https://api.example.com/posts");
      return resp.json();
    });
  }
  ```

  Returned data merges into `site.data` under the `as` key (or the source map key when `as` is omitted). Templates access it like any other data source: `site.data.blog.size`, `{% for post in site.data.blog %}`.

  Alloy caches plugin source results to `.alloy/fetch-cache/` using the same TTL and `--refetch` semantics as REST sources. A source handler error aborts the build. Duplicate `alloy.source()` calls for the same name produce a warning; the last registration wins.

  Source calls enforce a 5-second timeout matching `plugins.timeout`. Slow handlers produce a timeout error instead of blocking the build.
- Liquid shortcode arguments resolve variables from the template context. Unquoted arguments like `{% youtube page.videoId %}` look up `page.videoId` in the template context and pass the resolved value to the shortcode callback. Quoted arguments remain literal strings. Dotted paths traverse nested maps: `page.videoId` resolves to the `videoId` key inside the `page` map. Non-string values are converted to strings. When an unquoted argument does not match any context variable, it falls back to its literal token string.

  ```liquid
  {% assign vid = "dQw4w9WgXcQ" %}
  {% youtube vid %}          <!-- resolves vid to "dQw4w9WgXcQ" -->
  {% youtube "hardcoded" %}  <!-- stays literal "hardcoded" -->
  {% youtube page.videoId %} <!-- resolves nested path -->
  ```

  Mixed quoted and unquoted arguments work in the same tag: `{% card "primary" page.size %}` passes `"primary"` as a literal and resolves `page.size` from context.

  Shortcodes returning an empty string now produce no output in Liquid, matching Go template behavior. Previously, empty-returning shortcodes emitted an `<alloy-shortcode>` placeholder element into production HTML.
- Check for newer Alloy releases with `alloy version --check`. Alloy queries the GitHub Releases API and compares the latest tag against the running binary.

  ```
  alloy version --check
  ```

  Set `updateCheck: true` in the config file to receive a one-line notification when `alloy dev` or `alloy serve` starts and a newer version exists. Alloy caches the result for 24 hours at `~/.config/alloy/update-check.json` (respects `XDG_CONFIG_HOME`) and runs the check in the background without blocking server startup. `alloy build` never checks for updates.

  ```yaml
  # alloy.config.yaml
  updateCheck: true
  ```

  Update checking defaults to off. Alloy makes no outbound request unless you opt in via the config or use `--check`.
- `onBeforeValidation` receives `{ outputPaths: [...] }` and runs immediately before conflict detection. Plugins register additional output paths via `addOutputs`, and those paths feed into `DetectConflicts()`.

  ```javascript
  alloy.hook("onBeforeValidation", {}, (payload) => {
    return {
      addOutputs: {
        "sitemap.xml": "plugin:sitemap",
        "robots.txt": "plugin:sitemap"
      }
    };
  });
  ```

  `onAfterValidation` receives `{ outputPaths: [...], cascade: { ...siteData... } }` after conflict detection passes. Cascade mutations merge into site data for templates. The pipeline ignores `outputPaths` changes in the return.

  ```javascript
  alloy.hook("onAfterValidation", {}, (payload) => {
    payload.cascade.buildTimestamp = new Date().toISOString();
    return payload;
  });
  ```

  Both hooks reject unrecognized return keys and type-check `addOutputs`/`cascade` as maps. Omitting a return value is a valid no-op for observation-only use.

  Previously, the pipeline fired both hooks before content discovery with a stub payload and threw away the return values.
- `onPagesReady` virtual pages accept a `dependencies` array of project-root-relative file paths. On incremental rebuilds, Alloy re-renders virtual pages whose dependencies appear in `changedFiles` and skips the rest.

  ```javascript
  alloy.hook('onPagesReady', { pages: false }, function() {
    const demoFiles = glob.sync('elements/*/demo/*.html');
    const pages = demoFiles.map(file => ({
      path: 'demos/' + path.basename(file),
      url: '/demos/' + path.basename(file, '.html') + '/',
      dependencies: [file],
      frontMatter: { layout: 'demo', markdown: false },
      content: fs.readFileSync(file, 'utf-8')
    }));
    return { addPages: pages };
  });
  ```

  - `dependencies: ['a.html', 'b.css']` — re-render when a listed file changes, skip otherwise
  - `dependencies: []` — skip (no local file deps to invalidate)
  - no `dependencies` field — re-render on all incremental rebuilds (pre-#1058 behavior)

  A site with 400 file-derived virtual pages previously re-rendered all 400 per incremental rebuild. Declaring dependencies narrows that to the pages whose source files changed.

### Patch Changes

- Fix batch hooks firing a spurious timeout warning when called with 0 items. The effective timeout was calculated as `timeout * itemCount`, which produced a 0ms timeout that expired instantly. Alloy now skips the hook when there are no payloads to process. This surfaces during incremental rebuilds where scope filtering leaves 0 pages for post-render hooks like `onPageRendered`.
- `onContentLoaded` now applies `html` mutations back to page state via `SetRenderedBody`. Previously only `frontMatter` changes were merged back; `html` mutations were silently dropped.

  ```javascript
  alloy.hook("onContentLoaded", { pages: true, pageFields: ["*"] }, (pages) => {
    for (const page of pages) {
      page.html = page.html + "<footer>Injected</footer>";
    }
    return pages;
  });
  ```

  Both `html` and `frontMatter` mutations work independently or together in the same hook call. The fix applies to both `Build()` and `BuildIncremental()`.
- `onConfig` hooks that set `passthrough[].from` or `passthrough[].to` to an absolute path or a `..` traversal above the project root now produce a build error. The validator rejects `from: "."` (would copy the entire project root into the output directory). `to: "."` and `to: ""` remain valid, meaning "root of the output directory." Error messages include the zero-based array index and field name (e.g. `passthrough[2].from`).

  Passthrough path validation runs before the config is applied, so a bad passthrough entry cannot half-mutate `build.output` or `structure.*` fields.

  Previously, a plugin could set `passthrough[].from` to `/etc/shadow` to exfiltrate files into the output directory, or `passthrough[].to` to `../../evil` to write files outside it.
- `onConfig` hooks that set `build.output` or any `structure.*` field to an absolute path, a `..` traversal above the project root, `.`, or an empty string now produce a build error. Valid relative paths with embedded `..` segments that resolve within the project (e.g. `subdir/../dist`) are accepted and cleaned before use. On Windows, reserved device names (`NUL`, `CON`) and volume-relative paths (`C:..`) are also rejected via `filepath.IsLocal`.

  All path fields are validated before any are applied to the config, so a validation failure on one field cannot leave the config partially mutated.

  Previously, a plugin could set `structure.content = "/etc"` or `build.output = "../../evil"` via `onConfig` and the values flowed through `resolveDir` unchecked. With `clean: true` (the default), `CleanOutputDir` would run `os.RemoveAll` on directories outside the project tree.
- Fix omitted `pages` scope defaulting to "all pages" instead of "none". Hooks registered with `{}` or `{data: ["elements"]}` produced a spurious validation warning on pageless events like `onConfig` and `onBuildComplete`. Batch hooks also serialized all pages when the plugin had not requested page data. Plugins that want pages must declare `pages: true`.
- Plugin hooks that exceed their timeout no longer cause a panic during build teardown. `Close()` waits for any in-flight hook, filter, or shortcode call to finish before releasing the QuickJS runtime.

  Previously, a timed-out plugin hook could trigger an `out of bounds memory access` panic at the end of `Build()` because the runtime was freed while the hook was still executing.
- Fix Node bridge protocol corruption when a plugin or its dependencies call `process.stdout.write()`. The bridge captures the real `process.stdout.write` at startup and redirects subsequent stdout writes to stderr. `sendMessage` retains the only reference to real stdout, so plugin code and npm dependencies cannot inject bytes into the JSON-RPC channel.

  When non-frame bytes reach the Go frame reader (e.g., a child process inheriting stdout), the error message includes a bounded snippet of the offending output and points to stdout pollution as the cause. Replaces the generic "missing Content-Length header" message.
- Fix `alloy dev` skipping virtual pages from `onPagesReady` plugins on incremental rebuilds. Alloy tracks virtual page paths across rebuilds, so plugin-generated pages (demos, API docs, CMS-driven content) re-render when source files change instead of serving stale content.

## v0.4.2 (2026-07-11)

- Fix codeblock render hook over-escaping `"` and `'` to `&#34;` and `&#39;` in `markup.inner`, breaking shiki and other downstream highlighters that don't decode quote entities. Codeblock inner content is element content (inside `<alloy-code>…</alloy-code>`), where only `&`, `<`, and `>` need escaping — quote characters are safe and must pass through as literal characters. The `markup.language` field continues to use full HTML attribute escaping since it lands in a `lang="…"` attribute.

## v0.4.1 (2026-07-11)

- Fix raw HTML blocks inside blockquotes and tables rendering as live markup when blockquote or table render hooks are active. Alloy entity-encodes `<script>`, `<div>`, and other HTML blocks before the hook template runs. Inline formatting like bold and emphasis renders normally.
- Fix render hook templates receiving unescaped HTML in context fields. Alloy HTML-escapes all `markup.*` string values before the hook template runs, covering codeblock, link, image, and heading hooks. A `<script>` tag inside a fenced code block, an `&` in a link URL, `"` in image alt text, or `<beta>` in a heading display as code text instead of executing or rendering as HTML.

  Heading `markup.inner` preserves goldmark's own formatting (`<strong>`, `<em>`) while escaping user-supplied raw HTML. Heading `markup.id` is escaped unconditionally, covering both the auto-generated slug path and the `{id="..."}` attribute override which can contain raw `&`, `<`, and `"`.

## v0.4.0 (2026-07-11)

### Minor Changes

- Markdown block elements now support `{.class #id key=value}` attribute syntax beyond headings. Fenced code blocks accept attributes on the opening fence line, while blockquotes and tables accept them on the trailing line.

  ````markdown
  ```go {.highlight #example}
  fmt.Println("hello")
  ```

  > This is important
  {.callout}

  | Name  | Role     |
  | ----- | -------- |
  | Alice | Engineer |
  {.striped}
  ````

  Attributes are available in render hooks via `markup.attributes` for all block element types — headings, fenced code blocks, blockquotes, and tables. When no attributes are present, `markup.attributes` is an empty map.

  ```liquid
  <!-- layouts/render-codeblock.liquid -->
  <pre class="{{ markup.attributes.class }}" data-lang="{{ markup.language }}">{{ markup.inner }}</pre>
  ```

  Block-level attributes are automatically enabled when `autoHeadingID` is true (the default). No additional configuration is needed.
- Add `include` function to the Go template engine, providing parity with Liquid's `{% include %}` for cross-file template inclusion.

  ```gotemplate
  {{ include "partials/header" }}
  {{ include "partials/card" (dict "item" . "compact" true) }}
  {{ $nav := include "partials/nav" }}
  ```

  Includes resolve from the layouts directory (`.html` extension, then raw name), support nested calls, and default to the current context when no argument is given.
- Add `limit` array filter that returns the first N elements, giving Go templates parity with Liquid.

  Go template:

  ```gotemplate
  {{ range limit .collections.blog 5 }}
    <h2>{{ .data.title }}</h2>
  {{ end }}
  ```

  Liquid:

  ```liquid
  {% comment %} limit filter — assign first, then loop {% endcomment %}
  {% assign recent = collections.blog | limit: 5 %}
  {% for post in recent %}
    <h2>{{ post.data.title }}</h2>
  {% endfor %}

  {% comment %} limit: is also a built-in parameter of the for tag {% endcomment %}
  {% for post in collections.blog limit: 5 %}
    <h2>{{ post.data.title }}</h2>
  {% endfor %}
  ```
- Liquid layout resolution now falls back to bare extensions when `.liquid` files are missing. For each candidate in the lookup chain, the Liquid engine tries `.liquid` first then the bare extension (e.g., `default.html`, `single.json`) and parses the result as Liquid.

  This applies to standard page layouts, output format layouts (`single.json.liquid` → `single.json`), taxonomy layouts, and parent layouts in layout chains.

  Layout names with a recognized extension (e.g., `layout: "base.html"`, `layout: "feed.xml"`) are now used as literal filenames — no engine extension is appended. Bare names without an extension (e.g., `layout: "base"`) get the `.liquid` → `.html` fallback.
- **Breaking:** Remove automatic filename-based layout matching from layout resolution. Previously, `content/docs/getting-started.md` would automatically match `layouts/getting-started.liquid` — creating ambiguity when multiple content directories had pages with the same filename.

  Layouts are now resolved via three mechanisms only:

  1. Explicit `layout:` in front matter or `_data.yaml` cascade
  2. Date-based convention (`post.liquid` for section children, `<section>.liquid` for index pages)
  3. `default.liquid` fallback

  If you relied on filename matching, add an explicit `layout:` to your front matter or `_data.yaml`:

  ```yaml
  # content/docs/getting-started.md
  ---
  layout: "getting-started"
  ---
  ```

  ```yaml
  # content/docs/_data.yaml — applies to all pages in docs/
  layout: "docs-page"
  ```
- The `permalinks:` key in `alloy.config.yaml` has been removed. Permalink patterns are now set exclusively via `_data.yaml` cascade files in each section directory. Existing config files that still contain a `permalinks:` key will load without error — the key is silently ignored.

  Before (no longer supported):

  ```yaml
  # alloy.config.yaml
  permalinks:
    blog: "/:year/:month/:slug/"
  ```

  After:

  ```yaml
  # content/blog/_data.yaml
  permalink: "/:year/:month/:slug/"
  ```
- Render hook templates now receive richer context for links and headings.

  Link hooks receive `markup.title` from the Markdown link title syntax `[text](url "title")`.

  ```liquid
  <a href="{{ markup.destination }}" title="{{ markup.title }}">{{ markup.text }}</a>
  ```

  Heading hooks receive `markup.inner` as rendered HTML (preserving inline formatting like `<strong>`), `markup.text` as plain text, and `markup.attributes` as a map of goldmark attributes from `{.class #id key=value}` syntax. `markup.id` uses the explicit `{#custom-id}` attribute when present, falling back to the auto-generated slug.

  ```liquid
  <h{{ markup.level }} id="{{ markup.id }}" class="{{ markup.attributes.class }}">
    {{ markup.inner }}
  </h{{ markup.level }}>
  ```
- Sitemap generation can now be disabled site-wide with `sitemap: false` in the config file. When omitted, sitemaps are generated by default.

  ```yaml
  # alloy.config.yaml
  sitemap: false
  ```
- Front matter permalinks can now use full template syntax with `{{ }}` expressions. When a permalink contains `{{`, it is rendered through the configured template engine (Liquid or Go templates) with a `page.*` context containing all front matter fields, date, slug, summary, and collection.

  ```yaml
  # content/blog/hello.md
  ---
  title: Hello World
  slug: hello-world
  lang: en
  permalink: "/{{ page.lang }}/{{ page.slug }}/"
  ---
  ```

  ```yaml
  # Go template engine (templates.engine: "gotemplate")
  ---
  permalink: "/posts/{{ .page.title | slugify }}/"
  ---
  ```

  Template and token syntax are separate modes. When `{{` is detected, token syntax (`:year`, `:slug`) is not resolved — the entire string is a template expression. A template permalink that renders to an empty or whitespace-only string is a fatal build error, distinct from `permalink: false` which is an intentional opt-out.

  Pagination template permalinks now respect the configured engine. Previously, `engine: "gotemplate"` pagination pages fell back to Liquid for permalink rendering.
- TOC generation can now be disabled site-wide with `content.markdown.toc: false` in the config file. When omitted, `page.toc` is populated for all Markdown pages by default.

  ```yaml
  # alloy.config.yaml
  content:
    markdown:
      toc: false
  ```
- Format layouts (JSON, XML, etc.) now follow the same predictable lookup order as HTML layouts. The format name is inserted before the template extension, so a single bare layout name drives all output formats.

  The `layout` front-matter value must be a bare name with no extension:

  ```yaml
  layout: article        # correct — resolves to article.json.liquid, article.xml.liquid, etc.
  # layout: article.liquid  # build error — extension-bearing names cannot be used with format outputs
  outputs: [html, json]
  ```

  Using an extension-bearing layout name (e.g., `article.liquid`, `feed.xml`) with format outputs is now a build error. The error message tells you what to fix:

  > extension-bearing layout "article.liquid" cannot be used with format outputs; use `layout: article` instead

  ### With `layout` set in front matter

  Alloy looks for the named layout with the format infixed. For `layout: article` with JSON output:

  1. `layouts/article.json.liquid`
  2. `layouts/article.json` (bare-extension fallback)

  If neither exists, the build errors with the layout name, page, and format.

  ### Without `layout` in front matter

  Alloy walks the auto candidate chain. For a blog post with JSON output:

  1. `layouts/post.json.liquid` — date-based section child
  2. `layouts/post.json` — bare-extension fallback
  3. `layouts/my-post.json.liquid` — matches the content filename
  4. `layouts/my-post.json` — bare-extension fallback
  5. `layouts/default.json.liquid` — final fallback
  6. `layouts/default.json` — bare-extension fallback

  Each candidate tries `.format.liquid` first, then the bare format extension. Higher-priority candidates (including their bare fallback) are checked before lower-priority ones.

  Cascade layouts from `_data.yaml` also apply to format outputs, with front-matter taking priority as expected.

### Patch Changes

- Accept `"go"` as an alias for `"gotemplate"` in the `templates.engine` config field, and reject unknown engine values with a clear error instead of silently falling through to Liquid.
- Remove the extensionless (raw name) fallback from partial/include resolution in both template engines. Previously, `{% include "widget" %}` and `{{ include "widget" }}` would try `widget.liquid`, `widget.html`, and then `widget` (no extension) as a final fallback. A template file without an extension has no clear use case and is almost certainly a mistake.

  Both engines now try only recognized extensions:
  - **Liquid:** `widget.liquid`, then `widget.html`
  - **Go templates:** `widget.html` only

  Sites relying on extensionless template files will see a build error referencing the partial name.
- Fix Go template engine format layouts resolving to `name.format.html` (e.g., `default.json.html`) instead of the correct bare format extension (`default.json`, `feed.xml`). The format extension is now used directly as the file extension, so `layouts/feed.xml` renders XML output and `layouts/api.json` renders JSON output without an `.html` suffix.
- Fix language-specific `_data.yaml` permalink patterns being ignored in multi-language builds. A `permalink` set in `content/es/blog/_data.yaml` now correctly applies to pages in `content/es/blog/`, instead of falling back to the default path-based URL.
- Tighten `{% inline %}` tag validation. Paths must now start with `./` or `../` — bare paths like `{% inline "diagram.svg" %}` are rejected; use `{% inline "./diagram.svg" %}` instead. Only text-based extensions are accepted: `.svg`, `.html`, `.htm`, `.txt`, `.css`, `.js`, `.json`, `.xml`, `.toml`, `.yaml`, `.yml`, `.md`. Other file types produce a clear error, and binary types like `.png` suggest using `<img>` instead.
- Fix nested `_data.yaml` permalink patterns being silently ignored. Previously, only top-level section patterns were applied — a `content/blog/posts/_data.yaml` with `permalink: "/blog/:year/:month/:slug/"` had no effect. Permalink resolution now uses the nearest `_data.yaml` in the directory tree, so subdirectories can override their parent's URL pattern.

  ```yaml
  # content/blog/_data.yaml — simple slugs for static pages
  permalink: "/blog/:slug/"

  # content/blog/posts/_data.yaml — date-based URLs for posts
  permalink: "/blog/:year/:month/:slug/"
  ```

  A page at `content/blog/posts/first-post.md` now correctly resolves to `/blog/2026/04/first-post/` instead of falling back to the parent's `/blog/first-post/`.
- Remove the broken `fingerprint` template filter. It hashed the path string instead of file contents and emitted no renamed file, so fingerprinted URLs would 404. Use `cachebust` for query-string cache busting or a plugin for filename-rewriting fingerprinting.
- Remove dead `ResolveForSection` function from the permalink package. After issue #910 wired all permalink resolution through `ResolveFromCascade`, `ResolveForSection` had zero production call sites. Its flat `map[string]string` section→pattern lookup silently dropped nested `_data.yaml` permalink patterns — all coverage has been ported to `ResolveFromCascade` tests.
- `absolute_url` now prepends the site's configured `baseURL` automatically when no explicit argument is passed. The `url` filter prepends the path portion of `baseURL` to relative paths.

## v0.3.1 (2026-06-27)

### Patch Changes

- Make the plugins directory configurable via `structure.plugins` in the config file and `--plugins` flag in `alloy init`. Previously, the plugins directory was hardcoded to `plugins/` while all other managed directories were configurable. Also fixes plugin file changes not being detected by the dev server watcher, and a bug where nested plugin paths (e.g. `tools/plugins`) broke Node runtime project root derivation.

## v0.3.0 (2026-06-27)

### Minor Changes

- Custom elements (HTML tags with hyphens like `<alloy-code>`, `<wa-tab-group>`) are now treated as block-level HTML in Goldmark. Content inside is preserved verbatim — no markdown processing, no smart quotes, no `<p>` wrapping — and blank lines do not terminate the block.

  Configurable via `content.markdown.goldmark.customElements` (default: `true`).

  ```yaml
  # alloy.config.yaml
  content:
    markdown:
      goldmark:
        customElements: true     # treat custom elements as block-level HTML (default: true)
  ```

  ```markdown
  <!-- content/example.md -->
  <wa-tab-group>
  <wa-tab panel="one">Tab 1</wa-tab>

  <wa-tab-panel name="one">
  Panel content with "quotes" and blank lines — all preserved verbatim.
  </wa-tab-panel>
  </wa-tab-group>
  ```

### Patch Changes

- Fix Liquid delimiters in code blocks being interpreted as template syntax when render hooks replace the default `<code>` element. Delimiters are now entity-encoded in `markup.inner` before reaching the hook template.
- Fix `alloy dev` not rebuilding pages when layout partials change. Editing files like `layouts/partials/header.liquid` now correctly triggers a full rebuild instead of silently skipping all pages.
- Fix spurious warnings during `alloy dev` and `alloy serve` when atomic-write editors create temp files that vanish before the debounced watcher copy runs. Transient `os.ErrNotExist` errors are now silently skipped.

## v0.2.0 (2026-06-25)

### Minor Changes

- Sections listed in the `collections:` config now declare collection membership without requiring date-token permalink patterns. Non-blog sections like releases or changelogs can participate in `collections.*` pagination and template loops.

  ```yaml
  # alloy.config.yaml
  collections:
    releases:             # declares releases/ as a collection — no date tokens needed
      sortBy: date
      order: desc
  ```

  ```yaml
  # content/releases/_data.yaml
  permalink: "/releases/:title/"
  ```

  ```liquid
  {% for release in collections.releases %}
    <a href="{{ release.url }}">{{ release.title }}</a>
  {% endfor %}
  ```

### Patch Changes

- Fix race conditions in concurrent plugin hook execution and runtime initialization.

## v0.1.1 (2026-06-24)

### Patch Changes

- Reduce internal memory footprint by removing unused cascade data layers.

## v0.1.0 (2026-06-23)

### Minor Changes

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
