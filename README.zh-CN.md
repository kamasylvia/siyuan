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

基于上游 v3.8.10 stable：

- **备份子系统** — 多目标备份（PostgreSQL、S3、WebDAV），内容寻址去重快照；快照管理（单快照删除按引用计数清理 blob、清空全部、导出 zip）、删除与编辑门禁防孤儿快照、目标级 cron 定时备份、「保存 zip 到本地」离线出口、还原后自动重建数据索引
- **维护只读锁** — 备份或还原运行期间，内核拒绝工作空间写入请求，编辑器自动切只读，锁释放后恢复
- **远程内核连接增强** — 连接管理器（历史与逐地址信任）、纯 HTTP 远程内核与 MCP OAuth over plain HTTP、普通启动自动重连上次远程、一键切换到本地工作区、远程导出走系统保存对话框
- **远程工作空间离线只读** — 浏览过的文档离线可读（文档缓存、树缓存与灰化、外链图片缓存、缓存管理面板、插件韧性自动休眠与离线推迟）
- **查询嵌入块表格模式** — 查询嵌入块可渲染为表格（index / 文件名 / 目录三列），支持 raw SQL 与最近查看（recent-viewed）两种渲染模式
- **PDF 内联预览** — 文档中链接的 PDF 资产经 pdf.js 内联预览
- **复制可读路径含笔记本名** — 复制的可读路径包含笔记本名
- **fork 构建管线** — 每个 `v*` tag 经 GitHub Actions 构建无签名（ad-hoc）arm64 macOS DMG，并发布多架构 Docker 镜像到 GHCR

其余功能与上游一致，fork 跟随上游 stable 版本更新。备份子系统的设计与约束见 issue #15，远程内核连接增强见 issue #16。

## 安装

- **macOS（Apple Silicon）**：从 [Releases](https://github.com/kamasylvia/siyuan/releases) 下载 `siyuan-<version>-mac-arm64.dmg`，挂载后将 `SiYuan.app` 拷入 `/Applications`。构建为 ad-hoc 无签名，若 Gatekeeper 拦截首次启动，执行 `xattr -dr com.apple.quarantine /Applications/SiYuan.app`
- **Docker**：`ghcr.io/kamasylvia/siyuan` — 含 `v` 前缀版本 tag 与 `latest`，amd64 + arm64 双架构

fork 前端特性仅随 fork 客户端分发。连接 fork Docker 镜像提供的远程工作空间时须使用 fork 客户端——官方客户端永远不包含 fork 前端。

## 构建

推送 `v*` tag 触发 GitHub Actions：「Fork macOS Client」工作流产出 DMG（artifact + release），「Docker Image (fork)」工作流推送镜像到 GHCR。

## 许可

AGPL-3.0，沿用上游项目许可。
