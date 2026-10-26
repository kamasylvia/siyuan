<p align="center">
<img alt="SiYuan" src="https://b3log.org/images/brand/siyuan-128.png">
</p>

<h1 align="center">SiYuan（fork）</h1>

<p align="center">
<a href="README.md">English</a>
| <b>中文</b>
| <a href="README.ja.md">日本語</a>
| <a href="README.tr.md">Türkçe</a>
</p>

本仓库是 [siyuan-note/siyuan](https://github.com/siyuan-note/siyuan) 的个人 fork，跟随上游 stable 版本并叠加自用增强。上游项目、官方发布与文档请见原仓库。

## 与上游的差异

基于上游 v3.8.8 stable：

- **查询嵌入块表格模式** — 查询嵌入块可渲染为表格（index / 文件名 / 目录三列），支持 raw SQL 与最近查看（recent-viewed）两种渲染模式
- **文档树分页** — 带首页/末页按钮的分页控件
- **PDF 内联预览** — 文档中链接的 PDF 资产经 pdf.js 内联预览
- **纯 HTTP 远程内核** — 允许连接纯 HTTP 远程内核与 MCP OAuth endpoints
- **fork 构建管线** — 每个 `v*` tag 经 GitHub Actions 构建无签名（ad-hoc）arm64 macOS DMG，并发布多架构 Docker 镜像到 GHCR

其余功能与上游一致，fork 跟随上游 stable 版本更新。

## 安装

- **macOS（Apple Silicon）**：从 [Releases](https://github.com/kamasylvia/siyuan/releases) 下载 `siyuan-<version>-mac-arm64.dmg`，挂载后将 `SiYuan.app` 拷入 `/Applications`。构建为 ad-hoc 无签名，若 Gatekeeper 拦截首次启动，执行 `xattr -dr com.apple.quarantine /Applications/SiYuan.app`
- **Docker**：`ghcr.io/kamasylvia/siyuan` — 含 `v` 前缀版本 tag 与 `latest`，amd64 + arm64 双架构

fork 前端特性仅随 fork 客户端分发。连接 fork Docker 镜像提供的远程工作空间时须使用 fork 客户端——官方客户端永远不包含 fork 前端。

## 构建

推送 `v*` tag 触发 GitHub Actions：「Fork macOS Client」工作流产出 DMG（artifact + release），「Docker Image (fork)」工作流推送镜像到 GHCR。

## 许可

AGPL-3.0，沿用上游项目许可。
