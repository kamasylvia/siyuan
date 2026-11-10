// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package apicontract

import "mime/multipart"

type BackupInclude struct {
	Data         bool `json:"data"`
	Conf         bool `json:"conf"`
	History      bool `json:"history"`
	LocalStorage bool `json:"localStorage"`
}

type BackupRetention struct {
	KeepLast   int `json:"keepLast,omitempty" api:"optional"`
	MaxAgeDays int `json:"maxAgeDays,omitempty" api:"optional"`
}

type BackupPG struct {
	DSN            string `json:"dsn,omitempty" api:"optional"`
	Host           string `json:"host,omitempty" api:"optional"`
	Port           int    `json:"port,omitempty" api:"optional"`
	User           string `json:"user,omitempty" api:"optional"`
	Password       string `json:"password,omitempty" api:"optional"`
	Database       string `json:"database,omitempty" api:"optional"`
	SSLMode        string `json:"sslMode,omitempty" api:"optional"`
	Schema         string `json:"schema,omitempty" api:"optional"`
	Timeout        int    `json:"timeout,omitempty" api:"optional"`
	ConcurrentReqs int    `json:"concurrentReqs,omitempty" api:"optional"`
}

type BackupS3 struct {
	Endpoint      string `json:"endpoint"`
	AccessKey     string `json:"accessKey,omitempty" api:"optional"`
	SecretKey     string `json:"secretKey,omitempty" api:"optional"`
	Bucket        string `json:"bucket"`
	Region        string `json:"region,omitempty" api:"optional"`
	PathStyle     bool   `json:"pathStyle,omitempty" api:"optional"`
	SkipTLSVerify bool   `json:"skipTlsVerify,omitempty" api:"optional"`
	Timeout       int    `json:"timeout,omitempty" api:"optional"`
}

type BackupWebDAV struct {
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username,omitempty" api:"optional"`
	Password      string `json:"password,omitempty" api:"optional"`
	SkipTLSVerify bool   `json:"skipTlsVerify,omitempty" api:"optional"`
	Timeout       int    `json:"timeout,omitempty" api:"optional"`
}

type BackupTarget struct {
	ID        string           `json:"id"`
	Name      string           `json:"name,omitempty" api:"optional"`
	Type      string           `json:"type"`
	Enabled   bool             `json:"enabled"`
	PG        *BackupPG        `json:"pg,omitempty" api:"optional,nullable"`
	S3        *BackupS3        `json:"s3,omitempty" api:"optional,nullable"`
	WebDAV    *BackupWebDAV    `json:"webdav,omitempty" api:"optional,nullable"`
	Retention *BackupRetention `json:"retention,omitempty" api:"optional,nullable"`
	// Schedule 定时备份 cron 计划;nil 或空表达式表示不启用
	Schedule *BackupSchedule `json:"schedule,omitempty" api:"optional,nullable"`
}

// BackupSchedule 定时备份计划
type BackupSchedule struct {
	Cron string `json:"cron"`
}

type BackupZipSnapshotsRequest struct {
	File *multipart.FileHeader `json:"file" api:"optional"`
}

type BackupZipSnapshotsData struct {
	// ZipPath 是内核侧暂存路径,还原时原样传回
	ZipPath   string                  `json:"zipPath"`
	Snapshots []BackupSnapshotSummary `json:"snapshots"`
}

type BackupLocalZipRequest struct {
	// Passphrase 覆盖配置加密口令(仅本次 zip 生效);留空用配置值
	Passphrase string `json:"passphrase" api:"optional"`
}

type BackupLocalZipData struct {
	// Path 是 zip 在工作空间内的相对路径(temp 下),前端据此下载
	Path string `json:"path"`
}

type BackupRestoreZipRequest struct {
	ZipPath      string `json:"zipPath"`
	Snapshot     string `json:"snapshot"`
	Passphrase   string `json:"passphrase" api:"optional"`
	Force        bool   `json:"force" api:"optional"`
	KeepIdentity bool   `json:"keepIdentity" api:"optional"`
}

type BackupRestoreZipData struct {
	Files   int   `json:"files"`
	Bytes   int64 `json:"bytes"`
	Removed int   `json:"removed"`
	// FailedBoxes 是索引未就绪的笔记本 ID 列表;Reindexed 为 false 时定位陈旧范围
	FailedBoxes []string `json:"failedBoxes,omitempty"`
}

