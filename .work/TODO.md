# TODO — SiYuan fork(备份子系统 + 远程客户端)

> 更新于 2026-11-10(mac 本机,发布 alpha.7.16/.17 后)。换机继续开发时先读本文件,再读 `LESSONS.md`。
> 状态判据只认本文件;release 记录在 GitHub Releases。

## 一、当前版本线

- 上游线:upstream v3.8.10 stable(已合并,fork 跟随上游 stable 纪元)
- 最新已发布:**v3.8.10.4**(tag `dbc1a92cec`;快照时间改浏览器本地时区显示——修「设了 02:00
  cron 却看不到凌晨快照」的误读,快照 ID 仍 UTC;`backupRestoreNoTargets` 错译修正。双端已部署,
  DMG 280MB 已装本机,release 为 Latest,资产 2 个)
- 前版 **v3.8.10.3**(tag `1ae2743bea`):命名空间改内核自动分配 ULID(面板只读展示已分配值;
  分配后稳定,存量非法值仍拒;22 语言同步)
- 前版 **v3.8.10.2**(tag `96fb0edbd6`):命名空间作用域 + PG 目标移除 + 整行灰 + 删除结果弹窗
- 发版实测:2026-11-10 本机全量验证(go test backup+apicontract 绿、lint 绿、契约 785 pairs 绿)
- 设计文档:**issue #15「Backup subsystem: design, constraints and behavior」**——备份子系统全部
  设计与约束的权威记录,后续开发先读它
- 其余功能 issue:#16 远程内核连接增强(设计与行为)、#17 查询嵌入渲染模式(raw SQL/最近浏览)、
  #18 复制可读路径含笔记本名;#13 评论区有离线子系统 as-built 补记
- 本机客户端已装 v3.8.10.4,远程模式连接正常(2026-11-11 验证:app.log got remote kernel version 3.8.10)
- 设计文档:**issue #15「Backup subsystem: design, constraints and behavior」**——备份子系统全部
  设计与约束的权威记录,后续开发先读它
- 其余功能 issue:#16 远程内核连接增强(设计与行为)、#17 查询嵌入渲染模式(raw SQL/最近浏览)、
  #18 复制可读路径含笔记本名;#13 评论区有离线子系统 as-built 补记
- 文档树分页:fork 自有分页已随 upstream v3.8.10 合并移除(merge 3e9ef0e677),当前文件树走上游新方案;无残留代码,不开 issue 亦无后续动作
- 本机客户端已装 alpha.7.19,远程模式连接正常
- 版本线历程:.13 = P1 还原后重建索引+重载(实测 139s/4524 文件,不重启即可搜索);.14 = 快照删除互斥;
  .15 = 逐目标 busy 锁定(restore progress 记 target id,置灰收窄到该行)+存储标识唯一(Normalize 按
  storageIdentity 查重,重复配置 setConfig/编辑直接拒绝);.16 = 保存前连接测试门禁(表单级,指纹绑定,
  连接字段一变即失效;名称/保留/定时改动不触发);.17 = 本轮(见「二」)
- build script 版本自检:`scripts/fork-mac-build.sh` 会比对 `kernel/util/working.go` 的 Ver 与二进制内嵌串,不一致直接失败

## 二、alpha.7.16–.18 已实现,待逐项实测

### A. 只读维护锁(用户定案:备份期间编辑不放行)

- 备份 = 共享锁(引用计数,并行允许,`backup.lock` flock 已移除);还原 = 独占锁
- 任一持有期间,gin 中间件拒绝全部写入路由(transactions/notebook/filetree/asset/attr/setting/
  system setConf/sync/storage 写/storage setCriterion/format/clipboard/inbox/tag/export/ai/mcp),
  消息 "a backup or restore is running; the workspace is read-only"
- 锁状态变化经 `SetMaintenanceChangeHook` 广播:复用既有 `readonly` WS 通道翻转编辑器只读 +
  `msg` 通道进入提示;退出自动恢复编辑
- `runBackup` API 拒绝还原进行中;还原 handler 拒绝备份/还原进行中

### B. cron 定时备份

- Target.Schedule{cron},`Normalize()` 校验(无效拒绝/空白清 nil);内核 `StartScheduler`
  每分钟检查,窗口内有触发表即到期执行;busy 时跳过本轮(错过不补跑,标准 cron 语义);
  serve.go 挂载;前端表单「定时备份 (cron)」字段(留空=不启用)
- 注意:UI 保存流程曾丢 schedule 字段(契约缺字段),alpha.7.11 已修复
- 2026-11-11 首次真实 cron 触发确认:`0 2 * * *` 准点产出快照(本地 02:01:52 = UTC 18:01:52Z);
  快照时间此前按 UTC 原样显示,用户误以为凌晨快照缺失 → 快照管理/还原下拉/zip 还原三处
  改为浏览器本地时区显示(formatSnapshotTime,快照 ID 仍为 UTC 标识符不动)

