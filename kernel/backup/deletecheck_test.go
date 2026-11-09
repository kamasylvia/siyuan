// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"testing"
)

func TestApplyDeletionPolicy(t *testing.T) {
	const storageFingerprint = "pg/7234569911919294541/appdb/public"
	cases := []struct {
		name      string
		checks    []TargetCheck
		deletable map[string]bool
		reasons   map[string]string
		duplicate map[string]bool
	}{
		{
			name:      "reachable empty target is deletable",
			checks:    []TargetCheck{{Target: "a", Reachable: true, Snapshots: 0, Fingerprint: storageFingerprint}},
			deletable: map[string]bool{"a": true},
		},
		{
			name: "reachable target with snapshots is locked",
			checks: []TargetCheck{
				{Target: "a", Reachable: true, Snapshots: 3, Fingerprint: storageFingerprint},
			},
			reasons: map[string]string{"a": "snapshots"},
		},
		{
			name: "unreachable target without fingerprint is locked",
			checks: []TargetCheck{
				{Target: "a", Reachable: false},
			},
			reasons: map[string]string{"a": "unreachable"},
		},
		{
			name: "unreachable duplicate config follows an emptied reachable sibling",
			checks: []TargetCheck{
				{Target: "by-domain", Reachable: true, Snapshots: 0, Fingerprint: storageFingerprint},
				{Target: "by-ip", Reachable: false, Fingerprint: storageFingerprint},
			},
			deletable: map[string]bool{"by-domain": true, "by-ip": true},
			duplicate: map[string]bool{"by-ip": true},
		},
		{
			name: "unreachable duplicate config is locked while storage still has snapshots",
			checks: []TargetCheck{
				{Target: "by-domain", Reachable: true, Snapshots: 5, Fingerprint: storageFingerprint},
				{Target: "by-ip", Reachable: false, Fingerprint: storageFingerprint},
			},
			reasons: map[string]string{"by-domain": "snapshots", "by-ip": "unreachable"},
		},
		{
			name: "same fingerprint on two reachable configs deletes both only when both are empty",
			checks: []TargetCheck{
				{Target: "one", Reachable: true, Snapshots: 0, Fingerprint: storageFingerprint},
				{Target: "two", Reachable: true, Snapshots: 0, Fingerprint: storageFingerprint},
			},
			deletable: map[string]bool{"one": true, "two": true},
		},
		{
			name: "different storages do not inherit emptiness",
			checks: []TargetCheck{
				{Target: "empty", Reachable: true, Snapshots: 0, Fingerprint: "s3/bucket-a"},
				{Target: "unreachable", Reachable: false, Fingerprint: "s3/bucket-b"},
			},
			deletable: map[string]bool{"empty": true},
			reasons:   map[string]string{"unreachable": "unreachable"},
		},
		{
			name: "reachable empty target without fingerprint is still deletable",
			checks: []TargetCheck{
				{Target: "a", Reachable: true, Snapshots: 0},
			},
			deletable: map[string]bool{"a": true},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			results := ApplyDeletionPolicy(testCase.checks)
			if len(results) != len(testCase.checks) {
				t.Fatalf("expected %d results, got %d", len(testCase.checks), len(results))
			}
			for _, result := range results {
				if result.Deletable != testCase.deletable[result.Target] {
					t.Errorf("target %s deletable = %v, want %v", result.Target, result.Deletable, testCase.deletable[result.Target])
				}
				if result.Reason != testCase.reasons[result.Target] {
					t.Errorf("target %s reason = %q, want %q", result.Target, result.Reason, testCase.reasons[result.Target])
				}
				if result.Duplicate != testCase.duplicate[result.Target] {
					t.Errorf("target %s duplicate = %v, want %v", result.Target, result.Duplicate, testCase.duplicate[result.Target])
				}
			}
		})
	}
}

func TestLocalTargetFingerprintStable(t *testing.T) {
	dir := t.TempDir()
	one, err := NewLocalTarget("cfg-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewLocalTarget("cfg-2", dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := one.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := two.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 同一根目录的两个配置指纹一致:重复配置靠指纹识别同一物理存储
	if first != second {
		t.Fatalf("fingerprints for the same storage differ: %q vs %q", first, second)
	}
}
