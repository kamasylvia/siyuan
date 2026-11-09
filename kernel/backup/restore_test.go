// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
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
	if _, err = Restore(context.Background(), target, snapshot, restoreDir, false, "", false); err == nil {
		t.Fatal("expected conflict error without force")
	}
	got, err := os.ReadFile(filepath.Join(restoreDir, "data", "box", "doc.sy"))
	if err != nil || string(got) != `{"ID":"doc-v1"}` {
		t.Fatalf("rejected restore must not overwrite: %v %s", err, got)
	}

	// force=true:快照文件覆盖、快照外文件与空目录清理、排除项与软链保留
	result, err := Restore(context.Background(), target, snapshot, restoreDir, true, "", true)
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

func TestRunProgressLifecycle(t *testing.T) {
	if RunState() != nil || RestoreState() != nil {
		t.Fatal("progress should be idle initially")
	}
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", "v1")
	engine := newLocalTestEngine(t, workspace, targetDir)
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if RunState() != nil {
		t.Fatal("progress must clear after run returns")
	}
	// 锁被占用时不产生运行态
	engine2 := NewEngine(workspace, engine.Config)
	engine2.InjectedTargets = engine.InjectedTargets
	lock := flock.New(filepath.Join(workspace, "backup.lock"))
	locked, lockErr := lock.TryLock()
	if lockErr != nil {
		t.Fatal(lockErr)
	}
	if !locked {
		t.Fatal("failed to acquire backup lock for the conflict test")
	}
	defer func() { _ = lock.Unlock() }()
	if _, runErr := engine2.Run(context.Background()); runErr == nil {
		t.Fatal("expected lock conflict error")
	}
	if RunState() != nil {
		t.Fatal("failed lock attempt must not leave progress running")
	}
	// 还原运行态:原子占位拒绝并发,结束后清零
	if !MarkRestoreStarted(3, "local-1") {
		t.Fatal("first restore should acquire the slot")
	}
	if MarkRestoreStarted(3, "local-1") {
		t.Fatal("second restore must be rejected while one is running")
	}
	if state := RestoreState(); state == nil || state.Total != 3 || state.Done != 0 || state.Target != "local-1" {
		t.Fatalf("unexpected restore state: %+v", state)
	}
	AddRestoreDone()
	if state := RestoreState(); state == nil || state.Done != 1 {
		t.Fatalf("done counter not tracked: %+v", state)
	}
	MarkRestoreFinished()
	if RestoreState() != nil {
		t.Fatal("restore state must clear after finish")
	}
}

func TestTargetOwnershipClaim(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	// conf.json 带 system.id 才启用归属检查
	writeTestFile(t, workspace, "conf/conf.json", `{"system":{"id":"instance-a"},"lang":"zh-CN"}`)
	writeTestFile(t, workspace, "data/box/doc.sy", "v1")
	engine := newLocalTestEngine(t, workspace, targetDir)
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := readOwner(context.Background(), target)
	if err != nil || owner == nil || owner.InstanceID != "instance-a" {
		t.Fatalf("first backup should claim ownership: %v %+v", err, owner)
	}

	// 同实例再次备份:放行
	if _, err = engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 另一实例备份同一目标:拒绝并报告归属,不写快照
	workspaceB := t.TempDir()
	writeTestFile(t, workspaceB, "conf/conf.json", `{"system":{"id":"instance-b"},"lang":"zh-CN"}`)
	engineB := newLocalTestEngine(t, workspaceB, targetDir)
	resultB, err := engineB.Run(context.Background())
	if err != nil {
		t.Fatalf("denial should be reported per target, not fail the run: %v", err)
	}
	if len(resultB.Targets) != 1 || resultB.Targets[0].OK || resultB.Targets[0].ClaimedBy != "instance-a" {
		t.Fatalf("expected denial reporting instance-a: %+v", resultB.Targets)
	}
	keys, _ := target.List(context.Background(), ManifestPrefix)
	countsBeforeTakeover := len(keys)

	// takeover 接管:成功且归属切换
	engineB.Takeover = true
	resultB, err = engineB.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !resultB.Targets[0].OK {
		t.Fatalf("takeover run should succeed: %+v", resultB.Targets)
	}
	owner, err = readOwner(context.Background(), target)
	if err != nil || owner.InstanceID != "instance-b" {
		t.Fatalf("takeover should reassign ownership: %v %+v", err, owner)
	}
	keys, _ = target.List(context.Background(), ManifestPrefix)
	if len(keys) != countsBeforeTakeover+1 {
		t.Fatalf("takeover run should write one snapshot: %d -> %d", countsBeforeTakeover, len(keys))
	}
	// 原 owner(instance-a)现在被拒
	resultA, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resultA.Targets[0].OK || resultA.Targets[0].ClaimedBy != "instance-b" {
		t.Fatalf("original owner should be denied after takeover: %+v", resultA.Targets)
	}
}

