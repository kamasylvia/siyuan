// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"fmt"
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

func NewTarget(target *Target) (Store, error) {
	switch target.Type {
	case TypePG:
		return NewPGTarget(target.ID, target.PG.ConnectionString(), target.PG.Schema)
	case TypeS3:
		return NewS3Target(target)
	case TypeWebDAV:
		return NewWebDAVTarget(target)
	default:
		return nil, fmt.Errorf("backup target type [%s] is not available yet", target.Type)
	}
}
