// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func newLocalTestConfig(t *testing.T, workspace, targetDir string) *Config {
	t.Helper()
	cfg := NewConfig()
	cfg.Enabled = true
	cfg.Targets = []*Target{{
		ID: "local-1", Name: "local", Type: TypeLocal, Enabled: true,
		Local:     &Local{Path: targetDir},
		Retention: &Retention{KeepLast: 10},
	}}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLocalBackupIsIncrementalAndRestorable(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", `{"ID":"doc"}`)
	writeTestFile(t, workspace, "data/assets/a.bin", "attachment")
	writeTestFile(t, workspace, "data/storage/local.json", `{"local":true}`)
	writeTestFile(t, workspace, "conf/conf.json", `{"lang":"zh-CN"}`)
	writeTestFile(t, workspace, "conf/backup.json", `{"targets":[{"password":"secret"}]}`)
	writeTestFile(t, workspace, "temp/index.db", "not backed up")
	writeTestFile(t, workspace, "repo/indexes/x", "not backed up")

	cfg := newLocalTestConfig(t, workspace, targetDir)
	engine := NewEngine(workspace, cfg)
	first, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 3 {
		t.Fatalf("expected 3 backed up files, got %d", first.Files)
	}
	if len(first.Targets) != 1 || !first.Targets[0].OK {
		t.Fatalf("target failed: %+v", first.Targets)
	}
	if first.Targets[0].UploadedBlobs != 3 {
		t.Fatalf("expected 3 uploaded blobs, got %d", first.Targets[0].UploadedBlobs)
	}
	if _, err = os.Stat(filepath.Join(targetDir, "manifests", first.Snapshot+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(targetDir, "refs", "latest.json")); err != nil {
		t.Fatal(err)
	}

	second, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Targets[0].UploadedBlobs != 0 || second.Targets[0].SkippedBlobs != 3 {
		t.Fatalf("second run should reuse blobs: %+v", second.Targets[0])
	}

	writeTestFile(t, workspace, "data/assets/a.bin", "attachment-v2")
	third, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if third.Targets[0].UploadedBlobs != 1 {
		t.Fatalf("expected one changed blob, got %+v", third.Targets[0])
	}

	restoreDir := t.TempDir()
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(context.Background(), target, third.Snapshot, restoreDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Files != 3 {
		t.Fatalf("expected 3 restored files, got %d", restored.Files)
	}
	for _, rel := range []string{"data/box/doc.sy", "data/assets/a.bin", "conf/conf.json"} {
		got, err := os.ReadFile(filepath.Join(restoreDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("missing restored file %s: %v", rel, err)
		}
		want, _ := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel)))
		if string(got) != string(want) {
			t.Fatalf("restored %s mismatch", rel)
		}
	}
	for _, rel := range []string{"temp/index.db", "repo/indexes/x", "data/storage/local.json"} {
		if _, err = os.Stat(filepath.Join(restoreDir, filepath.FromSlash(rel))); err == nil {
			t.Fatalf("unexpected restored file %s", rel)
		}
	}

	state, err := LoadState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if state.Targets["local-1"] == nil || !state.Targets["local-1"].LastOK {
		t.Fatalf("state not recorded: %+v", state)
	}
}

func TestBackupIncludeLocalStorage(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/storage/local.json", `{"local":true}`)
	cfg := newLocalTestConfig(t, workspace, targetDir)
	cfg.Include.LocalStorage = true
	engine := NewEngine(workspace, cfg)
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 {
		t.Fatalf("expected local.json to be included, got %d", result.Files)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := NewConfig()
	cfg.Targets = []*Target{{ID: "x", Type: TypeLocal, Enabled: true}}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected missing local path error")
	}
	cfg = NewConfig()
	cfg.Targets = []*Target{{ID: "x", Type: TypePG, Enabled: true, PG: &PG{DSN: "postgres://x"}}}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Targets[0].PG.Schema != "public" {
		t.Fatalf("expected default schema, got %s", cfg.Targets[0].PG.Schema)
	}
}

func TestRetentionKeepsConfiguredSnapshots(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", "v1")
	cfg := newLocalTestConfig(t, workspace, targetDir)
	cfg.Targets[0].Retention = &Retention{KeepLast: 2}
	engine := NewEngine(workspace, cfg)
	for i := 0; i < 3; i++ {
		writeTestFile(t, workspace, "data/box/doc.sy", "v"+string(rune('1'+i)))
		if _, err := engine.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := target.List(context.Background(), ManifestPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 retained manifests, got %v", keys)
	}
	blobs, err := target.List(context.Background(), BlobPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 2 {
		t.Fatalf("expected 2 referenced blobs, got %v", blobs)
	}
}
