---
layout: doc
title: CLI Reference
nav_weight: 40
description: "The full Alloy CLI reference: build, dev, serve, init, and version commands with all flags and build modes."
---

Alloy ships as a single binary. All commands follow standard Unix conventions: exit 0 on success, exit 1 on failure.

```bash
alloy build
# [alloy] Built 34 pages in 53ms
```

## Commands

### `alloy build`

Runs the full build pipeline and writes output to `_site/` (or the configured output directory). This is what you deploy.

```bash
alloy build
alloy build --output dist
alloy build --config deploy/production.yaml --root .
alloy build --verbose
alloy build --profile
```

Every page is rendered, every file is written. No incremental shortcuts -- the output is deterministic and complete. Any page failure aborts the entire build with no partial output.

When `--profile` is passed, Alloy records per-stage wall-clock timings and writes `cpu.prof` and `mem.prof` to `.alloy/profiles` (or the directory specified by `--profile-dir`).

### `alloy dev`

Starts the development server with live reload. Phase 1 only, drafts visible.

```bash
alloy dev
alloy dev --port 8080
alloy dev --no-drafts
```

```
Serving at http://localhost:3000
```

Key behaviors:

- **Drafts are visible by default.** Pages with `draft: true` appear in the dev server so authors can preview work in progress. Use `--no-drafts` to hide them.
- **Incremental rebuilds.** After the initial build, file changes trigger incremental rebuilds -- only changed and invalidated pages are re-rendered. Template changes invalidate pages that use that specific template, not all pages.
- **Port auto-increment.** If the default port (3000) is occupied, Alloy tries up to 10 consecutive ports before failing.

### `alloy serve`

Starts the production server. Same pipeline as `alloy build` but keeps serving with file watching.

```bash
alloy serve
alloy serve --port 4000
alloy serve --refetch
```

Key behaviors:

- **Full pipeline.** Runs the same pipeline as `alloy build`.
- **Excludes drafts.** Draft content is hidden, matching production behavior.
- **Writes to `_site/`.** Output is written to disk and served from there.
- **Full rebuilds on change.** File changes trigger a complete pipeline rebuild (no incremental mode in serve).
- **Passthrough watching.** Changes to passthrough source directories trigger targeted file recopy, not full rebuilds.

### `alloy init`

Scaffolds a complete starter project with a config file, directory structure, default layout, index page, and stylesheet.

```bash
alloy init
alloy init my-new-site
alloy init --content pages --layouts templates
```

If an Alloy config file already exists in the target directory, the command prints a message and exits without modifying anything.

The generated project structure:

```
my-new-site/
├── alloy.config.yaml
├── content/
│   └── index.md
├── layouts/
│   └── default.liquid
├── assets/
├── static/
│   └── style.css
├── data/
└── plugins/
```

Use the `--content`, `--layouts`, `--assets`, `--static`, `--data`, and `--plugins` flags to customize directory names. Custom names are written to the `structure:` block in the generated config:

```bash
alloy init --content pages --layouts templates
```

```yaml
# alloy.config.yaml
title: "My Alloy Site"
baseURL: "http://localhost:3000"
structure:
  content: "pages"
  layouts: "templates"
```

### `alloy version`

Prints the current version and exits.

### `alloy help`

Prints usage information for all commands and flags.

## Flags

| Flag | Short | Description | Commands |
|---|---|---|---|
| `--config` | `-c` | Path to config file (default: `alloy.config.yaml`) | all |
| `--root` | `-r` | Project root directory (default: config file's directory) | all |
| `--output` | `-o` | Output directory (default: `_site`) | all |
| `--verbose` | `-v` | Verbose per-file logging | all |
| `--quiet` | `-q` | Suppress all output except errors | all |
| `--port` | `-p` | Server port (default: 3000) | dev, serve |
| `--no-drafts` | | Hide draft content (drafts visible by default) | dev |
| `--refetch` | | Bypass source cache TTL, fetch fresh data | dev, serve |
| `--profile` | | Enable per-stage timing and pprof profiling | build |
| `--profile-dir` | | Directory for profile output (default: `.alloy/profiles`) | build |
| `--content` | | Content directory name (default: `content`) | init |
| `--layouts` | | Layouts directory name (default: `layouts`) | init |
| `--assets` | | Assets directory name (default: `assets`) | init |
| `--static` | | Static directory name (default: `static`) | init |
| `--data` | | Data directory name (default: `data`) | init |
| `--plugins` | | Plugins directory name (default: `plugins`) | init |

### `--root` flag

By default, `ProjectRoot` is the directory containing the config file. The `--root` flag overrides this, making all relative `structure:` paths and `build.output` resolve against the specified directory instead.

This is essential for CI/CD environments where the config file lives in a subdirectory:

```bash
# Config is in deploy/, but paths resolve from CWD
alloy build --config deploy/production.yaml --root .
```

### `--verbose` flag

Shows per-file output with the pipeline stage, file path, and timing:

```
[alloy] discovering 34 pages found
[alloy] rendering content/index.md (128µs)
[alloy] rendering content/blog/first-post.md (169µs)
[alloy] Built 34 pages in 53ms
```

### `--quiet` flag

Suppresses all output except errors. Exit code communicates success or failure.

## Build output

**Interactive terminal (TTY):** A progress bar showing the current pipeline stage, percentage, and page count:

```
[alloy] Discovering... 33 pages found
[alloy] Rendering    ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰ 100% (33/33)
[alloy] Layouts      ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰ 100% (33/33)
[alloy] Writing      ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰ 100% (33/33)
[alloy] Built 33 pages in 52ms
```

**Non-TTY (CI/CD):** Only the summary line:

```
[alloy] Built 33 pages in 52ms
```

## Exit codes

- **Exit 0** -- command completed successfully
- **Exit 1** -- command failed (invalid config, build error, missing resource, unknown command)

## Build modes compared

| Feature | `alloy build` | `alloy serve` | `alloy dev` |
|---|---|---|---|
| Writes to `_site/` | yes | yes | yes |
| Shows drafts | no | no | yes (default) |
| File watching | no | yes (full rebuild) | yes (incremental) |
| Server | no | yes | yes |
| Use case | CI/CD, deploy | Local production preview | Active development |

Config-driven SSR is an [experimental](/experimental/ssr/) feature. When an `ssr:` block is present, `alloy build` and `alloy serve` run an extra server-side rendering phase; `alloy dev` always skips it. Plugin-based SSR via `ssr.render()` is separate and unaffected.

See also [Getting Started](/getting-started/) for installation and [Project Structure](/getting-started/project-structure/) for directory layout.
