// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ownerKey 保存备份目标的归属标记:同一存储可能被多个实例(IP+域名相同)
// 配置,归属标记保证只有最近认领的实例能写入,避免快照与保留策略交叉污染
const ownerKey = "refs/owner.json"

// TargetOwner 描述一个备份目标当前被哪个实例认领
type TargetOwner struct {
	InstanceID string    `json:"instanceID"`
	Hostname   string    `json:"hostname,omitempty"`
	OS         string    `json:"os,omitempty"`
	ClaimedAt  time.Time `json:"claimedAt"`
}

// ClaimOutcome 是一次归属检查的结果:Claimed 为认领成功的目标,
// Denied 为被其它实例占用而拒绝的目标
type ClaimOutcome struct {
	Claimed []string
	Denied  []DeniedTarget
}

type DeniedTarget struct {
	TargetID string
	Owner    TargetOwner
}

// loadInstanceID 读取工作空间 conf.json 中的系统实例 ID;读取失败返回空,
// 调用方据此跳过归属检查(防异常配置误锁)
func loadInstanceID(workspaceDir string) string {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "conf", "conf.json"))
	if err != nil {
		return ""
	}
	var raw struct {
		System struct {
			ID string `json:"id"`
		} `json:"system"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}
	return raw.System.ID
}

// readOwner 读取目标上的归属标记;不存在或损坏时返回 nil
func readOwner(ctx context.Context, target Store) (*TargetOwner, error) {
	exists, err := target.Has(ctx, ownerKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	data, err := target.Get(ctx, ownerKey)
	if err != nil {
		return nil, err
	}
	owner := &TargetOwner{}
	if json.Unmarshal(data, owner) != nil {
		return nil, nil
	}
	return owner, nil
}

// writeOwner 把归属标记写到目标存储
func writeOwner(ctx context.Context, target Store, owner TargetOwner) error {
	data, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	return target.Put(ctx, ownerKey, data)
}

// claimTargets 对本次要备份的目标逐个做归属检查:无主则认领,属于当前实例
// 则放行,属于其它实例时按 takeover 决定接管或拒绝
func claimTargets(ctx context.Context, targets []Store, instanceID string, takeover bool) (*ClaimOutcome, error) {
	hostname, _ := os.Hostname()
	outcome := &ClaimOutcome{}
	for _, target := range targets {
		owner, err := readOwner(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("target [%s]: %w", target.ID(), err)
		}
		if owner == nil || owner.InstanceID == instanceID {
			if owner == nil {
				owner = &TargetOwner{InstanceID: instanceID, Hostname: hostname, OS: runtime.GOOS, ClaimedAt: time.Now().UTC()}
				if err = writeOwner(ctx, target, *owner); err != nil {
					return nil, fmt.Errorf("target [%s]: %w", target.ID(), err)
				}
			}
			outcome.Claimed = append(outcome.Claimed, target.ID())
			continue
		}
		if takeover {
			claimed := &TargetOwner{InstanceID: instanceID, Hostname: hostname, OS: runtime.GOOS, ClaimedAt: time.Now().UTC()}
			if err = writeOwner(ctx, target, *claimed); err != nil {
				return nil, fmt.Errorf("target [%s]: %w", target.ID(), err)
			}
			outcome.Claimed = append(outcome.Claimed, target.ID())
			continue
		}
		outcome.Denied = append(outcome.Denied, DeniedTarget{TargetID: target.ID(), Owner: *owner})
	}
	return outcome, nil
}
