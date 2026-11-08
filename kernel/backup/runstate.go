// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"sync/atomic"
	"time"
)

// runProgress 记录包级备份运行态:API handler 每请求构建 Engine 实例,
// 实例字段跨请求不可见,故用包级原子变量
type runProgress struct {
	running   atomic.Bool
	startedAt atomic.Int64
}

var progress runProgress

// MarkRunStarted 在 Engine.Run 实际开始工作时调用
func MarkRunStarted() {
	progress.running.Store(true)
	progress.startedAt.Store(time.Now().Unix())
}

// MarkRunFinished 在 Engine.Run 返回前调用
func MarkRunFinished() {
	progress.running.Store(false)
	progress.startedAt.Store(0)
}

// RunStartedAt 返回在飞备份的 Unix 起始秒;空闲时返回 0
func RunStartedAt() int64 {
	if progress.running.Load() {
		return progress.startedAt.Load()
	}
	return 0
}
