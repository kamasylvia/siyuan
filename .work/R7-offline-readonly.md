# R7 离线方案(离线可读 → 离线编辑 → 续网同步)

> 双份台账之一:思源侧 = `Obsidian/10-Resources/11-CS/04-Applications/Note/SiYuan/R7 离线方案(离线可读→离线编辑→续网同步)`(doc id `20261027223609-jh24nhz`,经 MCP 更新)。
> 推进时同步更新两侧进度(勾选任务 + 追加进度行)。

## 🎯 终态愿景

远程空间断网后体验趋近 [inkstone](https://github.com/shuaiplus/inkstone):已缓存文档可读(含图片/PDF 附件)、后续版本可继续编辑,重连后自动同步离线修改,冲突时明确提示且不丢数据。云笔记模型不变——数据主权在 NAS,本地只有缓存与待同步队列。

**技术路线(方案 B,2026-10-27 用户裁决):Service Worker + Cache API + IndexedDB 统一实现,一套代码三端通吃——Electron 客户端、浏览器、PWA**(不再用 Electron fs/protocol 专属层)。

## 🧱 架构(方案 B)

- **SW 层(`stage/service-worker.js`,内核侧)**:官方已有 PWA 底子(预缓存静态资源)。扩展拦截 `/api/filetree/getDoc`(POST 可缓存)与 `/assets/**`,网络优先 + 后台落缓存,断网回缓存命中;SW 由内核 serve,浏览器与 Electron 远程模式同源加载,**NAS 更新镜像即三端生效**
- **渲染层(bundle)**:离线检测、横幅、只读锁、缓存索引、二期 outbox UI——随前端 bundle 发布(Electron 需客户端更新,浏览器直接生效)
- **Electron main 小改**:远程模式注册 SW(现仅 BROWSER 分支注册);远程请求策略放行 SW 通道
- **存储**:Cache API 存响应;IndexedDB 存索引(docID → updated 时间戳、LRU 元数据);LRU 500MB(storage.estimate 辅助)

## ⚠️ 关键设计难点(二期)

inkstone 写模型是「整篇 content + rev」,重放简单;**思源是块级 transaction(do/undo 操作序列)**——离线 transaction 重放到重连后的文档时,文档可能已被其它端修改。冲突策略:入队时快照(文档 updated + 涉及块 ID);重放前校验——快照未变直接提交,块已删/文档已变转冲突态(导出本地兜底,不静默丢);状态角标四态(在线同步/离线可编辑待同步 N 笔/同步中/冲突待处理)。

## 一期:离线可读(文档内容 + assets 附件)

纯客户端+SW 实现零内核逻辑改动(SW 文件在 stage/ 属静态资源),仅远程工作空间启用。

- 断网检测:`window online/offline` 事件 + WS onclose;恢复 = online 事件 + WS onopen
- 离线横幅「离线只读」+ 全部 Protyle 只读 + transaction 拦截提示
- getDoc 响应缓存(打开即写,LRU 500MB);离线打开命中渲染(只读),未命中提示「未缓存」
- assets 缓存:SW 拦截,断网图片/PDF 可读;按文档 updated 联动失效
- 重连:文档与 assets 缓存按 `updated` 校验失效重拉

### 任务清单

- [x] T1 offlineCore 状态机(渲染层纯函数 + 单测)
- [x] T2 断网/恢复信号接入(三端)
- [x] T3 离线横幅 UI + i18n 全语言(3 键 ×22 语言,check-lang-keys 过)
- [x] T4 只读锁 + transaction 拦截(fetch 咽喉 + getAllEditor 锁/还原)
- [x] T5 getDoc 缓存写入(IndexedDB 介质;POST 拦截改 fetch 咽喉实现,覆盖全部 20 处调用)
- [x] T6 离线打开命中渲染(fetch 失败兜底读缓存,原生编辑区)
- [x] T7 SW 扩展:assets 拦截缓存(网络优先/断网回退)
- [x] T8 IndexedDB 索引 + updated 失效(在线打开自然覆盖,懒失效语义)
- [x] T9 Electron 远程模式注册 SW + 浏览器端确认原已注册
- [x] T10 LRU 500MB(evictOverLimit 内嵌写入路径,单测覆盖)
- [x] T11 lint + 定向测试 + 断网实测(lint 绿/单测 4/4;**Playwright setOffline 断网验收 8/8 PASS**(v3.8.8.13 终版):在线打开含标题缓存→植入无标题旧条目→横幅→只读锁→断网渲染→离线清单标题派生→恢复)
- [x] T12 构建发布 + 部署 + 验收(**NAS = v3.8.8.13,stack update 两步法**;release v3.8.8.13 已建;mac 客户端 dmg 视容量补传)
- [x] 迭代轮(v3.8.8.10~.13):① 新增离线缓存文档数上限设置(设-关于-离线缓存,10–1000 默认 50;*int 防旧客户端缺键误清 + editorConfigApi 保存通道);② 缓存标题真源修正(data.docInfo.ial.title——data.ial 旧路径在现行内核不存在,曾全存空串)+ 离线清单从快照派生标题并持久化;③ 存量配置 10 修正回 50

## 二期:离线编辑 + 续网同步(outbox)

- [ ] T13 outbox 存储与 schema(IndexedDB:transaction 载荷 + updated 快照 + 涉及块 ID + writeId 依赖链)
- [ ] T14 离线编辑通路(解除只读、transaction 乐观应用 + 入队)
- [ ] T15 状态角标 UI(四态 + 待同步笔数)
- [ ] T16 重连重放器(按序提交、逐笔校验、失败转冲突)
- [ ] T17 冲突处理(本地导出 + 提示)+ 重连指数退避 + 双端并发实测
- [ ] T18 构建 + 全流程验收(断网编辑 30 分钟 → 重连 → 同步成功)

## 📌 决策记录

- 2026-10-27:一期禁编辑(outbox 归二期);LRU 500MB;仅远程空间启用;assets 缓存在一期
- 2026-10-27:方案 B——SW + Cache API + IndexedDB 三端统一(Electron/浏览器/PWA),弃 Electron 专属 fs/protocol 层;SW 部署在内核 stage,镜像更新三端生效
- 2026-10-27:参考 inkstone——window online/offline 检测、状态角标、outbox/冲突兜底设计;块级 transaction 冲突校验为自研重点

## 🚧 进度

- 2026-10-27:计划落盘(本文件 + 思源 R7 文档双份)
- 2026-10-27:T1-T10 完成(commit `3d0b5bb96e` + 浏览器启用修正 `1b98efe23c`,issue #13)。实施要点:http 远程源非安全上下文 → 文档缓存介质定 IndexedDB(SW/Cache API 仅 https);getDoc 拦截从 SW 改为 fetch.ts 单一咽喉(覆盖 20 处调用);离线冷启动(断网状态下全新打开页面)不在一期范围(启动链无缓存)。v3.8.8.8 镜像已部署 NAS(v3.8.8.9 浏览器启用修正构建中),Playwright setOffline 断网验收进行中
- 2026-10-28:**T11 断网验收 6/6 PASS**(Playwright setOffline:IndexedDB 缓存写入→横幅→只读锁 editable=0→断网打开缓存文档渲染 40 节点→恢复横幅消失)。**NAS 部署方式切换为 Portainer stack update**(用户裁决入全局 AGENTS §7A:stack 37 compose 是配置事实源,禁止裸 recreate;首版 PUT 未改 image 行 + 部署 Status 3 卡住的排查:compose 实际停在 v3.8.8、pullImage 环节疑似失败,先 docker API 拉镜像再 PUT pullImage=false 成功)。release v3.8.8.9 已建;mac 客户端 bundle 排队中(容量),出产物后补 release + 替换本机
- 2026-10-28:迭代轮收尾(v3.8.8.13 部署):终验 8/8 PASS(含离线标题派生)。排查沉淀:setEditor 是平铺 Merge 语义(带 config 包裹的探针无效);契约/mapper 丢字段模式再现(settingEditorPayload 无 pdfPreview/offlineDocCacheMaxCount;FileTreeGetDocData 无顶层 ial 属正常——真源在 data.docInfo.ial,前端 onGet 经 getEmbeddedDocInfoResponse 合成)
- 2026-10-28:应用户裁决改向——拆除独立篇数设置,复用「最近的文档最大数量」(recentDocsMaxListCount);500MB 字节上限保留兜底。commit `b8d2132971`,NAS 已部署 v3.8.8.14。桌面客户端本地构建 v3.8.8.13 实测远程全链路正常(用户解除本地构建禁令已入 AGENTS/TOOLCHAIN;本地构建实操配方入 release-flow 记忆)
- 2026-10-28:**用户裁决:二期离线编辑永久推迟,待显式命令**。浏览器「不停刷新」根修:fetch 层拆除 401 盲目 reload(上游 401 经 forwardProxy 包装为 HTTP 200 已实证,循环 401 真源待 console 定位),改为去重 toast + console 记录 URL;插件韧性层(错误≥3 休眠跳过 + 集市启用/卸载解除 + 离线延迟加载)上线。NAS = v3.8.8.15,实测 http://6806 会话 401×3 → 零刷新。commit `f974cbc426`+`c61f639cc3`
- 2026-10-29:迭代轮(v3.8.8.16,`46748fa6ad`):① 缓存管理窗幽灵条目根修(listDocCache 联表校验快照库 + 孤儿清理,用户报告的「最近打开但未缓存」项根除);② 文件树离线灰显未缓存文档(MutationObserver 重标,恢复清除;点击灰项仍提示未缓存)。实测:清缓存→开 1 篇→恰好 1 条目;43 项树 42 灰 1 正常。客户端 v3.8.8.16 已装(forkBuild 对齐),NAS 已部署 v3.8.8.16,release 已含 dmg
- 2026-10-29:迭代轮(v3.8.8.18~.19,`9f19d06a6b`):① 「最近的文档最大数量」tip 补离线缓存上限语义(×22);② 父链目录预取(打开文档向上遍历 listDocsByPath,会话去重)——离线整条父链可展开,解决「父文档灰显无法定位缓存子文档」;③ 离线资产缓存(文档 assets + 外链图片共用)上限:500 条 LRU + 配额 60% 自减半;④ 废弃设置死键清理×22。NAS = 客户端 = v3.8.8.19,release 含 dmg
- 2026-10-29:收尾(v3.8.8.20,`6487c6f2b2`):离线打开页签「未命名文档」根修——openFileById 前置 /api/block/getDocInfo 离线未接管,现从缓存快照 docInfo 合成(含别名解析);用户三个问题(页签标题/父链展开/折叠)对应修复分属 .19(getDocInfo+父链预取)与 .20。NAS 已部署 v3.8.8.20,客户端已装,待用户人工验收
- 2026-10-29:灰显语义修正(v3.8.8.21,`3761802cf8`):用户报告可展开父项被灰显——灰显仅标记「死胡同」(文档未缓存 且 目录展开缓存缺失);可展开父项恢复正常色作离线导航。NAS = 客户端 = v3.8.8.21,release 已含 dmg
- 2026-10-29:**SW 陈旧 bundle 总根因修复(v3.8.8.22,`4891cee1de`)**:SW 对 /stage/ 构建资源「缓存优先」+ 缓存名以静态 SIYUAN_VERSION 为键 → 浏览器首次访问后入口 app.js 永久陈旧,**NAS 部署的新前端浏览器永远拿不到**(用户浏览器侧所有「修了但没生效」的总根因;桌面端走本地 protocol 不受影响)。修复 = 可变 /stage/ 资源网络优先+回写缓存+断网回退;不可变哈希资源/字体保持缓存优先。**浏览器验收 8/8 PASS**(SW 新版部署/网络优先三次投毒验证/死胡同灰显/离线打开页签标题/折叠/恢复)。NAS 已部署 v3.8.8.22;本地客户端 .20 内容功能等价(SW 仅浏览器生效),不重装
- 2026-11-08:备份还原交付(v3.8.10-alpha.7.1,`4977520e71`+`cabd8efd6d`):还原 UI + 三 REST 端点(listSnapshots/restoreDryRun/restore)+ 凭证掩码解析;修上游合并遗留冲突标记(blockRender.ts/zh-CN.json);docker tag 校验跟随上游 prerelease。NAS = v3.8.10-alpha.7.1,release 含 dmg。快照形态:S3/WebDAV/PG = 内容寻址 blob+manifest(非单压缩包),增量上传,还原强制 SHA-256 校验,附件/插件随 data/ 整目录遍历覆盖
- 2026-11-08:Local 备份目标移除(v3.8.10-alpha.7.3,`b571fb9376`):思源历史+PG(多实例集中仓库)+S3/WebDAV 三层定位确立后,Local 同机副本无增量保护。备份目标收敛为 PG/S3/WebDAV;LocalTarget 降级为 Store 测试夹具(engine 增注入字段);顺修 backup CLI 残留引用。NAS 已部署 v3.8.10-alpha.7.3,release 含 dmg。教训:移除类型后必须 `go build ./...`(CLI 子包引用漏测曾致 CI 构建失败一次)
- 2026-11-08:LocalTarget 降级收尾(v3.8.10-alpha.7.4,`5ae7bf031b`):实现整体移入 backup 测试包作 Store 夹具,生产代码清零。NAS 已部署 v3.8.10-alpha.7.4,release 含 dmg
- 2026-11-08:客户端重装对齐:并行会话曾装其 dev1 构建(缺还原 UI,backupRestoreTitle=0 实证)。首用 `scripts/fork-mac-build.sh v3.8.10-alpha.7.4` 一键本地构建(内置 kernel 版本串校验+OCR 镜像预热)并重装,三特征指纹核对齐全;release .4 资产修正(补正确 dmg+SHA256SUMS,删误传旧名 dmg)。客户端已重启运行
- 2026-10-29:**终验通过(CDP 全流程,v3.8.8.21 客户端 + .21 NAS)**:零缓存→在线开 2 篇→断网→横幅+死胡同灰显(9 灰/2 正常,已展开父项不灰)→离线点开缓存文档→**页签标题=真名**(getDocInfo 合成)→折叠可用→恢复解锁。之前「未命名文档」页签的根因 = openFileById 前置 getDocInfo 未接管,已修。用户此前测试的旧构建问题已随重装解决(教训:.21 构建后忘了重装 /Applications——构建、安装、启动三步必须成链验证)
- 2026-10-29:迭代轮(v3.8.8.17,`734715adfc`):用户报告管理窗裸 ID 行——根因 = 已删文档残留缓存(删除未联动清理 + 旧快照无标题)。修复 = removeDoc/removeDocs/removeDocByID 成功后经 fetch 咽喉批量清理快照/元数据/别名 + 无标题条目显示「未命名文档」(i18n×22)。NAS = v3.8.8.17,客户端已装(forkBuild 对齐),release 含 dmg
