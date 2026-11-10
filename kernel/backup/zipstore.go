// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const zipEntryDir = "siyuan-backup"

// zipMeta 是 zip 包内的说明对象:快照身份与加密标记,解包程序据此还原
type zipMeta struct {
	Version    int64     `json:"version"`
	SnapshotID string    `json:"snapshotID"`
	CreatedAt  time.Time `json:"createdAt"`
	Encrypted  bool      `json:"encrypted"`
}

// WriteSnapshotZip 把目标存储上的一个快照打包为本地 zip:内容为存储对象集
// (manifest + blobs),与存储类型解耦,可在未连接目标时离线还原。加密快照
// 在 zip 中仍是密文 blob,还原照旧需要口令
func WriteSnapshotZip(ctx context.Context, store Store, snapshotID, zipPath string) error {
	manifestData, err := store.Get(ctx, ManifestPrefix+snapshotID+".json")
	if err != nil {
		return err
	}
	manifest := &Manifest{}
	if err = json.Unmarshal(manifestData, manifest); err != nil {
		return fmt.Errorf("parse snapshot %s: %w", snapshotID, err)
	}
	blobPrefix := BlobPrefix
	if manifest.Encryption != nil {
		blobPrefix = EncryptedBlobPrefix
	}
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	writeEntry := func(name string, data []byte) error {
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	}
	meta, err := json.Marshal(zipMeta{
		Version: 1, SnapshotID: snapshotID, CreatedAt: manifest.CreatedAt,
		Encrypted: manifest.Encryption != nil,
	})
	if err != nil {
		return err
	}
	if err = writeEntry(zipEntryDir+"/meta.json", meta); err != nil {
		return err
	}
	if err = writeEntry(zipEntryDir+"/manifest.json", manifestData); err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		blob, err := store.Get(ctx, blobPrefix+entry.Hash)
		if err != nil {
			return fmt.Errorf("read blob %s: %w", entry.Hash, err)
		}
		if err = writeEntry(zipEntryDir+"/blobs/"+entry.Hash, blob); err != nil {
			return err
		}
	}
	return archive.Close()
}

// SnapshotZipArchive 是已打开的快照 zip:实现 Store 的只读视图,供既有
// Restore 路径直接消费
type SnapshotZipArchive struct {
	reader       *zip.ReadCloser
	entries      map[string]*zip.File
	manifestData []byte
	manifest     *Manifest
	blobPrefix   string
	snapshotID   string
}

// OpenSnapshotZip 打开本地 zip 快照并解析 manifest
func OpenSnapshotZip(zipPath string) (*SnapshotZipArchive, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	archive := &SnapshotZipArchive{reader: reader, entries: map[string]*zip.File{}}
	for _, file := range reader.File {
		archive.entries[file.Name] = file
	}
	manifestFile := archive.entries[zipEntryDir+"/manifest.json"]
	if manifestFile == nil {
		reader.Close()
		return nil, errors.New("not a siyuan backup zip: manifest missing")
	}
	data, err := readZipFile(manifestFile)
	if err != nil {
		reader.Close()
		return nil, err
	}
	archive.manifestData = data
	manifest := &Manifest{}
	if err = json.Unmarshal(data, manifest); err != nil {
		reader.Close()
		return nil, fmt.Errorf("parse backup manifest: %w", err)
	}
	archive.manifest = manifest
	archive.blobPrefix = BlobPrefix
	if manifest.Encryption != nil {
		archive.blobPrefix = EncryptedBlobPrefix
	}
	archive.snapshotID = manifest.ID
	return archive, nil
}

func (archive *SnapshotZipArchive) Close() error { return archive.reader.Close() }

// SnapshotSummary 返回 zip 内快照摘要(当前格式为单快照包)
type SnapshotSummary struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"createdAt"`
	Files      int    `json:"files"`
	TotalBytes int64  `json:"totalBytes"`
	Encrypted  bool   `json:"encrypted"`
}

// Manifest 返回 zip 内快照的完整清单
func (archive *SnapshotZipArchive) Manifest() *Manifest { return archive.manifest }

// Snapshot 返回 zip 内唯一快照的摘要
func (archive *SnapshotZipArchive) Snapshot() SnapshotSummary {
	return SnapshotSummary{
		ID: archive.manifest.ID, CreatedAt: archive.manifest.CreatedAt.Format(time.RFC3339),
		Files: len(archive.manifest.Files), TotalBytes: archive.manifest.TotalBytes,
		Encrypted: archive.manifest.Encryption != nil,
	}
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// 以下实现 Store 只读视图,供 Restore/RestoreDryRun 直接消费

func (archive *SnapshotZipArchive) ID() string { return "zip:" + archive.snapshotID }

func (archive *SnapshotZipArchive) Fingerprint(_ context.Context) (string, error) {
	return "zip/" + archive.snapshotID, nil
}

func (archive *SnapshotZipArchive) manifestKey(key string) (string, bool) {
	if strings.HasPrefix(key, ManifestPrefix) && strings.HasSuffix(key, ".json") {
		return zipEntryDir + "/manifest.json", true
	}
	return "", false
}

func (archive *SnapshotZipArchive) blobKey(key string) (string, bool) {
	for _, prefix := range []string{BlobPrefix, EncryptedBlobPrefix} {
		if strings.HasPrefix(key, prefix) {
			return zipEntryDir + "/blobs/" + strings.TrimPrefix(key, prefix), true
		}
	}
	return "", false
}

func (archive *SnapshotZipArchive) Has(_ context.Context, key string) (bool, error) {
	if _, ok := archive.manifestKey(key); ok {
		return true, nil
	}
	if name, ok := archive.blobKey(key); ok {
		_, ok = archive.entries[name]
		return ok, nil
	}
	return false, nil
}

func (archive *SnapshotZipArchive) Put(_ context.Context, _ string, _ []byte) error {
	return errors.New("snapshot zip archive is read-only")
}

func (archive *SnapshotZipArchive) Get(_ context.Context, key string) ([]byte, error) {
	if name, ok := archive.manifestKey(key); ok {
		file := archive.entries[name]
		if file == nil {
			return nil, errors.New("backup object not found: " + key)
		}
		return readZipFile(file)
	}
	if name, ok := archive.blobKey(key); ok {
		file := archive.entries[name]
		if file == nil {
			return nil, errors.New("backup object not found: " + key)
		}
		return readZipFile(file)
	}
	return nil, errors.New("backup object not found: " + key)
}

func (archive *SnapshotZipArchive) Delete(_ context.Context, _ string) error {
	return errors.New("snapshot zip archive is read-only")
}

func (archive *SnapshotZipArchive) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	if strings.HasPrefix(prefix, ManifestPrefix) {
		keys = append(keys, ManifestPrefix+archive.snapshotID+".json")
	}
	return keys, nil
}
