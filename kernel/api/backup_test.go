// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"
	"testing"

	"github.com/siyuan-note/siyuan/kernel/apicontract"
	"github.com/siyuan-note/siyuan/kernel/backup"
)

func TestMaskDSN(t *testing.T) {
	cases := map[string]string{
		"postgres://user:secret@host:5432/db":            "postgres://user:***@host:5432/db",
		"host=db user=u password=secret sslmode=disable": "host=db user=u password=*** sslmode=disable",
		"postgres://user@host/db":                        "postgres://user@host/db",
	}
	for input, want := range cases {
		if got := maskDSN(input); got != want {
			t.Fatalf("maskDSN(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUnmaskBackupConfigKeepsMaskedSecrets(t *testing.T) {
	old := &backup.Config{Targets: []*backup.Target{
		{ID: "s3", Type: backup.TypeS3, S3: &backup.S3{SecretKey: "s3-secret"}},
		{ID: "dav", Type: backup.TypeWebDAV, WebDAV: &backup.WebDAV{Password: "dav-secret"}},
		{ID: "pg", Type: backup.TypePG, PG: &backup.PG{DSN: "postgres://u:pg-secret@h/db"}},
	}}
	next := &apicontract.BackupConfig{Targets: []*apicontract.BackupTarget{
		{ID: "s3", Type: "s3", S3: &apicontract.BackupS3{SecretKey: "***"}},
		{ID: "dav", Type: "webdav", WebDAV: &apicontract.BackupWebDAV{Password: "***"}},
		{ID: "pg", Type: "pg", PG: &apicontract.BackupPG{DSN: "postgres://u:***@h/db"}},
	}}
	got := unmaskBackupConfig(next, old)
	if got.Targets[0].S3.SecretKey != "s3-secret" {
		t.Fatalf("s3 secret not preserved: %q", got.Targets[0].S3.SecretKey)
	}
	if got.Targets[1].WebDAV.Password != "dav-secret" {
		t.Fatalf("webdav password not preserved: %q", got.Targets[1].WebDAV.Password)
	}
	if !strings.Contains(got.Targets[2].PG.DSN, "pg-secret") {
		t.Fatalf("pg dsn not preserved: %q", got.Targets[2].PG.DSN)
	}
}
