// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"net/url"
	"testing"
)

func TestPGConnectionStringFromFields(t *testing.T) {
	pg := &PG{Host: "db.example.com", Port: 5433, User: "siyuan", Password: "p@ss word", Database: "notes", SSLMode: "require"}
	dsn := pg.ConnectionString()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "postgres" || parsed.Host != "db.example.com:5433" || parsed.Path != "/notes" {
		t.Fatalf("unexpected dsn: %s", dsn)
	}
	if parsed.User.Username() != "siyuan" {
		t.Fatalf("unexpected user: %s", dsn)
	}
	if password, _ := parsed.User.Password(); password != "p@ss word" {
		t.Fatalf("unexpected password: %s", dsn)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("unexpected sslmode: %s", dsn)
	}
	if (&PG{DSN: "postgres://x"}).ConnectionString() != "postgres://x" {
		t.Fatal("explicit dsn should win")
	}
}

func TestPGStructuredFieldsNormalize(t *testing.T) {
	cfg := NewConfig()
	cfg.Targets = []*Target{{ID: "pg", Type: TypePG, Enabled: true, PG: &PG{Host: "db", Database: "notes"}}}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Targets[0].PG.Port != 5432 || cfg.Targets[0].PG.SSLMode != "disable" ||
		cfg.Targets[0].PG.Schema != "public" {
		t.Fatalf("defaults not applied: %+v", cfg.Targets[0].PG)
	}
}
