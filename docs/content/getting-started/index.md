---
layout: doc
title: Installation
nav_weight: 10
---

Install Alloy with a single `go install` command:

```bash
go install github.com/zeroedin/alloy@latest
```

This downloads, compiles, and installs the `alloy` binary to your `$GOPATH/bin` directory. Verify the installation:

```bash
alloy version
```

## Prerequisites

- **Go 1.21 or later.** Alloy uses modern Go features and module dependencies that require 1.21+. Check your version with `go version`.
- **Node.js (optional).** Only required if your project uses Tier 3 plugins (Node subprocess plugins). The core binary has zero runtime dependencies.

## Build from source

Clone the repository and build directly:

```bash
git clone https://github.com/zeroedin/alloy.git
cd alloy
go build -o alloy .
```

Move the binary to a directory on your `PATH`:

```bash
mv alloy /usr/local/bin/
```

Or install it directly into your `$GOPATH/bin`:

```bash
go install .
```

## Verify your installation

Create a minimal site and run a build:

```bash
mkdir my-site && cd my-site
alloy init
alloy build
```

`alloy init` creates a default `alloy.config.yaml` with a title and base URL. `alloy build` runs the pipeline and writes output to `_site/`. A project with no content produces a successful zero-page build.

## What's next

- [Quickstart](/getting-started/quickstart/) -- Build a blog from scratch in 5 minutes
- [Project Structure](/getting-started/project-structure/) -- Understand the directory layout
- [CLI Reference](/cli/) -- All commands and flags
