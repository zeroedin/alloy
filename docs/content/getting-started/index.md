---
layout: doc
title: Installation
nav_weight: 10
description: "Install Alloy via Homebrew, Go install, or from source on macOS, Linux, and Windows, then scaffold your first project."
---

## Homebrew (recommended)

```bash
brew tap zeroedin/alloy-ssg
brew install alloy-ssg
```

This installs a prebuilt binary for macOS (Apple Silicon and Intel) or Linux (amd64 and arm64). Verify the installation:

```bash
alloy version
```

See [Upgrading Alloy](/upgrade/) for upgrade instructions.

## Windows

On Windows, use [WSL](https://learn.microsoft.com/en-us/windows/wsl/install) and install via Homebrew inside your Linux environment. Alternatively, download a prebuilt binary from the [GitHub releases page](https://github.com/zeroedin/alloy/releases):

1. Download `alloy-windows-amd64.zip` (or `alloy-windows-arm64.zip` for ARM)
2. Extract the zip
3. Move `alloy.exe` to a directory on your `PATH`

## Go install

If you already have a Go toolchain, you can install from source:

```bash
go install github.com/zeroedin/alloy@latest
```

This compiles and installs the `alloy` binary to your `$GOPATH/bin` directory. Requires Go 1.25 or later.

## Build from source

```bash
git clone https://github.com/zeroedin/alloy.git
cd alloy
go build -o alloy .
```

Move the binary to a directory on your `PATH`:

```bash
mv alloy /usr/local/bin/
```

## Prerequisites

- **Node.js (optional).** Only required if your project uses Tier 3 plugins (Node subprocess plugins). The core binary has zero runtime dependencies.

## Verify your installation

```bash
alloy init my-site && cd my-site
alloy build
```

`alloy init` scaffolds a complete starter project -- config file, directories, a default layout, an index page, and a stylesheet. `alloy build` runs the pipeline and writes output to `_site/`.

## What's next

- [Quickstart](/getting-started/quickstart/) -- Build a blog from scratch in 5 minutes
- [Project Structure](/getting-started/project-structure/) -- Understand the directory layout
- [CLI Reference](/cli/) -- All commands and flags
