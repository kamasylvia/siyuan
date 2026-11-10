// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// runProgress 记录单个备份任务的运行态:API handler 每请求构建 Engine 实例,
// 实例字段跨请求不可见,故用包级注册表
type runProgress struct {
	startedAt int64
	target    atomic.Value
	done      atomic.Int64
	total     atomic.Int64
}

// restoreProgress 记录包级还原运行态
type restoreProgress struct {
	running   atomic.Bool
	startedAt atomic.Int64
	target    atomic.Value
	done      atomic.Int64
	total     atomic.Int64
}

var (
	// runsMu 保护并发备份注册表:备份是共享锁,可多任务并行,
	// 全部结束后(注册表清空)只读状态才解除
	runsMu    sync.Mutex
	runs      = map[int64]*runProgress{}
	nextRunID atomic.Int64

	restoreTracker restoreProgress
)

// RunStateView 是备份在飞状态的只读快照;并行备份时为聚合视图
type RunStateView struct {
	StartedAt int64
	Target    string
	Done      int64
	Total     int64
	Runs      int64
}

// RestoreStateView 是还原在飞状态的只读快照
type RestoreStateView struct {
	StartedAt int64
	Target    string
	Done      int64
	Total     int64
}

// 锁状态变化回调:由内核启动时注册,用于向前端广播只读状态
var maintenanceChangeHook func(active bool, kind string)

// SetMaintenanceChangeHook 注册只读状态变化回调(进入/退出备份或还原时触发)
func SetMaintenanceChangeHook(hook func(active bool, kind string)) {
	maintenanceChangeHook = hook
}

func MaintenanceLocked() bool {
	return len(runs) > 0 || restoreTracker.running.Load()
}

func notifyMaintenance(kind string, active bool) {
	if maintenanceChangeHook != nil {
		maintenanceChangeHook(active, kind)
	}
}

// AcquireRunSlot 登记一个并发备份任务并返回其运行 ID
func AcquireRunSlot() int64 {
	runsMu.Lock()
	defer runsMu.Unlock()
	id := nextRunID.Add(1)
	runs[id] = &runProgress{startedAt: time.Now().Unix()}
	wasEmpty := len(runs) == 1
	if wasEmpty {
		go notifyMaintenance("backup", true)
	}
	return id
}

// ReleaseRunSlot 在备份任务返回前移除登记;最后一个任务结束即解除只读
func ReleaseRunSlot(id int64) {
	runsMu.Lock()
	delete(runs, id)
	empty := len(runs) == 0
	runsMu.Unlock()
	if empty {
		go notifyMaintenance("backup", false)
	}
}

// MarkRunTotals 在快照清单收集完成后设定处理总量
func MarkRunTotals(id int64, total int64) {
	runsMu.Lock()
	defer runsMu.Unlock()
	if run := runs[id]; run != nil {
		run.total.Store(total)
	}
}

// MarkRunTarget 在开始上传一个目标前调用:进度按目标重置
func MarkRunTarget(id int64, name string) {
	runsMu.Lock()
	defer runsMu.Unlock()
	if run := runs[id]; run != nil {
		run.target.Store(name)
		run.done.Store(0)
	}
}

// AddRunDone 在一个 blob 处理完成(上传或复用)后调用
func AddRunDone(id int64) {
	runsMu.Lock()
	defer runsMu.Unlock()
	if run := runs[id]; run != nil {
		run.done.Add(1)
	}
}

// RunState 返回在飞备份的聚合视图(全部任务求和);空闲时返回 nil。
// 并行备份互相独立,聚合展示不区分单个任务
func RunState() *RunStateView {
	runsMu.Lock()
	defer runsMu.Unlock()
	if len(runs) == 0 {
		return nil
	}
	view := &RunStateView{StartedAt: int64(1 << 62)}
	for _, run := range runs {
		view.Runs++
		view.Done += run.done.Load()
		view.Total += run.total.Load()
		if run.startedAt < view.StartedAt {
			view.StartedAt = run.startedAt
		}
		target, _ := run.target.Load().(string)
		if target != "" {
			if view.Target != "" {
				view.Target += ", "
			}
			view.Target += target
		}
	}
	return view
}

// MarkRestoreStarted 原子占位还原运行态并记录目标;已有还原在跑时返回 false
func MarkRestoreStarted(total int64, targetID string) bool {
	if !restoreTracker.running.CompareAndSwap(false, true) {
		return false
	}
	restoreTracker.startedAt.Store(time.Now().Unix())
	restoreTracker.target.Store(targetID)
	restoreTracker.done.Store(0)
	restoreTracker.total.Store(total)
	go notifyMaintenance("restore", true)
	return true
}

// MarkRestoreFinished 在 Restore 返回前调用
func MarkRestoreFinished() {
	restoreTracker.running.Store(false)
	restoreTracker.startedAt.Store(0)
	restoreTracker.target.Store("")
	restoreTracker.done.Store(0)
	restoreTracker.total.Store(0)
	go notifyMaintenance("restore", false)
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
	target, _ := restoreTracker.target.Load().(string)
	return &RestoreStateView{
		StartedAt: restoreTracker.startedAt.Load(),
		Target:    target,
		Done:      restoreTracker.done.Load(),
		Total:     restoreTracker.total.Load(),
	}
}

// RunCount 返回在飞备份任务数;0 表示空闲
func RunCount() int64 {
	runsMu.Lock()
	defer runsMu.Unlock()
	return int64(len(runs))
}

// RunRunsLabel 供聚合视图展示的运行数描述
func RunRunsLabel(view *RunStateView) string {
	return fmt.Sprintf("%d", view.Runs)
}
