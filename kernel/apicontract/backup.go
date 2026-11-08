// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package apicontract

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
}

type BackupRunTarget struct {
	ID            string `json:"id"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	UploadedBlobs int    `json:"uploadedBlobs"`
	SkippedBlobs  int    `json:"skippedBlobs"`
	UploadedBytes int64  `json:"uploadedBytes"`
	Manifest      string `json:"manifest,omitempty"`
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
}

type BackupRestoreData struct {
	Snapshot string `json:"snapshot"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	Dest     string `json:"dest"`
	// Removed/RemovedBytes 是 force 还原清理掉的快照外文件规模
	Removed      int   `json:"removed"`
	RemovedBytes int64 `json:"removedBytes"`
}
