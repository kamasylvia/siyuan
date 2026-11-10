// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"strings"
	"testing"
)

func TestNewNamespaceIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewNamespaceID()
		if len(id) != 26 {
			t.Fatalf("namespace id must be 26 chars, got %d: %q", len(id), id)
		}
		for _, r := range id {
			if !strings.ContainsRune(crockfordBase32, r) {
				t.Fatalf("namespace id has invalid char %q in %q", r, id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate namespace id generated: %q", id)
		}
		seen[id] = true
		// 无论生成多少次都必须通过配置校验(存量校验规则不变)
		if !namespacePattern.MatchString(id) {
			t.Fatalf("generated namespace must satisfy validation: %q", id)
		}
	}
}

func TestNormalizeAssignsNamespaceOnce(t *testing.T) {
	mk := func(namespace string) *Config {
		return &Config{Enabled: true, Include: Include{Data: true},
			Targets: []*Target{{ID: "t1", Type: TypeS3, Namespace: namespace,
				S3: &S3{Endpoint: "https://s3.example.com", Bucket: "b"}}}}
	}
	// 空命名空间:自动分配
	cfg := mk("")
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	assigned := cfg.Targets[0].Namespace
	if len(assigned) != 26 {
		t.Fatalf("expected auto-assigned ULID namespace, got %q", assigned)
	}
	// 再次 Normalize:保持原值,保证快照谱系稳定
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Targets[0].Namespace != assigned {
		t.Fatalf("namespace must stay stable across normalizations: %q -> %q", assigned, cfg.Targets[0].Namespace)
	}
	// 非法存量值仍被拒绝
	bad := mk("bad/namespace")
	if err := bad.Normalize(); err == nil {
		t.Fatal("invalid namespace must be rejected")
	}
}
