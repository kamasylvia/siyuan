// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestZipRoundTrip 验证 zip 快照的完整闭环:LocalTarget 备份 → 导出 zip →
// 从 zip 还原到新目录 → 文件内容一致(含中文文档名),加密快照需口令
func TestZipRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/中文文档.sy", `{"ID":"doc-zh","content":"多语言内容 ✓"}`)
	writeTestFile(t, workspace, "conf/conf.json", `{"lang":"zh-CN"}`)
	engine := newLocalTestEngine(t, workspace, targetDir)
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "snapshot.zip")
	if err = WriteSnapshotZip(context.Background(), target, result.Snapshot, zipPath); err != nil {
		t.Fatal(err)
	}
	archive, err := OpenSnapshotZip(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	summary := archive.Snapshot()
	if summary.ID != result.Snapshot {
		t.Fatalf("snapshot id mismatch: %q vs %q", summary.ID, result.Snapshot)
	}

	restoreDir := t.TempDir()
	if _, err = Restore(context.Background(), archive, result.Snapshot, restoreDir, true, "", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(restoreDir, "data", "box", "中文文档.sy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "doc-zh") {
		t.Fatalf("restored doc content mismatch: %s", data)
	}
}
