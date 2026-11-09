// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"fmt"

	"github.com/siyuan-note/siyuan/kernel/sql"
	"github.com/siyuan-note/siyuan/kernel/treenode"
	"github.com/siyuan-note/siyuan/kernel/util"
)

// RestoreReindexError 记录还原落盘后索引未就绪的笔记本范围；文件已写入，
// 搜索与反链仍是陈旧的，调用方必须按错误处理，不可视为还原可用。
type RestoreReindexError struct {
	FailedBoxes []string
}

func (e *RestoreReindexError) Error() string {
	return fmt.Sprintf("restored files written but database index is stale for notebooks %v", e.FailedBoxes)
}

// ReindexRestoredWorkspace 在还原落盘后同步重建当前工作空间的数据索引，
// 序列与增量同步的保护内路径一致：全量重建、引用解析、队列排空、虚拟引用缓存重置。
// 还原改写整库，与云同步串行；加密锁定或损坏被跳过的笔记本沿启动语义静默，
// 仅索引实际失败的笔记本进入失败集并复用 syncMergeResult 通道广播。
func ReindexRestoredWorkspace() error {
	lockSync()
	defer unlockSync()
	FullReindexDirect()
	IndexRefs()
	sql.FlushQueue()
	ResetVirtualBlockRefCache()
	var failed []string
	for _, box := range Conf.GetOpenedBoxes() {
		if IsUserGuide(box.ID) {
			continue
		}
		// indexBox 对未挂载的加密笔记本直接返回,不触及元数据状态;
		// 此处沿同一语义跳过,避免把「未解锁」误判为索引失败
		if IsEncryptedBox(box.ID) && !IsBoxUnlocked(box.ID) {
			continue
		}
		if treenode.DocumentMetadataState(box.ID) != "ready" {
			failed = append(failed, box.ID)
		}
	}
	if 0 < len(failed) {
		util.BroadcastByType("main", "syncMergeResult", 0, "", map[string]any{"upsertRootIDs": []string{}, "removeRootIDs": []string{}})
		return &RestoreReindexError{FailedBoxes: failed}
	}
	return nil
}
