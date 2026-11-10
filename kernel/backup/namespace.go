// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"crypto/rand"
	"math/big"
	"time"
)

const crockfordBase32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewNamespaceID 生成一个新的命名空间标识(ULID 形态:26 位 Crockford Base32,
// 前 10 位为毫秒时间戳,后 16 位为随机数)。目标创建时由内核自动分配,
// 用户无需也无法自定义,从而保证同一存储上的不同实例天然隔离
func NewNamespaceID() string {
	var enc [26]byte
	// 48 位毫秒时间戳编码进前 10 个字符(50 位容量的高 2 位为零)
	ms := uint64(time.Now().UnixMilli())
	for i := 9; i >= 0; i-- {
		enc[i] = crockfordBase32[ms&0x1F]
		ms >>= 5
	}
	// 80 位随机数编码进后 16 个字符
	var random [10]byte
	if _, err := rand.Read(random[:]); err != nil {
		// 随机源不可用时退化为时间戳派生,保持格式与唯一性下限
		fallback := uint64(time.Now().UnixNano())
		for i := range random {
			random[i] = byte(fallback >> (8 * (i % 8)))
		}
	}
	n := new(big.Int).SetBytes(random[:])
	mod := big.NewInt(32)
	for i := 25; i >= 10; i-- {
		var digit big.Int
		n.DivMod(n, mod, &digit)
		enc[i] = crockfordBase32[digit.Int64()]
	}
	return string(enc[:])
}
