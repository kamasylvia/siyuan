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
	"strings"
)

const (
	TypePG     = "pg"
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

type Target struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Enabled   bool       `json:"enabled"`
	PG        *PG        `json:"pg,omitempty"`
	S3        *S3        `json:"s3,omitempty"`
	WebDAV    *WebDAV    `json:"webdav,omitempty"`
	Retention *Retention `json:"retention,omitempty"`
}

type PG struct {
	// DSN is a legacy/expert connection string. When set it wins over the
	// structured fields below.
	DSN            string `json:"dsn,omitempty"`
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	User           string `json:"user,omitempty"`
	Password       string `json:"password,omitempty"`
	Database       string `json:"database,omitempty"`
	SSLMode        string `json:"sslMode,omitempty"`
	Schema         string `json:"schema,omitempty"`
	Timeout        int    `json:"timeout,omitempty"`
	ConcurrentReqs int    `json:"concurrentReqs,omitempty"`
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
	seen := map[string]bool{}
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
		switch target.Type {
		case TypePG:
			if target.PG == nil {
				return fmt.Errorf("backup target [%s] requires pg settings", target.ID)
			}
			if strings.TrimSpace(target.PG.DSN) == "" &&
				(strings.TrimSpace(target.PG.Host) == "" || strings.TrimSpace(target.PG.Database) == "") {
				return fmt.Errorf("backup target [%s] requires pg.host and pg.database", target.ID)
			}
			if target.PG.Port == 0 {
				target.PG.Port = 5432
			}
			if target.PG.SSLMode == "" {
				target.PG.SSLMode = "disable"
			}
			if target.PG.Schema == "" {
				target.PG.Schema = "public"
			}
			if !validIdentifier(target.PG.Schema) {
				return fmt.Errorf("backup target [%s] has invalid pg.schema [%s]", target.ID, target.PG.Schema)
			}
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
	}
	return nil
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
