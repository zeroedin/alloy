---
layout: doc
title: CLI Reference
---

Alloy ships as a single binary with four commands. Every command accepts global flags for project root, output directory, and build options.

```bash
alloy build                          # production build
alloy serve                          # build + local server + live reload
alloy dev                            # dev server with incremental rebuilds
alloy new blog/my-post.md            # scaffold a new content file
```

## Commands

### build

Full production build. Processes all content, copies static files, and writes output to the configured directory (default: `_site/`). Exits with a non-zero status on any error.

```bash
alloy build
alloy build --clean --ssr
alloy build -o dist/
```

### serve

Builds the site, starts a local HTTP server, and watches for file changes. When a file changes, the site rebuilds and connected browsers reload automatically via live reload. Static and passthrough files are copied to the output directory.

```bash
alloy serve
alloy serve --port 3000
alloy serve --drafts
```

### dev

Development server optimized for speed. Drafts are visible by default. Static and passthrough files are served directly from source without copying. Supports incremental rebuilds when enabled.

```bash
alloy dev
alloy dev --port 8080
alloy dev --incremental
```

### new

Scaffolds a new content file with front matter. The path is relative to the `content/` directory:

```bash
alloy new blog/my-post.md
alloy new pages/about.md
```

This creates the file with a default front matter block including `title` (derived from the filename) and `date`.

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--root <path>` | `-r` | `.` | Project root directory |
| `--output <path>` | `-o` | `_site` | Override output directory |
| `--ssr` | | off | Enable server-side rendering |
| `--incremental` | | off | Enable incremental builds (dev mode) |
| `--refetch` | | off | Bypass external data cache on startup |
| `--clean` | | off | Clean output directory before build |
| `--drafts` | | off | Include draft content in output |
| `--port <number>` | | `4000` | Server port for dev and serve |

Flags can be combined freely:

```bash
alloy build --clean --ssr --output dist/
alloy dev --port 3000 --drafts --incremental
```

## Common workflows

### Production build with SSR

Clean the output directory, build with server-side rendering enabled:

```bash
alloy build --clean --ssr
```

### Development with drafts

Start the dev server showing draft content on a custom port:

```bash
alloy dev --drafts --port 3000
```

### Custom output directory

Build to a `dist/` directory instead of the default `_site/`:

```bash
alloy build -o dist/
```

### Fresh external data

Bypass the external data cache to pull fresh data from APIs and remote sources:

```bash
alloy build --refetch
```

### Incremental dev builds

Enable incremental rebuilds to only re-render pages affected by a change:

```bash
alloy dev --incremental
```
