// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/siyuan/kernel/apicontract"
	"github.com/siyuan-note/siyuan/kernel/backup"
	"github.com/siyuan-note/siyuan/kernel/model"
	"github.com/siyuan-note/siyuan/kernel/util"
)

const backupMasked = "***"

var pgPasswordPattern = regexp.MustCompile(`(://[^:/@]+:)([^@]+)(@)`)
var pgKeywordPasswordPattern = regexp.MustCompile(`(?i)(password=)\S+`)

func backupConfigPath() string {
	return filepath.Join(util.ConfDir, "backup.json")
}

func loadBackupConfigForAPI() (*backup.Config, error) {
	cfg, err := backup.LoadConfig(backupConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return backup.NewConfig(), nil
		}
		return nil, err
	}
	return cfg, nil
}

func maskBackupConfig(cfg *backup.Config) *apicontract.BackupConfig {
	if cfg == nil {
		return &apicontract.BackupConfig{}
	}
	out := &apicontract.BackupConfig{
		Enabled: cfg.Enabled,
		Include: apicontract.BackupInclude{
			Data: cfg.Include.Data, Conf: cfg.Include.Conf,
			History: cfg.Include.History, LocalStorage: cfg.Include.LocalStorage,
		},
		Targets: make([]*apicontract.BackupTarget, 0, len(cfg.Targets)),
	}
	if cfg.Encryption != nil {
		out.Encryption = &apicontract.BackupEncryption{
			Enabled: cfg.Encryption.Enabled, Passphrase: maskSecret(cfg.Encryption.Passphrase),
		}
	}
	for _, target := range cfg.Targets {
		item := &apicontract.BackupTarget{
			ID: target.ID, Name: target.Name, Type: target.Type, Enabled: target.Enabled,
		}
		if target.Retention != nil {
			item.Retention = &apicontract.BackupRetention{
				KeepLast: target.Retention.KeepLast, MaxAgeDays: target.Retention.MaxAgeDays,
			}
		}
		if target.PG != nil {
			item.PG = &apicontract.BackupPG{
				DSN: maskDSN(target.PG.DSN), Host: target.PG.Host, Port: target.PG.Port,
				User: target.PG.User, Password: maskSecret(target.PG.Password),
				Database: target.PG.Database, SSLMode: target.PG.SSLMode, Schema: target.PG.Schema,
				Timeout: target.PG.Timeout, ConcurrentReqs: target.PG.ConcurrentReqs,
			}
		}
		if target.S3 != nil {
			item.S3 = &apicontract.BackupS3{
				Endpoint: target.S3.Endpoint, AccessKey: target.S3.AccessKey,
				SecretKey: maskSecret(target.S3.SecretKey), Bucket: target.S3.Bucket,
				Region: target.S3.Region, PathStyle: target.S3.PathStyle,
				SkipTLSVerify: target.S3.SkipTLSVerify, Timeout: target.S3.Timeout,
			}
		}
		if target.WebDAV != nil {
			item.WebDAV = &apicontract.BackupWebDAV{
				Endpoint: target.WebDAV.Endpoint, Username: target.WebDAV.Username,
				Password: maskSecret(target.WebDAV.Password), SkipTLSVerify: target.WebDAV.SkipTLSVerify,
				Timeout: target.WebDAV.Timeout,
			}
		}
		out.Targets = append(out.Targets, item)
	}
	return out
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	return backupMasked
}

func maskDSN(dsn string) string {
	dsn = pgPasswordPattern.ReplaceAllString(dsn, "${1}"+backupMasked+"${3}")
	return pgKeywordPasswordPattern.ReplaceAllString(dsn, "${1}"+backupMasked)
}

