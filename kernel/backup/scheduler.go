// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"strings"
	"time"

	cronlib "github.com/robfig/cron/v3"
)

// StartScheduler 启动定时备份守护:每分钟检查一次各目标的 cron 触发表,窗口
// 内有触发表的目标自动执行备份;备份或还原进行中时跳过本轮(标准 cron 语义,
// 错过的触发表不补跑)。容器实例常驻负责触发,桌面实例打开时同样参与,写入
// 归属由认领机制去重
func StartScheduler(workspaceDir, configPath string) {
	go func() {
		lastCheck := time.Now()
		for now := range time.Tick(time.Minute) {
			if RunState() != nil || RestoreState() != nil {
				lastCheck = now
				continue
			}
			cfg, err := LoadConfig(configPath)
			if err != nil {
				lastCheck = now
				continue
			}
			var due []*Target
			for _, target := range cfg.Targets {
				if target == nil || !target.Enabled || target.Schedule == nil {
					continue
				}
				expr := strings.TrimSpace(target.Schedule.Cron)
				if expr == "" {
					continue
				}
				schedule, err := cronlib.ParseStandard(expr)
				if err != nil {
					continue // 保存时已校验,运行期遇到无效表达式只跳过
				}
				// 上次检查以来出现过触发表即视为到期
				if !schedule.Next(lastCheck).After(now) {
					due = append(due, target)
				}
			}
			lastCheck = now
			for _, target := range due {
				engine := NewEngine(workspaceDir, cfg)
				engine.Run(context.Background(), target.ID)
			}
		}
	}()
}
