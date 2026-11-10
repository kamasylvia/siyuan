// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"testing"
	"time"

	cronlib "github.com/robfig/cron/v3"
)

func TestNormalizeScheduleCron(t *testing.T) {
	cfg := NewConfig()
	cfg.Include = Include{Data: true, Conf: true}
	s3 := &S3{Endpoint: "https://s3.example.com", Bucket: "backup"}
	cfg.Targets = []*Target{
		{ID: "ok", Type: TypeS3, Enabled: true, S3: s3, Schedule: &Schedule{Cron: "0 2 * * *"}},
		{ID: "blank", Type: TypeS3, Enabled: true, S3: s3, Schedule: &Schedule{Cron: "   "}},
		{ID: "bad", Type: TypeS3, Enabled: true, S3: s3, Schedule: &Schedule{Cron: "not-a-cron"}},
	}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected an error for an invalid cron expression")
	}
	if cfg.Targets[0].Schedule == nil || cfg.Targets[0].Schedule.Cron != "0 2 * * *" {
		t.Fatalf("valid cron should be kept: %+v", cfg.Targets[0].Schedule)
	}
	if cfg.Targets[1].Schedule != nil {
		t.Fatalf("blank cron should normalize to no schedule: %+v", cfg.Targets[1].Schedule)
	}
}

func TestCronDueWindow(t *testing.T) {
	schedule, err := cronlib.ParseStandard("0 2 * * *")
	if err != nil {
		t.Fatal(err)
	}
	// 02:00 之后的一分钟窗口内到期;02:01 的窗口里 02:00 的触发表已经过去
	windowStart := time.Date(2026, 11, 9, 1, 59, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 11, 9, 2, 1, 0, 0, time.UTC)
	if schedule.Next(windowStart).After(windowEnd) {
		t.Fatal("02:00 fire should be due in the (01:59, 02:01] window")
	}
	windowStart = time.Date(2026, 11, 9, 2, 1, 0, 0, time.UTC)
	if !schedule.Next(windowStart).After(windowEnd) {
		t.Fatal("no fire should be due in the (02:01, 02:01] window")
	}
}
