<p align="center">
<img alt="SiYuan" src="https://b3log.org/images/brand/siyuan-128.png">
</p>

<h1 align="center">SiYuan (fork)</h1>

<p align="center">
<b>English</b>
| <a href="README.zh-CN.md">中文</a>
| <a href="README.ja.md">日本語</a>
| <a href="README.tr.md">Türkçe</a>
</p>

This repository is a personal fork of [siyuan-note/siyuan](https://github.com/siyuan-note/siyuan). It tracks upstream stable releases and adds self-use enhancements. For the upstream project, official releases, and documentation, see the original repository.

## Differences from upstream

Based on upstream v3.8.10 stable:

- **Backup subsystem** — multi-target backups (PostgreSQL, S3, WebDAV) with content-addressed, deduplicated snapshots, snapshot management (per-snapshot delete with reference-counted blob cleanup, clear-all, export zip), deletion and edit gates that prevent orphan snapshots, per-target cron scheduling, an offline "Save zip to local" exit, and restore with automatic data-index rebuild
- **Maintenance read-only lock** — while a backup or restore runs, workspace writes are rejected kernel-side and editors flip to read-only until the lock releases
- **Remote kernel connection enhancements** — connection manager with history and per-address trust, plain HTTP remote kernels and MCP OAuth over plain HTTP, auto-reconnect to the last remote workspace on plain launch, a switch-to-local-workspace exit, and remote exports saved through a native dialog
- **Offline read-only for remote workspaces** — browsed documents stay readable offline (document cache, tree cache with graying, external-image caching, a cache manager, plugin resilience with auto-snooze and offline deferral)
- **Query embed block table mode** — query embed blocks can render as a table with index, file name, and directory columns, and support raw SQL and recent-viewed render modes
- **Inline PDF preview** — PDF assets linked in documents preview inline via pdf.js
- **Copy readable path with notebook name** — the copied readable path includes the notebook name
- **Fork build pipeline** — every `v*` tag builds an unsigned (ad-hoc) arm64 macOS DMG through GitHub Actions and publishes multi-arch Docker images to GHCR

Everything else matches upstream; the fork follows upstream stable releases. The backup subsystem design, constraints and behavior are documented in issue #15; the remote kernel connection enhancements in issue #16.

## Installation

- **macOS (Apple Silicon)**: download `siyuan-<version>-mac-arm64.dmg` from [Releases](https://github.com/kamasylvia/siyuan/releases), mount it, and copy `SiYuan.app` into `/Applications`. The build is ad-hoc unsigned; if Gatekeeper blocks the first launch, run `xattr -dr com.apple.quarantine /Applications/SiYuan.app`
- **Docker**: `ghcr.io/kamasylvia/siyuan` — `v`-prefixed version tags plus `latest`, amd64 and arm64

The fork frontend features ship only with the fork client. When connecting to a remote workspace served by the fork Docker image, use the fork client; the official client never includes the fork frontend.

## Build

Pushing a `v*` tag triggers GitHub Actions: the "Fork macOS Client" workflow produces the DMG (artifact and release), and the "Docker Image (fork)" workflow pushes the image to GHCR.

## License

AGPL-3.0, inherited from the upstream project.
