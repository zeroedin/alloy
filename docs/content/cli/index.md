---
layout: doc
title: CLI Reference
nav_weight: 40
---

Alloy ships as a single binary. All commands follow standard Unix conventions: exit 0 on success, exit 1 on failure.

```bash
alloy build
# [alloy] Built 480 pages in 1.2s
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

The pipeline runs Phase 1 (content rendering) and Phase 2 (SSR, if `ssr:` is configured). Every page is rendered, every file is written. No incremental shortcuts -- the output is deterministic and complete. Any page failure aborts the entire build with no partial output.

When `--profile` is passed, Alloy records per-stage wall-clock timings and writes `cpu.prof` and `mem.prof` to `.alloy/profiles` (or the directory specified by `--profile-dir`).

### `alloy dev`

Starts the development server with live reload. Phase 1 only, drafts visible.

```bash
alloy dev
alloy dev --port 8080
alloy dev --no-drafts
```

```
[alloy] Built 420 pages in 1.8s
Serving at http://localhost:3000
```

Key behaviors:

- **Drafts are visible by default.** Pages with `draft: true` appear in the dev server so authors can preview work in progress. Use `--no-drafts` to hide them.
- **Incremental rebuilds.** After the initial build, file changes trigger incremental rebuilds -- only changed and invalidated pages are re-rendered. Template changes invalidate pages that use that specific template, not all pages.
- **In-memory rendering.** Rendered pages are held in memory, not written to `_site/`. Static and passthrough files are served directly from their source locations.
- **SSR is skipped.** The dev server runs Phase 1 only. Web Components render client-side.
- **Port auto-increment.** If the default port (3000) is occupied, Alloy tries up to 10 consecutive ports before failing.

### `alloy serve`

Starts the production server. Same pipeline as `alloy build` but keeps serving with file watching.

```bash
alloy serve
alloy serve --port 4000
alloy serve --refetch
```

Key behaviors:

- **Full pipeline.** Runs the same Phase 1 + Phase 2 pipeline as `alloy build`. SSR runs if configured.
- **Excludes drafts.** Draft content is hidden, matching production behavior.
- **Writes to `_site/`.** Output is written to disk and served from there.
- **Full rebuilds on change.** File changes trigger a complete pipeline rebuild (no incremental mode in serve).
- **Passthrough watching.** Changes to passthrough source directories trigger targeted file recopy, not full rebuilds.

### `alloy init`

Creates a default `alloy.config.yaml` in the target directory.

```bash
alloy init
alloy init my-new-site
```

Fails with exit 1 if `alloy.config.yaml` already exists. The generated config contains a valid `title` and `baseURL`:

```yaml
title: "My Alloy Site"
baseURL: "http://localhost:3000"
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
| `--output` | `-o` | Output directory (default: `_site`) | build, serve |
| `--verbose` | `-v` | Verbose per-file logging | all |
| `--quiet` | `-q` | Suppress all output except errors | all |
| `--port` | `-p` | Server port (default: 3000) | dev, serve |
| `--no-drafts` | | Hide draft content (drafts visible by default) | dev |
| `--refetch` | | Bypass source cache TTL, fetch fresh data | dev, serve |
| `--profile` | | Enable per-stage timing and pprof profiling | build |
| `--profile-dir` | | Directory for profile output (default: `.alloy/profiles`) | build |

### `--root` flag

By default, `ProjectRoot` is the directory containing the config file. The `--root` flag overrides this, making all relative `structure:` paths and `build.output` resolve against the specified directory instead.

This is essential for CI/CD environments where the config file lives in a subdirectory:

```bash
# Config is in deploy/, but paths resolve from CWD
alloy build --config deploy/production.yaml --root .
```

### `--verbose` flag

Replaces the progress bar with per-file output showing the pipeline stage, file path, and per-file timing:

```
[alloy] render content/index.md (12ms)
[alloy] render content/blog/first-post.md (8ms)
[alloy] ssr    content/components/card.md (45ms)
[alloy] write  _site/index.html
[alloy] Built 420 pages in 1.8s
```

### `--quiet` flag

Suppresses all output except errors. No progress bar, no summary line. Exit code communicates success or failure.

## Build progress

**Interactive terminal (TTY):** A progress bar showing the current pipeline stage, percentage, and page count:

```
[alloy] Discovering content... 480 pages found
[alloy] Rendering    [=========================] 100% (480/480)
[alloy] Layouts      [=========================] 100% (480/480)
[alloy] Writing      [=========================] 100% (480/480)
[alloy] Built 480 pages in 12.3s
```

**Piped output (CI/CD):** Only the final summary line, keeping logs clean:

```
[alloy] Built 420 pages in 1.8s
```

**Incremental rebuilds (dev mode):** A single summary line with timestamp:

```
[alloy] 12:34:58 Rebuilt 3 pages in 47ms (417 cached)
```

## Exit codes

- **Exit 0** -- command completed successfully
- **Exit 1** -- command failed (invalid config, build error, missing resource, unknown command)

## Build modes compared

| Feature | `alloy build` | `alloy serve` | `alloy dev` |
|---|---|---|---|
| Writes to `_site/` | yes | yes | no (in-memory) |
| Runs SSR (Phase 2) | yes (if configured) | yes (if configured) | no |
| Shows drafts | no | no | yes (default) |
| File watching | no | yes (full rebuild) | yes (incremental) |
| Server | no | yes | yes |
| Use case | CI/CD, deploy | Local production preview | Active development |

See also [Getting Started](/getting-started/) for installation and [Project Structure](/getting-started/project-structure/) for directory layout.