func unmaskBackupConfig(cfg *apicontract.BackupConfig, old *backup.Config) *backup.Config {
	out := &backup.Config{
		Enabled: cfg.Enabled,
		Include: backup.Include{
			Data: cfg.Include.Data, Conf: cfg.Include.Conf,
			History: cfg.Include.History, LocalStorage: cfg.Include.LocalStorage,
		},
	}
	if cfg.Encryption != nil {
		passphrase := cfg.Encryption.Passphrase
		if passphrase == "" || passphrase == backupMasked {
			if old != nil && old.Encryption != nil {
				passphrase = old.Encryption.Passphrase
			} else if passphrase == backupMasked {
				passphrase = ""
			}
		}
		out.Encryption = &backup.Encryption{Enabled: cfg.Encryption.Enabled, Passphrase: passphrase}
	}
	oldByID := map[string]*backup.Target{}
	if old != nil {
		for _, target := range old.Targets {
			oldByID[target.ID] = target
		}
	}
	for _, item := range cfg.Targets {
		out.Targets = append(out.Targets, backupTargetFromAPI(item, oldByID[item.ID]))
	}
	return out
}

// backupTargetFromAPI 把 API 提交的目标配置转换为内核目标;掩码密钥用 previous
// 中保存的真实值回填
func backupTargetFromAPI(item *apicontract.BackupTarget, previous *backup.Target) *backup.Target {
	target := &backup.Target{ID: item.ID, Name: item.Name, Type: item.Type, Enabled: item.Enabled}
	if item.Retention != nil {
		target.Retention = &backup.Retention{KeepLast: item.Retention.KeepLast, MaxAgeDays: item.Retention.MaxAgeDays}
	}
	if item.Schedule != nil {
		target.Schedule = &backup.Schedule{Cron: item.Schedule.Cron}
	}
	if item.PG != nil {
		dsn := item.PG.DSN
		if strings.Contains(dsn, backupMasked) {
			if previous != nil && previous.PG != nil {
				dsn = previous.PG.DSN
			} else {
				dsn = ""
			}
		}
		password := item.PG.Password
		if password == "" || password == backupMasked {
			if previous != nil && previous.PG != nil {
				password = previous.PG.Password
			} else if password == backupMasked {
				password = ""
			}
		}
		if dsn == "" && item.PG.Host == "" && previous != nil && previous.PG != nil && previous.PG.DSN != "" {
			dsn = previous.PG.DSN
		}
		target.PG = &backup.PG{
			DSN: dsn, Host: item.PG.Host, Port: item.PG.Port, User: item.PG.User,
			Password: password, Database: item.PG.Database, SSLMode: item.PG.SSLMode,
			Schema: item.PG.Schema, Timeout: item.PG.Timeout, ConcurrentReqs: item.PG.ConcurrentReqs,
		}
	}
	if item.S3 != nil {
		secret := item.S3.SecretKey
		if secret == "" || secret == backupMasked {
			if previous != nil && previous.S3 != nil {
				secret = previous.S3.SecretKey
			} else if secret == backupMasked {
				secret = ""
			}
		}
		target.S3 = &backup.S3{
			Endpoint: item.S3.Endpoint, AccessKey: item.S3.AccessKey, SecretKey: secret,
			Bucket: item.S3.Bucket, Region: item.S3.Region, PathStyle: item.S3.PathStyle,
			SkipTLSVerify: item.S3.SkipTLSVerify, Timeout: item.S3.Timeout,
		}
	}
	if item.WebDAV != nil {
		password := item.WebDAV.Password
		if password == "" || password == backupMasked {
			if previous != nil && previous.WebDAV != nil {
				password = previous.WebDAV.Password
			} else if password == backupMasked {
				password = ""
			}
		}
		target.WebDAV = &backup.WebDAV{
			Endpoint: item.WebDAV.Endpoint, Username: item.WebDAV.Username, Password: password,
			SkipTLSVerify: item.WebDAV.SkipTLSVerify, Timeout: item.WebDAV.Timeout,
		}
	}
	return target
}

func backupStateForAPI() *apicontract.BackupState {
	out := &apicontract.BackupState{Targets: map[string]*apicontract.BackupTargetState{}}
	state, err := backup.LoadState(util.WorkspaceDir)
	if err != nil {
		return out
	}
	if !state.UpdatedAt.IsZero() {
		out.UpdatedAt = state.UpdatedAt.Format(time.RFC3339)
	}
	for id, target := range state.Targets {
		out.Targets[id] = &apicontract.BackupTargetState{
			LastSnapshot: target.LastSnapshot, LastOK: target.LastOK, LastError: target.LastError,
			LastRun: target.LastRun.Format(time.RFC3339), UploadedBlobs: target.UploadedBlobs,
			UploadedBytes: target.UploadedBytes,
		}
	}
	return out
}

