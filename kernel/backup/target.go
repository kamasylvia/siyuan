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
)

// Store is a write-mostly object store. Keys are slash-separated and
// independent from the workspace layout.
type Store interface {
	ID() string
	Has(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

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
func NewTarget(target *Target) (Store, error) {
	switch target.Type {
	case TypeLocal:
		return NewLocalTarget(target.ID, target.Local.Path)
	case TypePG:
		return NewPGTarget(target.ID, target.PG.DSN, target.PG.Schema)
	case TypeS3:
		return NewS3Target(target)
	case TypeWebDAV:
		return NewWebDAVTarget(target)
	default:
		return nil, fmt.Errorf("backup target type [%s] is not available yet", target.Type)
	}
}