type BackupExportZipRequest struct {
	Target   string `json:"target"`
	Snapshot string `json:"snapshot"`
}

type BackupExportZipData struct {
	Path string `json:"path"`
}

type BackupEncryption struct {
	Enabled    bool   `json:"enabled"`
	Passphrase string `json:"passphrase,omitempty" api:"optional"`
}

type BackupConfig struct {
	Enabled    bool              `json:"enabled"`
	Include    BackupInclude     `json:"include"`
	Targets    []*BackupTarget   `json:"targets"`
	Encryption *BackupEncryption `json:"encryption,omitempty" api:"optional,nullable"`
}

type BackupTargetState struct {
	LastSnapshot  string `json:"lastSnapshot,omitempty"`
	LastOK        bool   `json:"lastOk"`
	LastError     string `json:"lastError,omitempty"`
	LastRun       string `json:"lastRun,omitempty"`
	UploadedBlobs int    `json:"uploadedBlobs"`
	UploadedBytes int64  `json:"uploadedBytes"`
}

type BackupState struct {
	UpdatedAt string                        `json:"updatedAt,omitempty"`
	Targets   map[string]*BackupTargetState `json:"targets"`
}

type BackupGetConfigData struct {
	Config *BackupConfig `json:"config"`
	State  *BackupState  `json:"state,omitempty"`
	// Progress 是备份/还原的在飞进度快照,空闲时为空
	Progress *BackupProgress `json:"progress,omitempty"`
}

type BackupProgress struct {
	Backup  *BackupRunProgress     `json:"backup,omitempty"`
	Restore *BackupRestoreProgress `json:"restore,omitempty"`
}

// BackupRunProgress 是在飞备份的进度:Total 为快照条目数,Done 按当前目标
// 逐一累计(上传与复用都计入)
type BackupRunProgress struct {
	StartedAt int64  `json:"startedAt"`
	Target    string `json:"target"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"`
	// Targets 是全部在飞备份占用的目标配置 ID;前端据此逐行置灰
	Targets []string `json:"targets,omitempty"`
}

// BackupRestoreProgress 是在飞还原的进度:Total 为快照文件数
type BackupRestoreProgress struct {
	StartedAt int64  `json:"startedAt"`
	Target    string `json:"target"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"`
}

type BackupSetConfigRequest struct {
	Config *BackupConfig `json:"config"`
}

type BackupTestTargetRequest struct {
	Target *BackupTarget `json:"target"`
}

type BackupTestTargetData struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type BackupRunRequest struct {
	Targets []string `json:"targets" api:"optional,nullable"`
	// Takeover 接管已被其它实例认领的目标归属,本次配置生效
	Takeover bool `json:"takeover" api:"optional"`
}