### C. zip 三入口(格式 = 存储对象集 manifest+blobs,纯 ASCII 条目名,零乱码,可回灌)

1. 「保存 zip 到本地」按钮(alpha.7.18):内核即时全新快照 → zip 二进制流回客户端落盘,
   零目标可用,本地/远程统一;原 LocalZipDir 自动产出配置已移除(按钮取代)
2. 快照管理行「导出 zip」:把已连目标快照打包下载(契约 Data=BinaryContent,
   ResponseOptions{Output: BinaryOutput, ErrorStatus: 500},前端 blob 下载)
3. 「从 zip 还原」:备份面板顶部按钮 → multipart 上传 `zipSnapshots` 解析 → 快照确认对话框
   (口令/收敛/身份勾选)→ `restoreZip` → 重建索引 → 前端重载,与 REST 还原同流程

### D. 锁状态与前端联动

- 契约 BackupRunProgress 加 `targets []string`(在飞备份占用的目标 ID)
- 前端 `setBackupBusy(on, busyTargets)` 语义:还原在飞(busyTargets 空)→ 全部动作灰;
  备份在飞 → 仅在飞目标行灰(备份/还原/删除/快照管理/编辑/测试连接),其余行备份亮(可并行);
  顶部「备份」还原时灰、「删除」任何维护时灰
- 内核 `runBackup` 拒绝与在飞目标交集非空的请求(消息列出冲突 ID);还原独占由
  MarkRestoreStarted CAS + restoreBackup/restoreZip/runBackup 三个入口守卫

## 三、P1. 还原后重建索引 + 前端整页重载(2026-11-09 已发布 .13 并实测通过)

**实现**(`a952cabaca`):
1. 内核 `model/restore_reindex.go::ReindexRestoredWorkspace`:落盘后同步 `FullReindexDirect()` + `IndexRefs()` + `FlushQueue()` + `ResetVirtualBlockRefCache()`(增量同步保护内路径 `repository.go:2940` 同序列),`lockSync` 与云同步串行;失败集按 `DocumentMetadataState != ready` 判定(用户指南/未解锁加密笔记本沿启动语义跳过)
2. 契约 `BackupRestoreData` 加 `reindexed` + `failedBoxes`,端点加 `DataOnError:true`;索引失败 → `FailureWithData(-1, msg, data)`(已落盘规模 + 失败笔记本,前端显错);成功 → `reindexed:true`
3. 前端 `backupTab.ts`:成功后调 `/api/ui/reloadUI`(与数据导入同通道),toast 换 `backupRestoreReindexed`(×22);失败走存量 error toast
4. CLI `backup restore`:dest 被运行内核锁定时 stderr 警告(重启或调 `/api/system/rebuildDataIndex`),不调索引
5. 不做:conf 热重载(内核无此能力,快照 conf 下次启动生效);claim 身份读盘同理

**实测**(2026-11-09 本机,本地内核 + NAS PG 快照 `20261109T065340Z` 4524 文件/1.4GB → 本地空库):还原返回 `reindexed:true`(139s);不重启全文搜中「SiYuan Fork 备份功能进度」✓;refs 表 72 行 ✓;getBacklink2 回链 1 条 ✓;日志 `rebuilt database` + `resolved refs [8]` ✓。NAS 侧仅 dry-run(覆盖 4521/删除 0),未动生产库。

## 四、P2. 备份子系统剩余待实测项(用户尚未逐项确认)

- [ ] 备份运行中:在飞目标行整行灰(.17 语义),其余行备份按钮亮(可并行);同目标重复启动被内核拒绝并列出 ID
- [ ] 只读:备份/还原期间编辑文档 → 保存被拒并提示只读;编辑器进入只读态(readonly 广播);结束自动恢复
- [ ] 快照管理:行内删除有「处理中…」状态 + 成功/失败独立提示;清空全部有两步确认与进行中提示
- [ ] 删除门禁:目标有快照时拒绝删除配置;手动清空后放行;同指纹互放行(alpha.7.15 起 unique storage identity 规则变化:同存储双配置新建会被 setConfig 拒绝,已有双配置编辑走指纹放行——两规则并存,注意用户反馈)
- [ ] 编辑门禁:改连接信息时原存储有快照则拒绝;同指纹放行;只改名称/保留/定时则跳探测
- [ ] cron 定时备份:表单填 `0 2 * * *` 保存后能回显;到点是否真的触发
- [ ] zip 三入口:「保存 zip 到本地」按钮(本地/远程两模式);快照管理行内「导出 zip」(alpha.7.19 新接线);
  从 zip 还原(口令/收敛/身份勾选)→ 不重启可搜索
