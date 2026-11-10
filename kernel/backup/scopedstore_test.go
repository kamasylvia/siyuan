// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"testing"
)

// TestScopedStoreIsolatesNamespaces 验证同一物理存储上的两个命名空间互不可见:
// 写入、列举、认领标记都隔离,底层对象以命名空间为前缀
func TestScopedStoreIsolatesNamespaces(t *testing.T) {
	dir := t.TempDir()
	base, err := NewLocalTarget("local-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	alpha := NewScopedStore(base, "nas-a")
	beta := NewScopedStore(base, "nas-b")

	if err = alpha.Put(ctx, ManifestPrefix+"snap-1.json", []byte(`{"id":"snap-1"}`)); err != nil {
		t.Fatal(err)
	}
	if err = alpha.Put(ctx, ownerKey, []byte(`{"instanceID":"inst-a"}`)); err != nil {
		t.Fatal(err)
	}

	// beta 视角:看不到 alpha 的任何对象
	if exists, err := beta.Has(ctx, ManifestPrefix+"snap-1.json"); err != nil || exists {
		t.Fatalf("beta must not see alpha manifest (exists=%v err=%v)", exists, err)
	}
	if keys, err := beta.List(ctx, ManifestPrefix); err != nil || len(keys) != 0 {
		t.Fatalf("beta list must be empty, got %v err=%v", keys, err)
	}
	if exists, err := beta.Has(ctx, ownerKey); err != nil || exists {
		t.Fatalf("beta must not see alpha owner marker (exists=%v err=%v)", exists, err)
	}

	// alpha 视角:自己的对象键名与无权空间时一致(前缀被透明剥离)
	if keys, err := alpha.List(ctx, ManifestPrefix); err != nil || len(keys) != 1 ||
		keys[0] != ManifestPrefix+"snap-1.json" {
		t.Fatalf("alpha list should be transparent, got %v err=%v", keys, err)
	}

	// 底层对象带命名空间前缀,证明作用域真实生效
	if exists, err := base.Has(ctx, "nas-a/"+ManifestPrefix+"snap-1.json"); err != nil || !exists {
		t.Fatalf("underlying key should be prefixed (exists=%v err=%v)", exists, err)
	}

	// 指纹不含命名空间:同一物理存储的两个作用域指纹一致(编辑门禁据此识别同存储)
	fpA, err := alpha.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := beta.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fpA != fpB {
		t.Fatalf("fingerprint must ignore namespace: %q vs %q", fpA, fpB)
	}
}

// TestScopedStoreEmptyNamespaceIsIdentity 验证空命名空间不加前缀(兼容存量快照)
func TestScopedStoreEmptyNamespaceIsIdentity(t *testing.T) {
	dir := t.TempDir()
	base, err := NewLocalTarget("local-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plain := NewScopedStore(base, "")
	if err = plain.Put(ctx, ManifestPrefix+"snap-1.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if exists, err := base.Has(ctx, ManifestPrefix+"snap-1.json"); err != nil || !exists {
		t.Fatalf("empty namespace must write unprefixed keys (exists=%v err=%v)", exists, err)
	}
}

// TestNamespaceValidation 验证命名空间字符集校验
func TestNamespaceValidation(t *testing.T) {
	for _, valid := range []string{"nas-a", "instance_1", "A", "abc123"} {
		cfg := NewConfig()
		cfg.Targets = []*Target{{ID: "t", Type: TypeS3, Enabled: true,
			S3: &S3{Endpoint: "https://s3.example.com", Bucket: "b"}, Namespace: valid}}
		if err := cfg.Normalize(); err != nil {
			t.Fatalf("namespace %q should be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"../escape", "a/b", "-leading", "with space", "中文"} {
		cfg := NewConfig()
		cfg.Targets = []*Target{{ID: "t", Type: TypeS3, Enabled: true,
			S3: &S3{Endpoint: "https://s3.example.com", Bucket: "b"}, Namespace: invalid}}
		if err := cfg.Normalize(); err == nil {
			t.Fatalf("namespace %q should be rejected", invalid)
		}
	}
}
