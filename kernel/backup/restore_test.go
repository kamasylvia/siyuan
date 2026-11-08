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

// prepareForceRestoreFixture 备份两个文件(data/box/doc.sy、conf/conf.json)并
// 构造一个脏的还原目标:含快照冲突文件、快照外多余文件与受保护排除项
func prepareForceRestoreFixture(t *testing.T) (workspace, targetDir, restoreDir string, snapshot string) {
	t.Helper()
	workspace = t.TempDir()
	targetDir = t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", `{"ID":"doc-v2"}`)
	writeTestFile(t, workspace, "conf/conf.json", `{"lang":"zh-CN"}`)
	engine := newLocalTestEngine(t, workspace, targetDir)
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	restoreDir = t.TempDir()
	writeTestFile(t, restoreDir, "data/box/doc.sy", `{"ID":"doc-v1"}`)
	writeTestFile(t, restoreDir, "conf/conf.json", `{"lang":"en"}`)
	writeTestFile(t, restoreDir, "data/old-doc.sy", "stale note")
	writeTestFile(t, restoreDir, "data/assets/stale.bin", "stale attachment")
	writeTestFile(t, restoreDir, "data/ghost/layer/deep.bin", "stale deep file")
	writeTestFile(t, restoreDir, "conf/legacy.json", "stale conf")
	// 排除项与符号链接:备份不管辖,force 还原必须原样保留
	writeTestFile(t, restoreDir, "conf/backup.json", `{"targets":[{"password":"secret"}]}`)
	writeTestFile(t, restoreDir, "data/storage/local.json", `{"local":true}`)
	writeTestFile(t, restoreDir, "data/cache.tmp", "scratch")
	writeTestFile(t, restoreDir, "data/.git/config", "[core]")
	if err = os.Symlink(workspace, filepath.Join(restoreDir, "data", "assets-link")); err != nil {
		t.Fatal(err)
	}
	return workspace, targetDir, restoreDir, result.Snapshot
}

func TestRestoreForceConvergesToSnapshotState(t *testing.T) {
	workspace, targetDir, restoreDir, snapshot := prepareForceRestoreFixture(t)
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}

	// dry-run 不动盘:报告 2 个冲突文件与 4 个待清理文件
	plan, err := RestoreDryRun(context.Background(), target, snapshot, "", restoreDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Conflicts != 2 {
		t.Fatalf("expected 2 conflicts, got %d", plan.Conflicts)
	}
	if plan.RemoveCount != 4 || plan.RemoveBytes != int64(len("stale note")+len("stale attachment")+len("stale deep file")+len("stale conf")) {
		t.Fatalf("unexpected removal plan: count=%d bytes=%d", plan.RemoveCount, plan.RemoveBytes)
	}
	if _, err = os.Stat(filepath.Join(restoreDir, "data", "old-doc.sy")); err != nil {
		t.Fatal("dry-run must not touch destination")
	}

	// force=false:冲突拒绝且零写入,脏数据保持原样
	if _, err = Restore(context.Background(), target, snapshot, restoreDir, false, ""); err == nil {
		t.Fatal("expected conflict error without force")
	}
	got, err := os.ReadFile(filepath.Join(restoreDir, "data", "box", "doc.sy"))
	if err != nil || string(got) != `{"ID":"doc-v1"}` {
		t.Fatalf("rejected restore must not overwrite: %v %s", err, got)
	}

	// force=true:快照文件覆盖、快照外文件与空目录清理、排除项与软链保留
	result, err := Restore(context.Background(), target, snapshot, restoreDir, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 || result.Removed != 4 {
		t.Fatalf("unexpected result: files=%d removed=%d", result.Files, result.Removed)
	}
	got, err = os.ReadFile(filepath.Join(restoreDir, "data", "box", "doc.sy"))
	if err != nil || string(got) != `{"ID":"doc-v2"}` {
		t.Fatalf("snapshot file not restored: %v %s", err, got)
	}
	got, err = os.ReadFile(filepath.Join(restoreDir, "conf", "conf.json"))
	if err != nil || string(got) != `{"lang":"zh-CN"}` {
		t.Fatalf("snapshot conf not restored: %v %s", err, got)
	}
	for _, rel := range []string{
		"data/old-doc.sy", "data/assets/stale.bin", "data/ghost/layer/deep.bin", "conf/legacy.json",
	} {
		if _, err = os.Stat(filepath.Join(restoreDir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("stale file survived restore [%s]", rel)
		}
	}
	if _, err = os.Stat(filepath.Join(restoreDir, "data", "ghost")); !os.IsNotExist(err) {
		t.Fatal("stale directory tree should be pruned")
	}
	keep := map[string]string{
		"conf/backup.json":        `{"targets":[{"password":"secret"}]}`,
		"data/storage/local.json": `{"local":true}`,
		"data/cache.tmp":          "scratch",
		"data/.git/config":        "[core]",
	}
	for rel, want := range keep {
		got, err = os.ReadFile(filepath.Join(restoreDir, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Fatalf("excluded file must survive restore [%s]: %v %s", rel, err, got)
		}
	}
	linkTarget, err := os.Readlink(filepath.Join(restoreDir, "data", "assets-link"))
	if err != nil || linkTarget != workspace {
		t.Fatalf("symlink must survive restore: %v %s", err, linkTarget)
	}
}

func TestRestoreDryRunOnEmptyDestination(t *testing.T) {
	_, targetDir, _, snapshot := prepareForceRestoreFixture(t)
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	plan, err := RestoreDryRun(context.Background(), target, snapshot, "", empty, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Conflicts != 0 || plan.RemoveCount != 0 {
		t.Fatalf("empty destination should be conflict-free: %+v", plan)
	}
	if !strings.Contains(plan.Snapshot, "-") || plan.Files != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}
