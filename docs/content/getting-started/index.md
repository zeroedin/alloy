---
title: "Installation"
layout: "doc"
weight: 1
section: "getting-started"
description: "Install Alloy and set up your first project."
---

## Prerequisites

- **Go 1.25+** — [Download Go](https://go.dev/dl/)
- **Node.js** (optional) — Only needed for Tier 3 plugins

## Install from Source

```bash
go install github.com/zeroedin/alloy@latest
```

Or clone and build:

```bash
git clone https://github.com/zeroedin/alloy.git
cd alloy
go build -o alloy .
```

## Verify Installation

```bash
alloy version
```

## Create Your First Project

```bash
mkdir my-site
cd my-site
alloy init
```

This creates an `alloy.config.yaml` with sensible defaults:

```yaml
title: "My Alloy Site"
baseURL: "http://localhost:3000"
```

## Start the Dev Server

```bash
alloy serve
```

Open `http://localhost:3000` in your browser. The dev server watches for file changes and reloads automatically.

## Build for Production

```bash
alloy build
```

Output is written to `_site/`. Deploy this directory to any static hosting provider.

## Next Steps

- [Quickstart](/getting-started/quickstart/) — Build a complete site in 5 minutes
- [Project Structure](/getting-started/project-structure/) — Understand the directory layout
- [Configuration](/configuration/) — Customize your build