var getBackupConfig = contractHandler(apicontract.GetBackupConfig, func(c *gin.Context,
	_ apicontract.EmptyRequest) apicontract.Response[apicontract.BackupGetConfigData] {
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupGetConfigData](-1, err.Error())
	}
	return apicontract.Success(apicontract.BackupGetConfigData{
		Config: maskBackupConfig(cfg), State: backupStateForAPI(),
		Progress: backupProgressForAPI(),
	})
})

var setBackupConfig = contractHandler(apicontract.SetBackupConfig, func(c *gin.Context,
	req apicontract.BackupSetConfigRequest) apicontract.Response[apicontract.BackupGetConfigData] {
	if req.Config == nil {
		return apicontract.Failure[apicontract.BackupGetConfigData](-1, "backup config is required")
	}
	old, _ := loadBackupConfigForAPI()
	next := unmaskBackupConfig(req.Config, old)
	if err := next.Normalize(); err != nil {
		return apicontract.Failure[apicontract.BackupGetConfigData](-1, err.Error())
	}
	if err := backup.SaveConfig(backupConfigPath(), next); err != nil {
		return apicontract.Failure[apicontract.BackupGetConfigData](-1, err.Error())
	}
	return apicontract.Success(apicontract.BackupGetConfigData{
		Config: maskBackupConfig(next), State: backupStateForAPI(),
		Progress: backupProgressForAPI(),
	})
})

var testBackupTarget = contractHandler(apicontract.TestBackupTarget, func(c *gin.Context,
	req apicontract.BackupTestTargetRequest) apicontract.Response[apicontract.BackupTestTargetData] {
	if req.Target == nil {
		return apicontract.Success(apicontract.BackupTestTargetData{Message: "target is required"})
	}
	cfg := &apicontract.BackupConfig{Targets: []*apicontract.BackupTarget{req.Target}}
	old, _ := loadBackupConfigForAPI()
	targetConfig := unmaskBackupConfig(cfg, old).Targets[0]
	store, err := backup.NewTarget(targetConfig)
	if err != nil {
		return apicontract.Success(apicontract.BackupTestTargetData{Message: err.Error()})
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	key := fmt.Sprintf("probes/%d", time.Now().UnixNano())
	payload := []byte("siyuan backup probe")
	if err = store.Put(ctx, key, payload); err == nil {
		var data []byte
		data, err = store.Get(ctx, key)
		if err == nil && !bytes.Equal(data, payload) {
			err = fmt.Errorf("probe content mismatch")
		}
	}
	_ = store.Delete(ctx, key)
	if err != nil {
		return apicontract.Success(apicontract.BackupTestTargetData{Message: err.Error()})
	}
	return apicontract.Success(apicontract.BackupTestTargetData{OK: true, Message: "ok"})
})

var runBackup = contractHandler(apicontract.RunBackup, func(c *gin.Context,
	req apicontract.BackupRunRequest) apicontract.Response[apicontract.BackupRunData] {
	cfg, err := backup.LoadConfig(backupConfigPath())
	if err != nil {
		return apicontract.Failure[apicontract.BackupRunData](-1, err.Error())
	}
	engine := backup.NewEngine(util.WorkspaceDir, cfg)
	engine.Takeover = req.Takeover
	result, err := engine.Run(c.Request.Context(), req.Targets...)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRunData](-1, err.Error())
	}
	out := apicontract.BackupRunData{
		Snapshot: result.Snapshot, StartedAt: result.StartedAt.Format(time.RFC3339),
		FinishedAt: result.FinishedAt.Format(time.RFC3339), Files: result.Files, TotalBytes: result.TotalBytes,
	}
	for _, target := range result.Targets {
		out.Targets = append(out.Targets, apicontract.BackupRunTarget{
			ID: target.ID, OK: target.OK, Error: target.Error, UploadedBlobs: target.UploadedBlobs,
			SkippedBlobs: target.SkippedBlobs, UploadedBytes: target.UploadedBytes, Manifest: target.Manifest,
			ClaimedBy: target.ClaimedBy, ClaimedHost: target.ClaimedHost, ClaimedAt: target.ClaimedAt,
		})
	}
	return apicontract.Success(out)
})

