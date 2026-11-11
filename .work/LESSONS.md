# LESSONS — SiYuan fork

## 2026-11-09

### 1. 前端 IPC 必须检查返回值,否则「点了没反应」

`switchLocal` 菜单点了毫无反应,根因不是功能没实现,而是主进程返回
`{error: "Invalid connection manager sender"}` 被前端直接丢弃。
**规则**:凡是 `ipcRenderer.invoke` 的调用,必须 `then` 里检查 `result?.error` 并 `showMessage`。
静默失败比报错更费时间——用户只能描述「没效果」。

### 2. 连接切换的退出路径不能等渲染进程

`app.quit()` 在远程模式下挂起:远程模式的前端来自对端内核,不参与
`SIYUAN_SAVE_CLOSE` 握手,`before-quit` 里 `event.preventDefault()` 之后就再没人推进。
**规则**:凡是「不打算保留当前会话」的退出(切换连接、切换工作区),直接在 before-quit
里销毁窗口,不要走保存确认流程。判断依据就是 `state.exit.pendingConnectionArgs` 是否存在。

### 3. sender 校验要覆盖真实调用方

连接管理器的 `isFromWindow` 只认它自己的对话框窗口,而新命令来自工作空间主窗口。
**规则**:给 IPC 加新命令前,先确认发送方属于哪个信任集合;校验逻辑要注入而不是
在模块内硬编码,否则跨目录(remote/ 不能 import window/)就只能放弃校验或抄一份。

### 4. 契约字段漏加会让功能静默失效

`backupTab` 的保存流程逐字段转换到 `apicontract.BackupTarget`,而契约里没有 `schedule`,
定时备份保存后字段被丢弃——UI 看起来保存成功,实际没存。
**规则**:新增内核配置字段时,同时检查 `apicontract` 结构体 + `unmask*` 转换 + 前端
readForm/saveForm 三处;只改其中一处等于没做。

### 5. 语言文件插入按键名定位,不要按值

`backupRestoreForceLabel` 在 22 个语言里值各不同,用英文值当锚点导致 18 个文件静默跳过。
**规则**:插入 i18n 键时按键名 `findIndex(line.includes('"key"'))`,写完全量 python 校验
22 文件里目标键存在;不要相信「脚本没报错就是成功」。

### 6. 全模块构建才算验证内核

`go build ./api/` 通过,但 CLI 包(`kernel/cli/cmd`)还调用旧签名,只有 `go build ./...`
(Docker 构建用的就是它)才能发现。**规则**:内核改动一律跑 `go build ./...`,不要只 build 改动的包。

### 7. 还原身份默认策略

还原会把快照的 `conf.json` 一起铺下来,原样保留会让本地副本继承源实例的 `system.id`
与锁屏口令——认领冲突、直接同步身份冲突、本地被远端口令锁住。
**规则**:还原默认换新身份(`system.id` 重新生成 + `accessAuthCode` 清空),保留原样必须是
显式勾选,且文案要写清「使用快照的系统标识与锁屏口令」而不是含糊的「保留」。

### 8. 整库级改写认准标杆:checkout(全量) vs 增量同步

还原/导入/快照检出改写整库→ 标杆是 `checkoutRepo` 全量路径,不是增量 `processSyncMergeResult`。
但同步内完成索引的序列(`FullReindexDirect` + `IndexRefs` + `FlushQueue` + `ResetVirtualBlockRefCache`)
在增量保护内路径(`repository.go:2940`)有现成样板,直接复用该序列,不要自己拼。
**规则**:整库改写先找 checkout 级标杆;保护内同步索引抄 2940 四步序列。

### 9. 索引失败判定用元数据状态,不要自己扫库

`indexBox` 内部失败只记日志不返回错误;`treenode.DocumentMetadataState(boxID)` 是现成
就绪信号(`ready` vs 其他),`document_source_archive.go:224` 已有 `!= "ready"` 即报错先例。
**规则**:需判断索引成败时查该状态;用户指南与未解锁加密笔记本沿启动语义跳过。

### 10. 失败载荷走 DataOnError,不要塞 msg 字符串

契约端点 `ResponseOptions{DataOnError: true}` + `FailureWithData` 让失败响应携带类型化
data(生成器/schema/前端类型全链路);塞 msg 字符串前端只能展示不能分支。
**规则**:失败时调用方需分支的数据一律 DataOnError,msg 只放人读文案。

### 11. Infisical 拉取管线禁 `IFS='=' read`,尾 `=` 会被吞

`while IFS='=' read -r k v` 读 `KEY=...=` 时 v 丢尾 `=`(47 vs 48 字符实证),
Portainer token 缺 padding 报 401 Invalid JWT,误判成服务端轮换查半天。
**规则**:改 `while IFS= read -r line; export "${line%%=*}...` 整行读 + 参数展开切首个 `=`。
⚠️ 全局 AGENTS §2.3.2 同段管线有同 bug,待修全局(本机已验证修法)。

### 12. skopeo 中转 NAS 标准三步(2026-11-09 .13 实测)

`skopeo copy --override-arch amd64 --override-os linux --src-creds kamasylvia:$GITHUB_CLASSIC_PAT
docker://ghcr.io/...:tag docker-archive:/tmp/x.tar:tag`(缺 override-os 报 OS darwin 无镜像)
→ `POST /api/endpoints/1/docker/images/load`(Content-Type application/x-tar,33s/322MB)
→ tag 丢失变 `tag:latest`,须 `POST /images/<错名>/tag?repo=...&tag=...`(201) 纠正
→ 纠正时留下的 `tag:latest` 畸形残根与新 tag 共存 = 双 repository 引用,Portainer 按 ID 删镜像
  必失败(force 也压不住:「cannot be forced) - image is referenced in multiple repositories」),
  须逐 tag `DELETE /images/<repo:tag>`,最后一个 tag 摘除时镜像层才回收(2026-11-11 alpha.7.13
  残根 335MB 实测)
→ PUT stack update(pullImage=false)→ 轮询 Status=1。直拉 Extracting 中途 curl 超时后服务端未必续传,miss 即转中转,勿死等。

### 13. 本地单测内核启动三坑

① 新 CLI 须 `serve` 子命令;② 必须在打包布局下起(同级 appearance/stage/guide,可用 /Applications 软链 farm);③ API 鉴权用 `Conf.Api.Token`(conf.json api.token),锁屏密码只过 BasicAuth 不认 `Authorization: Token`。
