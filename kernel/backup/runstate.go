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
	target    atomic.Value
	done      atomic.Int64
	total     atomic.Int64
}

// restoreProgress 记录包级还原运行态
type restoreProgress struct {
	running   atomic.Bool
	startedAt atomic.Int64
	done      atomic.Int64
	total     atomic.Int64
}

var (
	progress       runProgress
	restoreTracker restoreProgress
)

// RunStateView 是备份在飞状态的只读快照
type RunStateView struct {
	StartedAt int64
	Target    string
	Done      int64
	Total     int64
}

// RestoreStateView 是还原在飞状态的只读快照
type RestoreStateView struct {
	StartedAt int64
	Done      int64
	Total     int64
}

// MarkRunStarted 在 Engine.Run 实际开始工作时调用
func MarkRunStarted() {
	progress.running.Store(true)
	progress.startedAt.Store(time.Now().Unix())
}

// MarkRunFinished 在 Engine.Run 返回前调用
func MarkRunFinished() {
	progress.running.Store(false)
	progress.startedAt.Store(0)
	progress.target.Store("")
	progress.done.Store(0)
	progress.total.Store(0)
}

// MarkRunTotals 在快照清单收集完成后设定处理总量
func MarkRunTotals(total int64) {
	progress.total.Store(total)
}

// MarkRunTarget 在开始上传一个目标前调用:进度按目标重置
func MarkRunTarget(id string) {
	progress.target.Store(id)
	progress.done.Store(0)
}

// AddRunDone 在一个 blob 处理完成(上传或复用)后调用
func AddRunDone() {
	progress.done.Add(1)
}

// RunState 返回在飞备份快照;空闲时返回 nil
func RunState() *RunStateView {
	if !progress.running.Load() {
		return nil
	}
	target, _ := progress.target.Load().(string)
	return &RunStateView{
		StartedAt: progress.startedAt.Load(),
		Target:    target,
		Done:      progress.done.Load(),
		Total:     progress.total.Load(),
	}
}

// MarkRestoreStarted 原子占位还原运行态;已有还原在跑时返回 false
func MarkRestoreStarted(total int64) bool {
	if !restoreTracker.running.CompareAndSwap(false, true) {
		return false
	}
	restoreTracker.startedAt.Store(time.Now().Unix())
	restoreTracker.done.Store(0)
	restoreTracker.total.Store(total)
	return true
}

// MarkRestoreFinished 在 Restore 返回前调用
func MarkRestoreFinished() {
	restoreTracker.running.Store(false)
	restoreTracker.startedAt.Store(0)
	restoreTracker.done.Store(0)
	restoreTracker.total.Store(0)
}

// AddRestoreDone 在一个快照文件写盘后调用
func AddRestoreDone() {
	restoreTracker.done.Add(1)
}

// RestoreState 返回在飞还原快照;空闲时返回 nil
func RestoreState() *RestoreStateView {
	if !restoreTracker.running.Load() {
		return nil
	}
	return &RestoreStateView{
		StartedAt: restoreTracker.startedAt.Load(),
		Done:      restoreTracker.done.Load(),
		Total:     restoreTracker.total.Load(),
	}
}
