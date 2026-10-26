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

Based on upstream v3.8.8 stable:

- **Query embed block table mode** — query embed blocks can render as a table with index, file name, and directory columns, and support raw SQL and recent-viewed render modes
- **Document tree pagination** — pager controls with first and last page buttons
- **Inline PDF preview** — PDF assets linked in documents preview inline via pdf.js
- **Plain HTTP remote kernels** — connections to plain HTTP remote kernels and MCP OAuth endpoints are allowed
- **Fork build pipeline** — every `v*` tag builds an unsigned (ad-hoc) arm64 macOS DMG through GitHub Actions and publishes multi-arch Docker images to GHCR

Everything else matches upstream; the fork follows upstream stable releases.

## Installation

- **macOS (Apple Silicon)**: download `siyuan-<version>-mac-arm64.dmg` from [Releases](https://github.com/kamasylvia/siyuan/releases), mount it, and copy `SiYuan.app` into `/Applications`. The build is ad-hoc unsigned; if Gatekeeper blocks the first launch, run `xattr -dr com.apple.quarantine /Applications/SiYuan.app`
- **Docker**: `ghcr.io/kamasylvia/siyuan` — `v`-prefixed version tags plus `latest`, amd64 and arm64

The fork frontend features ship only with the fork client. When connecting to a remote workspace served by the fork Docker image, use the fork client; the official client never includes the fork frontend.

## Build

Pushing a `v*` tag triggers GitHub Actions: the "Fork macOS Client" workflow produces the DMG (artifact and release), and the "Docker Image (fork)" workflow pushes the image to GHCR.

## License

AGPL-3.0, inherited from the upstream project.