// backupProgressForAPI 组装在飞备份/还原的进度快照,空闲时返回 nil
func backupProgressForAPI() *apicontract.BackupProgress {
	out := &apicontract.BackupProgress{}
	if run := backup.RunState(); run != nil {
		out.Backup = &apicontract.BackupRunProgress{
			StartedAt: run.StartedAt, Target: run.Target, Done: run.Done, Total: run.Total,
		}
	}
	if restore := backup.RestoreState(); restore != nil {
		out.Restore = &apicontract.BackupRestoreProgress{
			StartedAt: restore.StartedAt, Done: restore.Done, Total: restore.Total,
		}
	}
	if out.Backup == nil && out.Restore == nil {
		return nil
	}
	return out
}

func backupTargetByID(cfg *backup.Config, id string) (*backup.Target, error) {
	for _, cfgTarget := range cfg.Targets {
		if cfgTarget.ID == id {
			return cfgTarget, nil
		}
	}
	return nil, fmt.Errorf("backup target [%s] not found", id)
}

var listBackupSnapshots = contractHandler(apicontract.BackupListSnapshots, func(c *gin.Context,
	req apicontract.BackupListSnapshotsRequest) apicontract.Response[apicontract.BackupListSnapshotsData] {
	if strings.TrimSpace(req.Target) == "" {
		return apicontract.Failure[apicontract.BackupListSnapshotsData](-1, "backup target id is required")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupListSnapshotsData](-1, err.Error())
	}
	targetConfig, err := backupTargetByID(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BackupListSnapshotsData](-1, err.Error())
	}
	store, err := backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
	if err != nil {
		return apicontract.Failure[apicontract.BackupListSnapshotsData](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	manifests, err := backup.ListSnapshots(ctx, store)
	if err != nil {
		return apicontract.Failure[apicontract.BackupListSnapshotsData](-1, err.Error())
	}
	out := apicontract.BackupListSnapshotsData{Target: req.Target}
	for _, manifest := range manifests {
		out.Snapshots = append(out.Snapshots, apicontract.BackupSnapshotSummary{
			ID: manifest.ID, CreatedAt: manifest.CreatedAt.Format(time.RFC3339), Roots: manifest.Roots,
			Files: len(manifest.Files), TotalBytes: manifest.TotalBytes, Encrypted: manifest.Encryption != nil,
		})
	}
	return apicontract.Success(out)
})

// deleteCheckBackup 逐目标校验连通与快照数量并返回删除门禁判定:目标确认已
// 清空才可删除配置,同一存储的重复配置经指纹比对放行
var deleteCheckBackup = contractHandler(apicontract.BackupDeleteCheck, func(c *gin.Context,
	req apicontract.BackupDeleteCheckRequest) apicontract.Response[apicontract.BackupDeleteCheckData] {
	if len(req.Targets) == 0 {
		return apicontract.Failure[apicontract.BackupDeleteCheckData](-1, "backup target ids are required")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupDeleteCheckData](-1, err.Error())
	}
	checks := make([]backup.TargetCheck, 0, len(req.Targets))
	for _, id := range req.Targets {
		checks = append(checks, probeBackupTargetCheck(c.Request.Context(), cfg, id))
	}
	policy := backup.ApplyDeletionPolicy(checks)
	out := apicontract.BackupDeleteCheckData{Results: make([]apicontract.BackupDeleteCheckItem, 0, len(policy))}
	for _, result := range policy {
		out.Results = append(out.Results, apicontract.BackupDeleteCheckItem{
			Target: result.Target, Deletable: result.Deletable, Reason: result.Reason,
			Snapshots: result.Snapshots, Duplicate: result.Duplicate,
		})
	}
	return apicontract.Success(out)
})

// probeBackupTargetCheck 连接目标并列出快照,返回连通/快照数/物理指纹;连接
// 或列举失败即视为不可达
func probeBackupTargetCheck(ctx context.Context, cfg *backup.Config, id string) backup.TargetCheck {
	check := backup.TargetCheck{Target: id}
	targetConfig, err := backupTargetByID(cfg, id)
	if err != nil {
		return check
	}
	store, err := backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
	if err != nil {
		return check
	}
	manifests, err := backup.ListSnapshots(ctx, store)
	if err != nil {
		return check
	}
	check.Reachable = true
	check.Snapshots = len(manifests)
	if fingerprint, fingerprintErr := store.Fingerprint(ctx); fingerprintErr == nil {
		check.Fingerprint = fingerprint
	}
	return check
}