- [ ] 还原身份勾选:默认换新 system.id + 清锁屏口令;勾选「使用快照的系统标识与锁屏口令」时原样保留
- [ ] 单源还原:还原入口只在目标行内,对话框源目标钉死不允切换
- [ ] switchLocal:工作空间菜单「切换到本地工作区」远程模式生效(修复提交 c8a50b2447,已实测,待用户复测)

## 五、可选/待评估(不要未经确认就做)

- **定时还原**:结论已给出——不做活动工作区的自动还原(覆盖式破坏,会删快照后的本地编辑;与原生
  双向块级合并同步完全不同)。若将来要做,形态应为还原到非活动目录 / 仅空工作区灾难恢复。
  **做之前必须先跟用户对齐**
- zip 目录的「列表+下载」UI(远程实例 zip 在容器卷内,目前经 exportZip 下载目标快照可用;
  zip 目录本身的列表/下载列为可选)
- `backupRestoreNoTargets` 中文文案错译(zh-CN「还原目标」→ 应为「没有可还原的目标」),顺手可修

## 六、环境与操作备忘

- 客户端构建:macOS arm64 上 `scripts/fork-mac-build.sh <tag>`,DMG 在 `app/build/`(需 go/node/python3)
- 发布链:commit → stamp forkBuild → tag → push → 等 `Docker Image (fork)` → Portainer fork stack
  预拉镜像 + stack update → 装 DMG → `gh release create` 带 DMG + SHA256SUMS
- 容器侧:镜像 `ghcr.io/kamasylvia/siyuan:v<tag>`;必须 Portainer stack update,不能裸 recreate
- 凭据:一律 Infisical(本机 `~/.agents/config.toml` [infisical] 段);本文件不记录 token
- 部署目标与远程内核地址:见本机配置,不入库
- 本地工作区路径:本机 `~/.config/siyuan/workspace.json` 最后条目
- 连接标记:`~/.config/siyuan/connections.json` `lastMode`/`lastRemoteOrigin`;普通启动读到 remote 即自动重连
- 远程模式回本地:工作空间菜单 →「切换到本地工作区」(重启带 --workspace,标记翻转为本地)

### 发布产物清理(2026-11-11)

- Release 保留锚点制:当前线全保(`v3.8.10.1/.2/.3`)+ 各已终结版本线末代(alpha.7.19 / 3.8.9.6 / 3.8.8.23);
  中间版本删 release 时带 `--cleanup-tag` 一并删远端 tag(39 → 6,回收 ~8.9GB)
- GHCR 清理**必须先算保护 digest**:保留 tag 的镜像都是 OCI index,含 4 个 child
  (amd64/arm64 manifest + 2 attestation),这些 child 在 versions API 里就是 untagged/平台 tag 版本;
  盲删 untagged 会打坏 `latest`。做法:对每个保留 tag 拉 `/v2/<repo>/manifests/<tag>` 收集 child digest
  → 与 736 个 version 全集比对 → 删除非保护项(736 → 33,零失败),删完逐个 tag 验 HTTP 200
- 远端仍有 22 个从未建 release 的 orphan tag(`v3.8.8.3/.4/.6` 等),不占存储,保留不动;
  本地只删远端已不存在的 33 个残 tag

## 七、已知坑

- `go build ./api/` 过 ≠ 全模块过 —— 一律 `go build ./...`(CLI 包历史上漏过,alpha.7.10 Docker 失败根因)
- 跨包调用必须带包限定符:serve.go(package server)无限名调用 api 包函数报 undefined,易误判为构建缓存问题
- 语言文件 22 个;插入 i18n 键一律按键名定位(**不要按值锚点**,各语言值不同);每次改完 python 全量校验 22 文件
- 前端 `ipcRenderer.invoke` 必须检查返回值里的 error,否则静默失败(switchLocal 首版教训)
- 连接切换的退出路径不能等渲染进程:远程模式前端来自对端,不参与保存握手;before-quit 里按
  `state.exit.pendingConnectionArgs` 直接销毁窗口
- 契约字段漏加会让功能静默失效(schedule 曾被 UI 保存流程丢弃):内核配置字段新增时同步检查
  apicontract 结构体 + unmask 转换 + 前端 readForm/saveForm 三处
- `npx pnpm run lint` 必须走 `npx -y pnpm@12.6.0`,本机 PATH 上的 pnpm 版本太老(`packages field missing or empty`)
- 本地 electron 测试基线:connectionStore 2 个 + rendererProcess 1 个在未改动基线上也失败,不是回归
- tag 移签:`git tag -f` + `git push --force-with-lease=refs/tags/<tag>:<旧值>`(轻量 tag 远端值为旧 commit)
