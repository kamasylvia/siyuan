// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

// 备份 zip:上传离线快照并还原、从已连目标导出 zip 快照。
// zip 内容是存储对象集(manifest + blobs),与存储类型解耦,可离线还原。

package api

import (
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/siyuan/kernel/apicontract"
	"github.com/siyuan-note/siyuan/kernel/backup"
	"github.com/siyuan-note/siyuan/kernel/model"
	"github.com/siyuan-note/siyuan/kernel/util"
)

// saveUploadedZip 把上传的 zip 落到工作空间 temp,返回落盘路径
func saveUploadedZip(c *gin.Context, file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", fmt.Errorf("zip file is required")
	}
	zipPath := filepath.Join(util.WorkspaceDir, "temp",
		fmt.Sprintf("backup-import-%d.zip", time.Now().UnixNano()))
	if err := c.SaveUploadedFile(file, zipPath); err != nil {
		return "", err
	}
	return zipPath, nil
}

var listZipSnapshots = contractHandler(apicontract.BackupZipSnapshots, func(c *gin.Context,
	req apicontract.BackupZipSnapshotsRequest) apicontract.Response[apicontract.BackupZipSnapshotsData] {
	zipPath, err := saveUploadedZip(c, req.File)
	if err != nil {
		return apicontract.Failure[apicontract.BackupZipSnapshotsData](-1, err.Error())
	}
	archive, err := backup.OpenSnapshotZip(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return apicontract.Failure[apicontract.BackupZipSnapshotsData](-1, err.Error())
	}
	defer archive.Close()
	summary := archive.Snapshot()
	return apicontract.Success(apicontract.BackupZipSnapshotsData{
		ZipPath: zipPath,
		Snapshots: []apicontract.BackupSnapshotSummary{{
			ID: summary.ID, CreatedAt: summary.CreatedAt, Files: summary.Files,
			TotalBytes: summary.TotalBytes, Encrypted: summary.Encrypted,
		}},
	})
})

var restoreZip = contractHandler(apicontract.BackupRestoreZip, func(c *gin.Context,
	req apicontract.BackupRestoreZipRequest) apicontract.Response[apicontract.BackupRestoreZipData] {
	if backup.RunState() != nil || backup.RestoreState() != nil {
		return apicontract.Failure[apicontract.BackupRestoreZipData](-1, "a backup or restore is already running")
	}
	archive, err := backup.OpenSnapshotZip(req.ZipPath)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreZipData](-1, err.Error())
	}
	defer archive.Close()
	if !backup.MarkRestoreStarted(int64(len(archive.Manifest().Files)), req.Snapshot) {
		return apicontract.Failure[apicontract.BackupRestoreZipData](-1, "a restore is already running")
	}
	defer backup.MarkRestoreFinished()

	dest := util.WorkspaceDir
	result, err := backup.Restore(c.Request.Context(), archive, req.Snapshot, dest,
		req.Force, req.Passphrase, req.KeepIdentity)
	if err != nil {
		return apicontract.Failure[apicontract.BackupRestoreZipData](-1, err.Error())
	}
	os.Remove(req.ZipPath)
	// 还原改写整库:与 REST 还原同流程,落盘后重建数据索引再返回;
	// 索引失败按错误返回已落盘规模与失败笔记本
	data := apicontract.BackupRestoreZipData{
		Files: result.Files, Bytes: result.Bytes, Removed: result.Removed,
	}
	if err = model.ReindexRestoredWorkspace(); err != nil {
		if reindexErr, ok := err.(*model.RestoreReindexError); ok {
			data.FailedBoxes = reindexErr.FailedBoxes
		}
		return apicontract.BackupRestoreZip.FailureWithData(-1, err.Error(), data)
	}
	return apicontract.Success(data)
})

// exportZip 把已连目标上的快照打包为 zip 下载,做离线留存;与存储类型解耦,
// PG/S3/WebDAV 快照同样导出。二进制响应走 SuccessBinary,前端按 blob 下载
var exportZip = contractHandler(apicontract.BackupExportZip, func(c *gin.Context,
	req apicontract.BackupExportZipRequest) apicontract.Response[apicontract.BinaryContent] {
	if req.Target == "" || req.Snapshot == "" {
		return apicontract.Failure[apicontract.BinaryContent](-1, "target and snapshot are required")
	}
	cfg, err := loadBackupConfigForAPI()
	if err != nil {
		return apicontract.Failure[apicontract.BinaryContent](-1, err.Error())
	}
	targetConfig, err := backupTargetByID(cfg, req.Target)
	if err != nil {
		return apicontract.Failure[apicontract.BinaryContent](-1, err.Error())
	}
	store, err := backup.NewTarget(unmaskBackupTarget(cfg, targetConfig))
	if err != nil {
		return apicontract.Failure[apicontract.BinaryContent](-1, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	zipPath := filepath.Join(util.WorkspaceDir, "temp",
		fmt.Sprintf("backup-export-%d.zip", time.Now().UnixNano()))
	if err = backup.WriteSnapshotZip(ctx, store, req.Snapshot, zipPath); err != nil {
		return apicontract.Failure[apicontract.BinaryContent](-1, err.Error())
	}
	defer os.Remove(zipPath)
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return apicontract.Failure[apicontract.BinaryContent](-1, err.Error())
	}
	c.Header("Content-Disposition",
		fmt.Sprintf("attachment; filename=siyuan-backup-%s.zip", req.Snapshot))
	return apicontract.SuccessBinary("application/zip", data)
})
