// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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

func newLocalTestEngine(t *testing.T, workspace, targetDir string) *Engine {
	t.Helper()
	cfg := NewConfig()
	cfg.Enabled = true
	engine := NewEngine(workspace, cfg)
	local, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	engine.InjectedTargets = []Store{local}
	engine.InjectedRetention = map[string]Retention{"local-1": {KeepLast: 10}}
	return engine
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

	engine := newLocalTestEngine(t, workspace, targetDir)
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
	restored, err := Restore(context.Background(), target, third.Snapshot, restoreDir, false, "", true)
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
	engine := newLocalTestEngine(t, workspace, targetDir)
	engine.Config.Include.LocalStorage = true
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
	cfg.Targets = []*Target{{ID: "x", Type: TypeS3, Enabled: true}}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected missing s3 settings error")
	}
	cfg = NewConfig()
	cfg.Targets = []*Target{{ID: "x", Type: TypeS3, Enabled: true, S3: &S3{Endpoint: "https://s3.example.com", Bucket: "b"}}}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionKeepsConfiguredSnapshots(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "data/box/doc.sy", "v1")
	engine := newLocalTestEngine(t, workspace, targetDir)
	engine.InjectedRetention = map[string]Retention{"local-1": {KeepLast: 2}}
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

func TestEncryptedBackupRequiresPassphrase(t *testing.T) {
	workspace := t.TempDir()
	targetDir := t.TempDir()
	writeTestFile(t, workspace, "conf/conf.json", `{"api":{"token":"secret-token"}}`)
	engine := newLocalTestEngine(t, workspace, targetDir)
	cfg := engine.Config
	cfg.Encryption = &Encryption{Enabled: true, Passphrase: "correct horse battery staple"}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := os.ReadDir(filepath.Join(targetDir, EncryptedBlobPrefix))
	if err != nil || len(blobs) == 0 {
		t.Fatalf("expected encrypted blobs: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(targetDir, EncryptedBlobPrefix, blobs[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-token") {
		t.Fatal("encrypted blob contains plaintext")
	}
	target, err := NewLocalTarget("local-1", targetDir)
	if err != nil {
		t.Fatal(err)
	}
	restoreDir := t.TempDir()
	if _, err = Restore(context.Background(), target, result.Snapshot, restoreDir, false, "", false); err == nil {
		t.Fatal("expected missing passphrase error")
	}
	if _, err = Restore(context.Background(), target, result.Snapshot, restoreDir, false, "wrong", false); err == nil {
		t.Fatal("expected wrong passphrase error")
	}
	restored, err := Restore(context.Background(), target, result.Snapshot, restoreDir, false,
		"correct horse battery staple", false)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Files != 1 {
		t.Fatalf("expected 1 restored file, got %d", restored.Files)
	}
	got, err := os.ReadFile(filepath.Join(restoreDir, "conf", "conf.json"))
	if err != nil || !strings.Contains(string(got), "secret-token") {
		t.Fatalf("restored content mismatch: %v", err)
	}
}

// 测试夹具:本地目录对象存储,实现 Store 接口;生产代码不再提供 Local 目标。
type LocalTarget struct {
	id   string
	root string
}

func NewLocalTarget(id, root string) (*LocalTarget, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local backup target path is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	return &LocalTarget{id: id, root: abs}, nil
}

func (target *LocalTarget) ID() string { return target.id }

func (target *LocalTarget) Fingerprint(_ context.Context) (string, error) {
	// 指纹取物理根目录而非配置 ID:同一根目录的多个配置视为同一存储
	return "local/" + filepath.ToSlash(target.root), nil
}

func (target *LocalTarget) resolve(key string) (string, error) {
	key = path.Clean("/" + strings.TrimSpace(key))
	if key == "/" {
		return "", errors.New("backup key is empty")
	}
	abs := filepath.Join(target.root, filepath.FromSlash(strings.TrimPrefix(key, "/")))
	rel, err := filepath.Rel(target.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("backup key escapes target: %s", key)
	}
	return abs, nil
}

func (target *LocalTarget) Has(_ context.Context, key string) (bool, error) {
	abs, err := target.resolve(key)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func (target *LocalTarget) Put(_ context.Context, key string, data []byte) error {
	abs, err := target.resolve(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".backup-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0644); err != nil {
		return err
	}
	return os.Rename(name, abs)
}

func (target *LocalTarget) Get(_ context.Context, key string) ([]byte, error) {
	abs, err := target.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

func (target *LocalTarget) Delete(_ context.Context, key string) error {
	abs, err := target.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(abs)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (target *LocalTarget) List(_ context.Context, prefix string) ([]string, error) {
	abs, err := target.resolve(prefix)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var keys []string
	if !info.IsDir() {
		return []string{path.Clean(prefix)}, nil
	}
	err = filepath.WalkDir(abs, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(target.root, p)
		if relErr != nil {
			return relErr
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// NewTarget builds the configured backend. S3 and WebDAV targets connect
// lazily enough that an unreachable target only fails that target, not the
// whole run.