type BackupRunTarget struct {
	ID            string `json:"id"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	UploadedBlobs int    `json:"uploadedBlobs"`
	SkippedBlobs  int    `json:"skippedBlobs"`
	UploadedBytes int64  `json:"uploadedBytes"`
	Manifest      string `json:"manifest,omitempty"`
	// ClaimedBy/ClaimedHost/ClaimedAt 非空表示该目标被其它实例认领而拒绝备份
	ClaimedBy   string `json:"claimedBy,omitempty"`
	ClaimedHost string `json:"claimedHost,omitempty"`
	ClaimedAt   string `json:"claimedAt,omitempty"`
}

type BackupRunData struct {
	Snapshot   string            `json:"snapshot"`
	StartedAt  string            `json:"startedAt"`
	FinishedAt string            `json:"finishedAt"`
	Files      int               `json:"files"`
	TotalBytes int64             `json:"totalBytes"`
	Targets    []BackupRunTarget `json:"targets"`
}

type BackupListSnapshotsRequest struct {
	Target string `json:"target"`
}

type BackupSnapshotSummary struct {
	ID         string   `json:"id"`
	CreatedAt  string   `json:"createdAt"`
	Roots      []string `json:"roots"`
	Files      int      `json:"files"`
	TotalBytes int64    `json:"totalBytes"`
	Encrypted  bool     `json:"encrypted"`
}

type BackupListSnapshotsData struct {
	Target    string                  `json:"target"`
	Snapshots []BackupSnapshotSummary `json:"snapshots"`
}

type BackupRestoreDryRunRequest struct {
	Target     string `json:"target"`
	Snapshot   string `json:"snapshot"`
	Passphrase string `json:"passphrase" api:"optional"`
	// Force 与还原请求一致时,dry-run 额外报告将被清理的快照外文件规模
	Force bool `json:"force" api:"optional"`
}

type BackupRestoreDryRunData struct {
	Snapshot   string   `json:"snapshot"`
	CreatedAt  string   `json:"createdAt"`
	Roots      []string `json:"roots"`
	Files      int      `json:"files"`
	TotalBytes int64    `json:"totalBytes"`
	Encrypted  bool     `json:"encrypted"`
	// Conflicts 是目标工作空间中会被快照覆盖的已存在非空文件数;
	// RemoveCount/RemoveBytes 是 force 还原将清理的快照外文件规模
	Conflicts   int   `json:"conflicts"`
	RemoveCount int   `json:"removeCount"`
	RemoveBytes int64 `json:"removeBytes"`
}

type BackupRestoreRequest struct {
	Target     string `json:"target"`
	Snapshot   string `json:"snapshot"`
	Passphrase string `json:"passphrase" api:"optional"`
	Force      bool   `json:"force" api:"optional"`
	// KeepIdentity 为 true 时保留快照 conf.json 中的 system.id 与锁屏口令;
	// 默认为 false,还原副本重新生成实例身份,避免继承源实例造成冲突
	KeepIdentity bool `json:"keepIdentity" api:"optional"`
}

type BackupRestoreData struct {
	Snapshot string `json:"snapshot"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	Dest     string `json:"dest"`
	// Removed/RemovedBytes 是 force 还原清理掉的快照外文件规模
	Removed      int   `json:"removed"`
	RemovedBytes int64 `json:"removedBytes"`
	// Reindexed 为 true 表示落盘后内核已同步重建数据索引并完成引用解析;
	// 失败时经 FailureWithData 返回 false,调用方须按错误处理,不可视为还原可用
	Reindexed bool `json:"reindexed"`
	// FailedBoxes 是索引未就绪的笔记本 ID 列表;Reindexed 为 false 时定位陈旧范围
	FailedBoxes []string `json:"failedBoxes,omitempty"`
}

type BackupDeleteCheckRequest struct {
	// Targets 是待删除校验的目标配置 ID 列表
	Targets []string `json:"targets"`
}

type BackupDeleteCheckItem struct {
	Target string `json:"target"`
	// Deletable 为 true 表示目标确认已清空,允许删除配置
	Deletable bool `json:"deletable"`
	// Reason 为删除被拒绝的原因: snapshots=目标上仍有快照, unreachable=目标不可达且无法证明已清空
	Reason string `json:"reason,omitempty"`
	// Snapshots 是可达目标上的快照数量,不可达时为 0
	Snapshots int `json:"snapshots"`
	// Duplicate 为 true 表示该配置因与另一个可连接且已清空的目标指向同一存储而放行
	Duplicate bool `json:"duplicate"`
}

type BackupDeleteCheckData struct {
	Results []BackupDeleteCheckItem `json:"results"`
}

type BackupEditCheckRequest struct {
	// ID 是被编辑的目标配置 ID;Target 是编辑后的完整目标配置(掩码密钥可用)
	ID     string        `json:"id"`
	Target *BackupTarget `json:"target"`
}

type BackupEditCheckData struct {
	// Allowed 为 true 表示允许保存编辑后的连接
	Allowed bool `json:"allowed"`
	// Reason 为拒绝原因: snapshots=原目标仍有快照, unreachable=原目标不可达且无法证明已清空
	Reason string `json:"reason,omitempty"`
	// OldSnapshots 是原目标上的快照数量,不可达时为 0
	OldSnapshots int  `json:"oldSnapshots"`
	SameStorage  bool `json:"sameStorage"`
}

type BackupDeleteSnapshotRequest struct {
	Target   string `json:"target"`
	Snapshot string `json:"snapshot"`
}

type BackupDeleteSnapshotData struct {
	// Blobs 是随之清理的不再被其它快照引用的数据块数量
	Blobs int `json:"blobs"`
}

type BackupEmptyTargetRequest struct {
	Target string `json:"target"`
}

type BackupEmptyTargetData struct {
	// Snapshots/Blobs 是清空删除的快照与数据块数量
	Snapshots int `json:"snapshots"`
	Blobs     int `json:"blobs"`
}
