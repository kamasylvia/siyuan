// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"strings"
)

// scopedStore 把全部键限定到一个命名空间前缀下:多实例共用同一物理存储时,
// 各实例用不同命名空间隔离快照谱系、认领标记与保留策略,互不可见。
// Fingerprint 保持物理存储身份(不含命名空间),供门禁识别「同一存储」。
type scopedStore struct {
	inner Store
	ns    string // 形如 "nas-a/",含尾斜杠;空字符串表示默认作用域(不加前缀)
}

// NewScopedStore 用命名空间包装存储;空命名空间原样返回
func NewScopedStore(store Store, namespace string) Store {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		return store
	}
	return &scopedStore{inner: store, ns: ns + "/"}
}

func (s *scopedStore) ID() string { return s.inner.ID() }

// Fingerprint 返回底层物理存储指纹:同一物理存储的不同命名空间指纹一致,
// 供编辑门禁识别「仅作用域写法变化」
func (s *scopedStore) Fingerprint(ctx context.Context) (string, error) {
	return s.inner.Fingerprint(ctx)
}

func (s *scopedStore) scope(key string) string { return s.ns + key }

func (s *scopedStore) Has(ctx context.Context, key string) (bool, error) {
	return s.inner.Has(ctx, s.scope(key))
}

func (s *scopedStore) Put(ctx context.Context, key string, data []byte) error {
	return s.inner.Put(ctx, s.scope(key), data)
}

func (s *scopedStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.inner.Get(ctx, s.scope(key))
}

func (s *scopedStore) Delete(ctx context.Context, key string) error {
	return s.inner.Delete(ctx, s.scope(key))
}

func (s *scopedStore) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := s.inner.List(ctx, s.ns+prefix)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i] = strings.TrimPrefix(keys[i], s.ns)
	}
	return keys, nil
}