func TestRestoreIdentityHandling(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", `{"ID":"doc-v1"}`)
	writeTestFile(t, workspace, "conf/conf.json",
		`{"lang":"zh-CN","accessAuthCode":"fixture-code","system":{"id":"346317AD-F9F4-558E-B076-06508C552CAE"}}`)
	engine := newLocalTestEngine(t, workspace, targetDir)
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}

	// 默认:换新身份 + 清空锁屏口令,其余配置键原样保留
	restoreDir := t.TempDir()
	if _, err = Restore(context.Background(), target, result.Snapshot, restoreDir, true, "", false); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(restoreDir, "conf", "conf.json"))
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		Lang           string `json:"lang"`
		AccessAuthCode string `json:"accessAuthCode"`
		System         struct {
			ID string `json:"id"`
		} `json:"system"`
	}
	if err = json.Unmarshal(restored, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Lang != "zh-CN" {
		t.Fatalf("unrelated conf keys should survive, lang = %q", identity.Lang)
	}
	if identity.AccessAuthCode != "" {
		t.Fatalf("lock-screen passphrase should be cleared, got %q", identity.AccessAuthCode)
	}
	if identity.System.ID == "" || identity.System.ID == "346317AD-F9F4-558E-B076-06508C552CAE" {
		t.Fatalf("expected a fresh system id, got %q", identity.System.ID)
	}

	// keepIdentity=true:迁移整个实例时身份原样保留
	migrateDir := t.TempDir()
	if _, err = Restore(context.Background(), target, result.Snapshot, migrateDir, true, "", true); err != nil {
		t.Fatal(err)
	}
	kept, err := os.ReadFile(filepath.Join(migrateDir, "conf", "conf.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kept), "346317AD-F9F4-558E-B076-06508C552CAE") ||
		!strings.Contains(string(kept), "fixture-code") {
		t.Fatal("keepIdentity=true should preserve the snapshot identity")
	}
}

func TestStorageIdentityUniqueness(t *testing.T) {
	mkConfig := func(modify func(cfg *Config)) *Config {
		cfg := &Config{
			Enabled: true,
			Include: Include{Data: true, Conf: true},
			Targets: []*Target{
				{ID: "t1", Type: TypePG, PG: &PG{Host: "10.0.0.5", Port: 5432, Database: "siyuan", Schema: "public"}},
				{ID: "t2", Type: TypePG, PG: &PG{Host: "10.0.0.9", Port: 5433, Database: "other", Schema: "public"}},
			},
		}
		modify(cfg)
		return cfg
	}
	// 同 host/端口/库(即使换 IP 写法以外的大小写差异)拒绝
	same := mkConfig(func(cfg *Config) {
		cfg.Targets[1] = &Target{ID: "t2", Type: TypePG, PG: &PG{Host: "10.0.0.5", Port: 5432, Database: "Siyuan", Schema: "public"}}
	})
	if err := same.Normalize(); err == nil || !strings.Contains(err.Error(), "points to the same storage as [t1]") {
		t.Fatalf("expected duplicate storage rejection, got %v", err)
	}
	// 不同库放行
	distinct := mkConfig(func(cfg *Config) {})
	if err := distinct.Normalize(); err != nil {
		t.Fatal(err)
	}
	// S3 同 endpoint+bucket(尾斜杠与大小写差异)拒绝
	s3 := &Config{Enabled: true, Include: Include{Data: true},
		Targets: []*Target{
			{ID: "a", Type: TypeS3, S3: &S3{Endpoint: "https://s3.example.com/", Bucket: "Backup"}},
			{ID: "b", Type: TypeS3, S3: &S3{Endpoint: "s3.example.com", Bucket: "backup"}},
		}}
	if err := s3.Normalize(); err == nil || !strings.Contains(err.Error(), "points to the same storage as [a]") {
		t.Fatalf("expected s3 duplicate rejection, got %v", err)
	}
	// WebDAV 同 endpoint 拒绝
	wd := &Config{Enabled: true, Include: Include{Data: true},
		Targets: []*Target{
			{ID: "a", Type: TypeWebDAV, WebDAV: &WebDAV{Endpoint: "https://nas.example.com:5006/dav/"}},
			{ID: "b", Type: TypeWebDAV, WebDAV: &WebDAV{Endpoint: "nas.example.com:5006/dav"}},
		}}
	if err := wd.Normalize(); err == nil || !strings.Contains(err.Error(), "points to the same storage as [a]") {
		t.Fatalf("expected webdav duplicate rejection, got %v", err)
	}
}
