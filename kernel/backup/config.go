// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package backup implements an independent, multi-target workspace backup
// pipeline. It deliberately does not reuse the sync provider: sync is
// bidirectional and single-target, while backup fans one immutable snapshot
// out to any number of write-only destinations.
package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	cronlib "github.com/robfig/cron/v3"
)

const (
	TypeS3     = "s3"
	TypeWebDAV = "webdav"

	DefaultKeepLast = 30
)

// Config is stored at conf/backup.json. It is intentionally independent from
// conf.json so enabling backup never changes sync settings or paid-provider
// gates.
type Config struct {
	Enabled    bool        `json:"enabled"`
	Include    Include     `json:"include"`
	Targets    []*Target   `json:"targets"`
	Encryption *Encryption `json:"encryption,omitempty"`
}

type Encryption struct {
	Enabled    bool   `json:"enabled"`
	Passphrase string `json:"passphrase,omitempty"`
}

type Include struct {
	Data         bool `json:"data"`
	Conf         bool `json:"conf"`
	History      bool `json:"history"`
	LocalStorage bool `json:"localStorage"` // data/storage/local.json (UI/plugin runtime state)
}

// namespacePattern 限定命名空间字符集:字母/数字开头,字母数字/-/_,最长 64;
// 禁止 "/" 与 "..",防止作用域键越界或与其它命名空间串拢
var namespacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type Target struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Enabled   bool       `json:"enabled"`
	S3        *S3        `json:"s3,omitempty"`
	WebDAV    *WebDAV    `json:"webdav,omitempty"`
	Retention *Retention `json:"retention,omitempty"`
	// Schedule 定时备份:cron 为标准 5 段表达式(分 时 日 月 周),由内核调度器触发
	Schedule *Schedule `json:"schedule,omitempty"`
	// Namespace 多实例共用同一存储时的作用域:各实例用不同命名空间隔离各自的
	// 快照谱系与认领标记;空字符串 = 默认作用域(兼容存量快照)。创建后不可修改
	Namespace string `json:"namespace,omitempty"`
}

// Schedule 定时备份计划;Cron 为空字符串或 nil 表示不启用
type Schedule struct {
	Cron string `json:"cron"`
}

type S3 struct {
	Endpoint      string `json:"endpoint"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	Bucket        string `json:"bucket"`
	Region        string `json:"region"`
	PathStyle     bool   `json:"pathStyle"`
	SkipTLSVerify bool   `json:"skipTlsVerify"`
	Timeout       int    `json:"timeout"`
}

type WebDAV struct {
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skipTlsVerify"`
	Timeout       int    `json:"timeout"`
}

// Retention is per target. Zero values fall back to DefaultKeepLast.
type Retention struct {
	KeepLast   int `json:"keepLast,omitempty"`
	MaxAgeDays int `json:"maxAgeDays,omitempty"`
}

func NewConfig() *Config {
	return &Config{
		Include: Include{Data: true, Conf: true},
	}
}