// editCheckBackup 编辑连接门禁:连接信息变化时比较新旧存储指纹,同一存储或
// 原存储确认已清空才放行保存
var editCheckBackup = contractHandler(apicontract.BackupEditCheck, func(c *gin.Context,
	req apicontract.BackupEditCheckRequest) apicontract.Response[apicontract.BackupEditCheckData] {
	if strings.TrimSpace(req.ID) == "" || req.Target == nil {
		return apicontract.Failure[apicontract.BackupEditCheckData](-1, "backup target id and edited target are required")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupEditCheckData](-1, err.Error())
	}
	oldTarget, err := backupTargetByID(cfg, req.ID)
	if err != nil {
		return apicontract.Failure[apicontract.BackupEditCheckData](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	oldCheck := probeBackupTargetCheck(ctx, cfg, req.ID)
	newCheck := backup.TargetCheck{Target: req.ID}
	if newStore, storeErr := backup.NewTarget(backupTargetFromAPI(req.Target, oldTarget)); storeErr == nil {
		if manifests, listErr := backup.ListSnapshots(ctx, newStore); listErr == nil {
			newCheck.Reachable = true
			newCheck.Snapshots = len(manifests)
			if fingerprint, fingerprintErr := newStore.Fingerprint(ctx); fingerprintErr == nil {
				newCheck.Fingerprint = fingerprint
			}
		}
	}
	allowed, reason := backup.ApplyEditPolicy(oldCheck, newCheck)
	sameStorage := oldCheck.Fingerprint != "" && oldCheck.Fingerprint == newCheck.Fingerprint
	return apicontract.Success(apicontract.BackupEditCheckData{
		Allowed: allowed, Reason: reason, OldSnapshots: oldCheck.Snapshots, SameStorage: sameStorage,
	})
})

// backupTargetStoreForAPI 加载配置并构建目标存储;任一步失败返回错误
func backupTargetStoreForAPI(cfg *backup.Config, id string) (backup.Store, error) {
	targetConfig, err := backupTargetByID(cfg, id)
	if err != nil {
		return nil, err
	}
	return backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
}

// deleteBackupSnapshot 删除目标上的单个快照,共享数据块按引用计数保留
var deleteBackupSnapshot = contractHandler(apicontract.BackupDeleteSnapshot, func(c *gin.Context,
	req apicontract.BackupDeleteSnapshotRequest) apicontract.Response[apicontract.BackupDeleteSnapshotData] {
	if backup.RunState() != nil || backup.RestoreState() != nil {
		return apicontract.Failure[apicontract.BackupDeleteSnapshotData](-1, "a backup or restore is running")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupDeleteSnapshotData](-1, err.Error())
	}
	store, err := backupTargetStoreForAPI(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BackupDeleteSnapshotData](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	blobs, err := backup.DeleteSnapshot(ctx, store, req.Snapshot)
	if err != nil {
		return apicontract.Failure[apicontract.BackupDeleteSnapshotData](-1, err.Error())
	}
	return apicontract.Success(apicontract.BackupDeleteSnapshotData{Blobs: blobs})
})

// emptyBackupTarget 清空目标上的全部快照与数据块,保留归属标记
var emptyBackupTarget = contractHandler(apicontract.BackupEmptyTarget, func(c *gin.Context,
	req apicontract.BackupEmptyTargetRequest) apicontract.Response[apicontract.BackupEmptyTargetData] {
	if backup.RunState() != nil || backup.RestoreState() != nil {
		return apicontract.Failure[apicontract.BackupEmptyTargetData](-1, "a backup or restore is running")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupEmptyTargetData](-1, err.Error())
	}
	store, err := backupTargetStoreForAPI(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BackupEmptyTargetData](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 600*time.Second)
	defer cancel()
	snapshots, blobs, err := backup.EmptyTarget(ctx, store)
	if err != nil {
		return apicontract.Failure[apicontract.BackupEmptyTargetData](-1, err.Error())
	}
	return apicontract.Success(apicontract.BackupEmptyTargetData{Snapshots: snapshots, Blobs: blobs})
})

