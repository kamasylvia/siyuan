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
	// Fingerprint 返回物理存储身份:同一存储经不同连接配置(IP/域名)访问时
	// 指纹必须一致;存储从未写入过身份标记时返回空指纹。指纹用于删除门禁
	// 识别同一存储的重复配置
	Fingerprint(ctx context.Context) (string, error)
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
