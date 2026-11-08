// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RestoreResult struct {
	Snapshot string `json:"snapshot"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	// force 还原清理掉的快照外文件数与字节数
	Removed      int   `json:"removed"`
	RemovedBytes int64 `json:"removedBytes"`
}

// loadManifestForRestore 读取并解析快照 manifest;加密快照要求非空口令,
// 返回派生密钥与对应的 blob 前缀
func loadManifestForRestore(ctx context.Context, target Store, snapshotID, passphrase string) (*Manifest, []byte, string, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, nil, "", errors.New("snapshot id is required")
	}
	data, err := target.Get(ctx, ManifestPrefix+snapshotID+".json")
	if err != nil {
		return nil, nil, "", err
	}
	manifest := &Manifest{}
	if err = json.Unmarshal(data, manifest); err != nil {
		return nil, nil, "", fmt.Errorf("parse backup manifest [%s]: %w", snapshotID, err)
	}
	var encryptionKey []byte
	blobPrefix := BlobPrefix
	if manifest.Encryption != nil {
		if passphrase == "" {
			return nil, nil, "", errors.New("backup is encrypted; passphrase is required")
		}
		encryptionKey, err = deriveEncryptionKey(passphrase, manifest.Encryption)
		if err != nil {
			return nil, nil, "", err
		}
		blobPrefix = EncryptedBlobPrefix
	}
	return manifest, encryptionKey, blobPrefix, nil
}

// restoreDelta 是目标目录与快照 manifest 的差异:冲突 = 会被快照覆盖的已存在
// 非空文件(force=false 时这些文件使还原拒绝);多余 = Roots 之内、manifest
// 之外且不属于排除规则的文件(force 还原时被清理)
type restoreDelta struct {
	conflictCount int
	firstConflict string
	removeKeys    []string
	removeSizes   []int64
}

// scanRestoreDelta 对比 dest 与 manifest,统计冲突文件并收集待清理清单。
// 排除项(conf/backup.json、data/storage/local.json、临时/锁文件、.git 等)
// 备份不管辖,还原清理同样保留;符号链接等非普通文件保留,避免破坏用户的
// 外部卷软链布局
func scanRestoreDelta(destAbs string, manifest *Manifest) (*restoreDelta, error) {
	delta := &restoreDelta{}
	known := make(map[string]bool, len(manifest.Files))
	for _, entry := range manifest.Files {
		known[entry.Path] = true
	}
	for _, rootName := range manifest.Roots {
		rootDir := filepath.Join(destAbs, filepath.FromSlash(rootName))
		info, err := os.Stat(rootDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if p != rootDir && shouldSkipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
				return nil
			}
			rel, relErr := filepath.Rel(rootDir, p)
			if relErr != nil {
				return relErr
			}
			key := path.Join(rootName, filepath.ToSlash(rel))
			if key == "conf/backup.json" || key == "data/storage/local.json" || shouldSkipFile(d.Name()) {
				return nil
			}
			fileInfo, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			if known[key] {
				if fileInfo.Size() > 0 {
					delta.conflictCount++
					if delta.firstConflict == "" {
						delta.firstConflict = key
					}
				}
				return nil
			}
			delta.removeKeys = append(delta.removeKeys, key)
			delta.removeSizes = append(delta.removeSizes, fileInfo.Size())
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return delta, nil
}

// Restore 把一个快照物化到 dest。force=false 时目标存在同路径非空文件即拒绝,
// 不做任何写入;force=true 时目标被收敛为快照时点的纯净状态:快照内文件按
// 路径覆盖,Roots 内快照外文件与空目录被清理,排除项与符号链接原样保留
func Restore(ctx context.Context, target Store, snapshotID, dest string, force bool, passphrase string) (*RestoreResult, error) {
	if strings.TrimSpace(dest) == "" {
		return nil, errors.New("restore destination is required")
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	manifest, encryptionKey, blobPrefix, err := loadManifestForRestore(ctx, target, snapshotID, passphrase)
	if err != nil {
		return nil, err
	}
	delta, err := scanRestoreDelta(destAbs, manifest)
	if err != nil {
		return nil, err
	}
	if !force && delta.conflictCount > 0 {
		return nil, fmt.Errorf("restore destination already contains [%s]; use force to overwrite", delta.firstConflict)
	}
	result := &RestoreResult{Snapshot: snapshotID}
	for _, entry := range manifest.Files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		destPath, pathErr := safeRestorePath(destAbs, entry.Path)
		if pathErr != nil {
			return nil, pathErr
		}
		blob, getErr := target.Get(ctx, blobPrefix+entry.Hash)
		if getErr != nil {
			return nil, fmt.Errorf("read backup blob for [%s]: %w", entry.Path, getErr)
		}
		if encryptionKey != nil {
			blob, getErr = openBlob(encryptionKey, blob)
			if getErr != nil {
				return nil, fmt.Errorf("decrypt backup blob for [%s]: %w", entry.Path, getErr)
			}
		}
		sum := sha256.Sum256(blob)
		if hex.EncodeToString(sum[:]) != entry.Hash {
			return nil, fmt.Errorf("backup blob hash mismatch for [%s]", entry.Path)
		}
		if err = writeRestoredFile(destPath, blob, os.FileMode(entry.Mode)); err != nil {
			return nil, err
		}
		if entry.ModTime > 0 {
			_ = os.Chtimes(destPath, manifest.CreatedAt, timeUnix(entry.ModTime))
		}
		result.Files++
		result.Bytes += int64(len(blob))
	}
	if force {
		if err = pruneRestoreDelta(destAbs, manifest.Roots, delta, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// pruneRestoreDelta 删除扫描阶段收集的快照外文件,随后自底向上清理 Roots 内
// 的空目录(根目录本身保留);删除路径再次过 safeRestorePath 防越界
func pruneRestoreDelta(destAbs string, roots []string, delta *restoreDelta, result *RestoreResult) error {
	for i, key := range delta.removeKeys {
		destPath, pathErr := safeRestorePath(destAbs, key)
		if pathErr != nil {
			return pathErr
		}
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		result.Removed++
		result.RemovedBytes += delta.removeSizes[i]
	}
	return pruneEmptyDirs(destAbs, roots)
}

// pruneEmptyDirs 自底向上尝试删除 root 之内不再含任何条目的目录;失败(仍
// 非空、权限不足等)忽略,不影响还原结果
func pruneEmptyDirs(destAbs string, roots []string) error {
	var dirs []string
	for _, rootName := range roots {
		rootDir := filepath.Join(destAbs, filepath.FromSlash(rootName))
		info, err := os.Stat(rootDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() && p != rootDir {
				dirs = append(dirs, p)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	// 按目录深度降序:子目录先删,父目录才可能变空
	sep := string(filepath.Separator)
	sort.Slice(dirs, func(i, j int) bool {
		ci := strings.Count(dirs[i], sep)
		cj := strings.Count(dirs[j], sep)
		if ci != cj {
			return ci > cj
		}
		return dirs[i] > dirs[j]
	})
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
	return nil
}

func safeRestorePath(root, key string) (string, error) {
	key = filepath.ToSlash(strings.TrimSpace(key))
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\x00") {
		return "", fmt.Errorf("invalid backup path [%s]", key)
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid backup path [%s]", key)
	}
	abs := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("backup path escapes restore root [%s]", key)
	}
	return abs, nil
}

func timeUnix(sec int64) time.Time {
	return time.Unix(sec, 0)
}

func writeRestoredFile(dest string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".restore-*")
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
	if err = os.Chmod(name, mode.Perm()); err != nil {
		return err
	}
	return os.Rename(name, dest)
}

// RestoreDryRun 报告一次还原将做的改动而不动盘:快照统计 + 与目标目录的冲突
// 数,force 时还给出将被清理的快照外文件规模;dest 为空时只校验快照本身
func RestoreDryRun(ctx context.Context, target Store, snapshotID, passphrase, dest string, force bool) (*RestorePlan, error) {
	manifest, _, _, err := loadManifestForRestore(ctx, target, snapshotID, passphrase)
	if err != nil {
		return nil, err
	}
	plan := &RestorePlan{Snapshot: snapshotID, CreatedAt: manifest.CreatedAt.Format(time.RFC3339),
		Roots: manifest.Roots, Files: len(manifest.Files), TotalBytes: manifest.TotalBytes,
		Encrypted: manifest.Encryption != nil}
	if strings.TrimSpace(dest) == "" {
		return plan, nil
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	delta, err := scanRestoreDelta(destAbs, manifest)
	if err != nil {
		return nil, err
	}
	plan.Conflicts = delta.conflictCount
	if force {
		plan.RemoveCount = len(delta.removeKeys)
		for _, size := range delta.removeSizes {
			plan.RemoveBytes += size
		}
	}
	return plan, nil
}
