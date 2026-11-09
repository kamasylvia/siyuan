# TODO — SiYuan fork(备份子系统 + 远程客户端)

> 更新于 2026-11-09。换机继续开发时先读本文件,再读 `LESSONS.md`。
> 状态判据只认本文件;release 记录在 GitHub Releases。

## 一、当前版本线

- 上游线:upstream v3.8.10-alpha.7(客户端与容器严格版本匹配)
- 最新已发布:**v3.8.10-alpha.7.14**(tag `59509a4b7e`:还原/备份运行中禁止删快照与清空目标——内核 deleteSnapshot/emptyTarget 双 guard 加 RestoreState,面板 busy 态连 Snapshots 按钮一起置灰;.14 双端已部署,release 资产齐。前版 .13 见 git log)
- 本机客户端已装 `.13`(2026-11-09 本地构建,forkBuild + 内核哈希双指纹验证);release v3.8.10-alpha.7.13 已补 dmg + SHA256SUMS
- build script 版本自检:`scripts/fork-mac-build.sh` 会比对 `kernel/util/working.go` 的 Ver 与二进制内嵌串,不一致直接失败

## 二、下一步要做(按优先级)

### P1. 还原后重建索引 + 前端整页重载(2026-11-09 已发布 .13 并实测通过)

**实现**(`a952cabaca`,同上):
1. 内核 `model/restore_reindex.go::ReindexRestoredWorkspace`:落盘后同步 `FullReindexDirect()` + `IndexRefs()` + `FlushQueue()` + `ResetVirtualBlockRefCache()`(增量同步保护内路径 `repository.go:2940` 同序列),`lockSync` 与云同步串行;失败集按 `DocumentMetadataState != ready` 判定(用户指南/未解锁加密笔记本沿启动语义跳过)
2. 契约 `BackupRestoreData` 加 `reindexed` + `failedBoxes`,端点加 `DataOnError:true`;索引失败 → `FailureWithData(-1, msg, data)`(已落盘规模 + 失败笔记本,前端显错);成功 → `reindexed:true`
3. 前端 `backupTab.ts`:成功后调 `/api/ui/reloadUI`(与数据导入同通道),toast 换 `backupRestoreReindexed`(×22);失败走存量 error toast
4. CLI `backup restore`:dest 被运行内核锁定时 stderr 警告(重启或调 `/api/system/rebuildDataIndex`),不调索引
5. 不做:conf 热重载(内核无此能力,快照 conf 下次启动生效);claim 身份读盘同理

**实测**(2026-11-09 本机,本地内核 + NAS PG 快照 `20261109T065340Z` 4524 文件/1.4GB → 本地空库):还原返回 `reindexed:true`(139s);不重启全文搜中「SiYuan Fork 备份功能进度」✓;refs 表 72 行 ✓;getBacklink2 回链 1 条 ✓;日志 `rebuilt database` + `resolved refs [8]` ✓。NAS 侧仅 dry-run(覆盖 4521/删除 0),未动生产库。

### P2. 备份子系统剩余待实测项(用户尚未逐项确认)

已实现并发布在 alpha.7.11/7.12,但用户只确认过「页面能打开」,以下未逐项回归:

- [ ] 备份运行中:顶部「备份」「删除」+ 行内「备份」「还原」「删除」全部置灰;面板重新打开时若任务在飞也置灰
- [ ] 快照管理:行内删除有「处理中…」状态 + 成功/失败独立提示;清空全部有两步确认与进行中提示
- [ ] 删除门禁:目标有快照时拒绝删除配置;手动清空后放行;IP/域名双配置同指纹互相放行
- [ ] 编辑门禁:改连接信息时原存储有快照则拒绝;同指纹(IP↔域名)放行;只改名称/保留/定时则跳探测
- [ ] cron 定时备份:表单填 `0 2 * * *` 保存后能回显(契约字段曾丢失,已在 alpha.7.11 修复);到点是否真的触发
- [ ] 还原身份勾选:默认换新 system.id + 清锁屏口令;勾选「使用快照的系统标识与锁屏口令」时原样保留
- [ ] 单源还原:还原入口只在目标行内,对话框源目标钉死不���切换

### P3. 可选/待评估(不要未经确认就做)

- **定时还原**:用户问过「定时还原是否等于定时同步」。已给出结论:**不建议**实现到活动工作区
  (覆盖式单向写入会删掉快照后的本地编辑;与原生双向同步语义完全不同)。若将来要做,形态应是
  还原到非活动目录 / 仅在空工作区灾难恢复场景自动还原。**做之前必须先跟用户对齐**
- `backupRestoreNoTargets` 的中文文案是错的(zh-CN 写作「还原目标」,语义应为「没有可还原的目标」),顺手可修

## 三、环境与操作备忘

- 客户端构建:macOS arm64 上跑 `scripts/fork-mac-build.sh <tag>`,DMG 在 `app/build/`(构建机需 go / node / python3)
- 发布链(已验证多次):commit → stamp `app/package.json` forkBuild → tag → push → 等 `Docker Image (fork)`
  workflow → Portainer 上的 fork stack 预拉镜像 + stack update → 装 DMG → `gh release create` 带 DMG + SHA256SUMS
- 容器侧:镜像 `ghcr.io/kamasylvia/siyuan:v<tag>`;部署必须走 Portainer stack update,不能裸 recreate 容器
- 凭据:一律从 Infisical 取(具体项目/路径见本机 `~/.agents/config.toml` 的 `[infisical]` 段);本文件不记录任何 token
- 部署目标与远程内核地址:见本机配置,不在本文件记录
- 本地工作区路径:见本机 `~/.config/siyuan/workspace.json` 的最后一个条目
- 连接标记:`~/.config/siyuan/connections.json` 的 `lastMode` / `lastRemoteOrigin`;普通启动读到 remote 就走自动重连
- 远程模式回本地:工作空间菜单 →「切换到本地工作区」(alpha.7.12 起;修复提交 c8a50b2447 未发布)

## 四、已知坑

- `go build ./api/` 通过不代表全模块通过 —— Docker 构建跑 `go build ./...`,历史上 CLI 包漏改导致镜像构建失败
- 语言文件是全量 22 个;**用锚点值匹配可能失效**(各语言值不同),插入键一律按键名定位;每次改完用 python 全量校验 22 文件
- `npx pnpm run lint` 必须走 `npx -y pnpm@12.6.0`,本机 PATH 上的 pnpm 版本太老(`packages field missing or empty`)
- 本地 electron 测试基线:connectionStore 2 个 + rendererProcess 1 个在未改动基线上也失败,不是回归