var restoreBackupDryRun = contractHandler(apicontract.BackupRestoreDryRun, func(c *gin.Context,
	req apicontract.BackupRestoreDryRunRequest) apicontract.Response[apicontract.BackupRestoreDryRunData] {
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreDryRunData](-1, err.Error())
	}
	targetConfig, err := backupTargetByID(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreDryRunData](-1, err.Error())
	}
	store, err := backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreDryRunData](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	plan, err := backup.RestoreDryRun(ctx, store, req.Snapshot, req.Passphrase, util.WorkspaceDir, req.Force)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreDryRunData](-1, err.Error())
	}
	return apicontract.Success(apicontract.BackupRestoreDryRunData{
		Snapshot: plan.Snapshot, CreatedAt: plan.CreatedAt, Roots: plan.Roots,
		Files: plan.Files, TotalBytes: plan.TotalBytes, Encrypted: plan.Encrypted,
		Conflicts: plan.Conflicts, RemoveCount: plan.RemoveCount, RemoveBytes: plan.RemoveBytes,
	})
})

var restoreBackup = contractHandler(apicontract.BackupRestore, func(c *gin.Context,
	req apicontract.BackupRestoreRequest) apicontract.Response[apicontract.BackupRestoreData] {
	// 还原会整体改写工作空间文件,与备份、另一场还原互斥
	if backup.RunState() != nil || backup.RestoreState() != nil {
		return apicontract.Failure[apicontract.BackupRestoreData](-1, "a backup or restore is already running")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreData](-1, err.Error())
	}
	targetConfig, err := backupTargetByID(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreData](-1, err.Error())
	}
	store, err := backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreData](-1, err.Error())
	}
	// 还原写入当前工作空间:先做 dry-run 校验快照与口令及目标差异,再落盘
	planCtx, planCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	if _, err = backup.RestoreDryRun(planCtx, store, req.Snapshot, req.Passphrase, util.WorkspaceDir, req.Force); err != nil {
		planCancel()
		return apicontract.Failure[apicontract.BackupRestoreData](-1, err.Error())
	}
	planCancel()
	dest := util.WorkspaceDir
	result, err := backup.Restore(c.Request.Context(), store, req.Snapshot, dest, req.Force, req.Passphrase, req.KeepIdentity)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreData](-1, err.Error())
	}
	data := apicontract.BackupRestoreData{
		Snapshot: result.Snapshot, Files: result.Files, Bytes: result.Bytes, Dest: dest,
		Removed: result.Removed, RemovedBytes: result.RemovedBytes,
	}
	// 还原改写整库:索引在 temp/ 不入快照,落盘后必须在返回前同步重建,
	// 否则搜索/反链与磁盘实际内容不一致;索引失败按错误返回已落盘规模
	if err = model.ReindexRestoredWorkspace(); err != nil {
		if reindexErr, ok := err.(*model.RestoreReindexError); ok {
			data.FailedBoxes = reindexErr.FailedBoxes
		}
		return apicontract.BackupRestore.FailureWithData(-1, err.Error(), data)
	}
	data.Reindexed = true
	return apicontract.Success(data)
})

// unmaskBackupTarget resolves credential masking in an API-supplied target
// config against the stored config, so masked secrets resolve to real values.
func unmaskBackupTarget(cfg *backup.Config, targetConfig *backup.Target) *backup.Target {
	for _, stored := range cfg.Targets {
		if stored.ID == targetConfig.ID {
			if targetConfig.PG != nil && stored.PG != nil && targetConfig.PG.Password == backupMasked {
				targetConfig.PG.Password = stored.PG.Password
			}
			if targetConfig.S3 != nil && stored.S3 != nil && targetConfig.S3.SecretKey == backupMasked {
				targetConfig.S3.SecretKey = stored.S3.SecretKey
			}
			if targetConfig.WebDAV != nil && stored.WebDAV != nil && targetConfig.WebDAV.Password == backupMasked {
				targetConfig.WebDAV.Password = stored.WebDAV.Password
			}
			return targetConfig
		}
	}
	return targetConfig
}
