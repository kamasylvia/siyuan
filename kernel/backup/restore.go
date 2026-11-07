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
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RestoreResult struct {
	Snapshot string `json:"snapshot"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
}

// Restore materializes one manifest into dest. dest is expected to be an empty
// workspace; existing files are overwritten only when force is true.
func Restore(ctx context.Context, target Store, snapshotID, dest string, force bool, passphrase string) (*RestoreResult, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, errors.New("snapshot id is required")
	}
	if strings.TrimSpace(dest) == "" {
		return nil, errors.New("restore destination is required")
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	data, err := target.Get(ctx, ManifestPrefix+snapshotID+".json")
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{}
	if err = json.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("parse backup manifest [%s]: %w", snapshotID, err)
	}
	var encryptionKey []byte
	blobPrefix := BlobPrefix
	if manifest.Encryption != nil {
		if passphrase == "" {
			return nil, errors.New("backup is encrypted; passphrase is required")
		}
		encryptionKey, err = deriveEncryptionKey(passphrase, manifest.Encryption)
		if err != nil {
			return nil, err
		}
		blobPrefix = EncryptedBlobPrefix
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
		if !force {
			if info, statErr := os.Stat(destPath); statErr == nil && info.Size() > 0 {
				return nil, fmt.Errorf("restore destination already contains [%s]; use force to overwrite", entry.Path)
			}
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
	return result, nil
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
