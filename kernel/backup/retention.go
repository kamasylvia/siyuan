// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"
)

// prune keeps the newest KeepLast manifests and removes manifests older than
// MaxAgeDays, then deletes blobs no longer referenced by any retained manifest.
// Both limits are optional; a zero value disables that dimension.
func (engine *Engine) prune(ctx context.Context, target Store, retention Retention) error {
	if retention.KeepLast <= 0 && retention.MaxAgeDays <= 0 {
		return nil
	}
	keys, err := target.List(ctx, ManifestPrefix)
	if err != nil {
		return err
	}
	var manifests []string
	for _, key := range keys {
		if strings.HasSuffix(key, ".json") {
			manifests = append(manifests, key)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(manifests)))
	cutoff := time.Time{}
	if retention.MaxAgeDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -retention.MaxAgeDays)
	}
	keep := make([]string, 0, len(manifests))
	for index, key := range manifests {
		remove := false
		if retention.KeepLast > 0 && index >= retention.KeepLast {
			remove = true
		}
		if !cutoff.IsZero() {
			if created, ok := manifestTime(key); ok && created.Before(cutoff) {
				remove = true
			}
		}
		if remove {
			if err = target.Delete(ctx, key); err != nil {
				return err
			}
			continue
		}
		keep = append(keep, key)
	}
	referenced := map[string]bool{}
	for _, key := range keep {
		data, readErr := target.Get(ctx, key)
		if readErr != nil {
			return readErr
		}
		manifest := &Manifest{}
		if err = json.Unmarshal(data, manifest); err != nil {
			return err
		}
		for _, entry := range manifest.Files {
			referenced[entry.Hash] = true
		}
	}
	for _, prefix := range []string{BlobPrefix, EncryptedBlobPrefix} {
		blobs, err := target.List(ctx, prefix)
		if err != nil {
			return err
		}
		for _, blob := range blobs {
			hash := path.Base(blob)
			if referenced[hash] {
				continue
			}
			if err = target.Delete(ctx, blob); err != nil {
				return err
			}
		}
	}
	return nil
}

func manifestTime(key string) (time.Time, bool) {
	name := strings.TrimSuffix(path.Base(key), ".json")
	if index := strings.IndexByte(name, '-'); index > 0 {
		name = name[:index]
	}
	created, err := time.Parse("20060102T150405Z", name)
	if err != nil {
		return time.Time{}, false
	}
	return created, true
}
