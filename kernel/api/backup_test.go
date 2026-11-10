// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"testing"

	"github.com/siyuan-note/siyuan/kernel/apicontract"
	"github.com/siyuan-note/siyuan/kernel/backup"
)

func TestUnmaskBackupConfigKeepsMaskedSecrets(t *testing.T) {
	old := &backup.Config{Targets: []*backup.Target{
		{ID: "s3", Type: backup.TypeS3, S3: &backup.S3{SecretKey: "s3-secret"}},
		{ID: "dav", Type: backup.TypeWebDAV, WebDAV: &backup.WebDAV{Password: "dav-secret"}},
	}}
	next := &apicontract.BackupConfig{Targets: []*apicontract.BackupTarget{
		{ID: "s3", Type: "s3", S3: &apicontract.BackupS3{SecretKey: "***"}},
		{ID: "dav", Type: "webdav", WebDAV: &apicontract.BackupWebDAV{Password: "***"}},
	}}
	got := unmaskBackupConfig(next, old)
	if got.Targets[0].S3.SecretKey != "s3-secret" {
		t.Fatalf("s3 secret not preserved: %q", got.Targets[0].S3.SecretKey)
	}
	if got.Targets[1].WebDAV.Password != "dav-secret" {
		t.Fatalf("webdav password not preserved: %q", got.Targets[1].WebDAV.Password)
	}
}

func TestUnmaskBackupEncryptionKeepsPassphrase(t *testing.T) {
	old := &backup.Config{Encryption: &backup.Encryption{Enabled: true, Passphrase: "secret"}}
	next := &apicontract.BackupConfig{Encryption: &apicontract.BackupEncryption{Enabled: true, Passphrase: "***"}}
	got := unmaskBackupConfig(next, old)
	if got.Encryption == nil || got.Encryption.Passphrase != "secret" {
		t.Fatalf("passphrase not preserved: %+v", got.Encryption)
	}
	next.Encryption.Passphrase = "***"
	got = unmaskBackupConfig(next, nil)
	if got.Encryption.Passphrase != "" {
		t.Fatalf("masked passphrase without previous config must clear: %q", got.Encryption.Passphrase)
	}
}

func TestRestoreReindexFailureCarriesWrittenPayload(t *testing.T) {
	// 索引失败时失败载荷必须携带已落盘规模与失败笔记本,调用方据此显错而非静默可用
	data := apicontract.BackupRestoreData{
		Snapshot: "snap-1", Files: 10, Bytes: 100, Dest: "/tmp/ws",
		Removed: 2, RemovedBytes: 20, FailedBoxes: []string{"20200101000000-abcdefg"},
	}
	payload, err := json.Marshal(apicontract.BackupRestore.FailureWithData(-1, "restored files written but database index is stale for notebooks [20200101000000-abcdefg]", data))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Code int                           `json:"code"`
		Msg  string                        `json:"msg"`
		Data apicontract.BackupRestoreData `json:"data"`
	}
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != -1 || decoded.Data.Reindexed {
		t.Fatalf("reindex failure must be code -1 with reindexed=false: %+v", decoded)
	}
	if decoded.Data.Files != 10 || len(decoded.Data.FailedBoxes) != 1 {
		t.Fatalf("failure payload lost written scale or failed boxes: %+v", decoded.Data)
	}
}
