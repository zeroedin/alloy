---
layout: doc
title: Upgrading Alloy
nav_weight: 90
description: "How to upgrade Alloy to the latest version for each install method."
---

Upgrade instructions for each install method. Run `alloy version` after upgrading to confirm the new version.

## Homebrew

```bash
brew upgrade alloy
```

## Binary download

Download the latest release for your platform from the [GitHub releases page](https://github.com/zeroedin/alloy/releases/latest). Replace the existing `alloy` binary in your `PATH`:

```bash
# macOS / Linux
curl -L https://github.com/zeroedin/alloy/releases/latest/download/alloy-$(uname -s)-$(uname -m).tar.gz | tar xz
sudo mv alloy /usr/local/bin/alloy
```

## From source

```bash
git pull origin main
go build -o alloy .
```

Move the built binary to your `PATH` if it is not already there.

## Verifying the upgrade

```bash
alloy version
```
