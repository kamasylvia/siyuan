// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

// TargetCheck 描述一个备份目标的连通与快照状态;Fingerprint 为空表示无法获得
// 物理存储身份
type TargetCheck struct {
	Target      string
	Reachable   bool
	Snapshots   int
	Fingerprint string
}

// DeleteCheckResult 的 Reason 取值: ""=可删除, snapshots=目标上仍有快照,
// unreachable=目标不可达且无法证明已清空
type DeleteCheckResult struct {
	Target    string
	Deletable bool
	Reason    string
	Snapshots int
	Duplicate bool
}

// ApplyDeletionPolicy 实施删除门禁:只有确认已清空的目标才能删除配置,防止在
// 存储上遗留孤儿快照;同一物理存储(指纹相同)只要有一个可连接且已清空的配置,
// 其余不可达配置按重复配置放行
func ApplyDeletionPolicy(checks []TargetCheck) []DeleteCheckResult {
	provenEmpty := map[string]bool{}
	for _, check := range checks {
		if check.Reachable && check.Snapshots == 0 && check.Fingerprint != "" {
			provenEmpty[check.Fingerprint] = true
		}
	}
	results := make([]DeleteCheckResult, 0, len(checks))
	for _, check := range checks {
		result := DeleteCheckResult{Target: check.Target, Snapshots: check.Snapshots}
		switch {
		case check.Reachable && check.Snapshots == 0:
			result.Deletable = true
		case check.Reachable:
			result.Reason = "snapshots"
		case check.Fingerprint != "" && provenEmpty[check.Fingerprint]:
			result.Deletable = true
			result.Duplicate = true
		default:
			result.Reason = "unreachable"
		}
		results = append(results, result)
	}
	return results
}

// ApplyEditPolicy 编辑连接门禁:同一物理存储(指纹一致,如 IP 与域名两种配置)
// 或原存储确认已清空时放行;原目标仍有快照或不可达(无法证明已清空)时拒绝,
// 防止更换存储后把有快照的存储留在无人认领的状态
func ApplyEditPolicy(oldCheck, newCheck TargetCheck) (bool, string) {
	if oldCheck.Reachable && newCheck.Reachable &&
		oldCheck.Fingerprint != "" && oldCheck.Fingerprint == newCheck.Fingerprint {
		return true, ""
	}
	if oldCheck.Reachable && oldCheck.Snapshots == 0 {
		return true, ""
	}
	if oldCheck.Reachable {
		return false, "snapshots"
	}
	return false, "unreachable"
}
