// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func seedManifest(t *testing.T, target *LocalTarget, id string, encrypted bool, hashes []string) {
	t.Helper()
	manifest := &Manifest{ID: id, CreatedAt: time.Now(), Files: make([]Entry, 0, len(hashes))}
	if encrypted {
		manifest.Encryption = &ManifestEncryption{}
	}
	for _, hash := range hashes {
		manifest.Files = append(manifest.Files, Entry{Path: "doc/" + hash, Hash: hash, Size: 3})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = target.Put(context.Background(), ManifestPrefix+id+".json", data); err != nil {
		t.Fatal(err)
	}
	blobPrefix := BlobPrefix
	if encrypted {
		blobPrefix = EncryptedBlobPrefix
	}
	for _, hash := range hashes {
		if err = target.Put(context.Background(), blobPrefix+hash, []byte("abc")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmptyTargetRemovesSnapshotsAndBlobs(t *testing.T) {
	target, err := NewLocalTarget("empty-1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedManifest(t, target, "snap-a", false, []string{"h1", "h2"})
	seedManifest(t, target, "snap-b", true, []string{"h3"})
	if err = target.Put(ctx, LatestRef, []byte(`{"snapshot":"snap-a"}`)); err != nil {
		t.Fatal(err)
	}
	if err = target.Put(ctx, ownerKey, []byte(`{"instanceID":"inst-1"}`)); err != nil {
		t.Fatal(err)
	}
	snapshots, blobs, err := EmptyTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if snapshots != 2 || blobs != 3 {
		t.Fatalf("expected 2 snapshots and 3 blobs removed, got %d and %d", snapshots, blobs)
	}
	for _, key := range []string{ManifestPrefix + "snap-a.json", ManifestPrefix + "snap-b.json",
		BlobPrefix + "h1", BlobPrefix + "h2", EncryptedBlobPrefix + "h3", LatestRef} {
		exists, err := target.Has(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("key %s should be removed", key)
		}
	}
	// 归属标记保留:清空不解除实例认领
	exists, err := target.Has(ctx, ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("owner marker should survive EmptyTarget")
	}
}

func TestDeleteSnapshotKeepsSharedBlobs(t *testing.T) {
	target, err := NewLocalTarget("del-1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedManifest(t, target, "snap-a", false, []string{"h1", "h2"})
	seedManifest(t, target, "snap-b", false, []string{"h1", "h3"})
	if err = target.Put(ctx, LatestRef, []byte(`{"snapshot":"snap-a"}`)); err != nil {
		t.Fatal(err)
	}
	blobs, err := DeleteSnapshot(ctx, target, "snap-a")
	if err != nil {
		t.Fatal(err)
	}
	if blobs != 1 {
		t.Fatalf("expected 1 exclusive blob removed, got %d", blobs)
	}
	// h1 被 snap-b 共享必须保留;h2 为 snap-a 独占,随之清理;h3 与 snap-a 无关
	for key, want := range map[string]bool{
		BlobPrefix + "h1": true, BlobPrefix + "h2": false, BlobPrefix + "h3": true,
		ManifestPrefix + "snap-a.json": false, ManifestPrefix + "snap-b.json": true,
		LatestRef: false,
	} {
		exists, err := target.Has(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if exists != want {
			t.Errorf("key %s exists = %v, want %v", key, exists, want)
		}
	}
}

func TestDeleteSnapshotAbortsOnBrokenSibling(t *testing.T) {
	target, err := NewLocalTarget("del-2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedManifest(t, target, "snap-a", false, []string{"h1"})
	seedManifest(t, target, "snap-b", false, []string{"h2"})
	// 损坏的兄弟快照:若不中止,共享数据块判定会漏掉它引用的数据块
	if err = target.Put(ctx, ManifestPrefix+"snap-c.json", []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if _, err = DeleteSnapshot(ctx, target, "snap-a"); err == nil {
		t.Fatal("expected an error for a broken sibling snapshot")
	}
	for _, key := range []string{ManifestPrefix + "snap-a.json", BlobPrefix + "h1"} {
		exists, err := target.Has(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("key %s should survive the aborted deletion", key)
		}
	}
}