// LoadConfig reads conf/backup.json and normalizes defaults. A missing file is
// reported as an error so the caller can show the sample configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := NewConfig()
	if err = json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse backup config [%s]: %w", path, err)
	}
	if err = cfg.Normalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) Normalize() error {
	if cfg == nil {
		return fmt.Errorf("backup config is nil")
	}
	if !cfg.Include.Data && !cfg.Include.Conf && !cfg.Include.History {
		return fmt.Errorf("backup config includes no roots")
	}
	if cfg.Encryption != nil && cfg.Encryption.Enabled && strings.TrimSpace(cfg.Encryption.Passphrase) == "" {
		return fmt.Errorf("backup encryption is enabled but the passphrase is empty")
	}
	// 迁移:PG 备份目标已移除,加载时直接丢弃存量 pg 目标(PG 库数据不动),
	// 避免旧配置在无 pg 支持的构建上加载失败
	cfg.Targets = slices.DeleteFunc(cfg.Targets, func(t *Target) bool { return t != nil && t.Type == "pg" })
	seen := map[string]bool{}
	identities := map[string]string{}
	for i, target := range cfg.Targets {
		if target == nil {
			return fmt.Errorf("backup target %d is empty", i)
		}
		target.Type = strings.ToLower(strings.TrimSpace(target.Type))
		target.ID = strings.TrimSpace(target.ID)
		if target.ID == "" {
			return fmt.Errorf("backup target %d has no id", i)
		}
		if seen[target.ID] {
			return fmt.Errorf("duplicate backup target id [%s]", target.ID)
		}
		seen[target.ID] = true
		if target.Name == "" {
			target.Name = target.ID
		}
		if target.Retention == nil {
			target.Retention = &Retention{KeepLast: DefaultKeepLast, MaxAgeDays: 90}
		}
		if target.Retention.KeepLast <= 0 && target.Retention.MaxAgeDays <= 0 {
			target.Retention.KeepLast = DefaultKeepLast
		}
		if target.Schedule != nil {
			target.Schedule.Cron = strings.TrimSpace(target.Schedule.Cron)
			if target.Schedule.Cron == "" {
				target.Schedule = nil
			} else if _, err := cronlib.ParseStandard(target.Schedule.Cron); err != nil {
				return fmt.Errorf("backup target [%s] has invalid cron [%s]: %w", target.ID, target.Schedule.Cron, err)
			}
		}
		// 命名空间由内核自动分配:空值即新目标,生成 ULID;已有值(存量配置或
		// 再次保存)保持不变,保证快照谱系稳定
		target.Namespace = strings.TrimSpace(target.Namespace)
		if target.Namespace == "" {
			target.Namespace = NewNamespaceID()
		} else if !namespacePattern.MatchString(target.Namespace) {
			return fmt.Errorf("backup target [%s] has invalid namespace [%s]: use letters, digits, dash or underscore (max 64)", target.ID, target.Namespace)
		}
		switch target.Type {
		case TypeS3:
			if target.S3 == nil || target.S3.Endpoint == "" || target.S3.Bucket == "" {
				return fmt.Errorf("backup target [%s] requires s3.endpoint and s3.bucket", target.ID)
			}
		case TypeWebDAV:
			if target.WebDAV == nil || target.WebDAV.Endpoint == "" {
				return fmt.Errorf("backup target [%s] requires webdav.endpoint", target.ID)
			}
		default:
			return fmt.Errorf("backup target [%s] has unsupported type [%s]", target.ID, target.Type)
		}
		identity, idErr := storageIdentity(target)
		if idErr != nil {
			return fmt.Errorf("backup target [%s]: %w", target.ID, idErr)
		}
		if first, clash := identities[identity]; clash {
			return fmt.Errorf("backup target [%s] points to the same storage as [%s]; each target must use a unique host, port, database or bucket", target.ID, first)
		}
		identities[identity] = target.ID
	}
	return nil
}

// storageIdentity 归一化目标的连接标识:host、端口、数据库或桶相同即视为
// 同一存储,与 IP/域名的写法差异无关;DSN 写法用整串比较
func storageIdentity(target *Target) (string, error) {
	normalizeEndpoint := func(endpoint string) string {
		value := strings.ToLower(strings.TrimSpace(endpoint))
		value = strings.TrimSuffix(value, "/")
		value = strings.TrimPrefix(value, "https://")
		value = strings.TrimPrefix(value, "http://")
		return value
	}
	switch target.Type {
	case TypeS3:
		return "s3|" + normalizeEndpoint(target.S3.Endpoint) + "|" + strings.ToLower(strings.TrimSpace(target.S3.Bucket)), nil
	case TypeWebDAV:
		return "webdav|" + normalizeEndpoint(target.WebDAV.Endpoint), nil
	}
	return "", fmt.Errorf("unsupported type [%s]", target.Type)
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (target *Target) RetentionSpec() Retention {
	if target == nil || target.Retention == nil {
		return Retention{KeepLast: DefaultKeepLast, MaxAgeDays: 90}
	}
	return *target.Retention
}

func (cfg *Config) RetentionFor(id string) Retention {
	if cfg != nil {
		for _, target := range cfg.Targets {
			if target.ID == id {
				return target.RetentionSpec()
			}
		}
	}
	return Retention{KeepLast: DefaultKeepLast}
}

// SaveConfig writes conf/backup.json atomically with owner-only permissions.
func SaveConfig(path string, cfg *Config) error {
	if err := cfg.Normalize(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backup-config-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0600); err != nil {
		return err
	}
	return os.Rename(name, path)
}
