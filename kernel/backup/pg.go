// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// PGTarget stores keys in a single objects table. The table is deliberately
// generic: backup keys are opaque (blobs/<sha>, manifests/<id>, refs/latest),
// so adding a new kind of object never changes the schema.
type PGTarget struct {
	id     string
	db     *sql.DB
	schema string
	table  string
}

func NewPGTarget(id, dsn, schema string) (*PGTarget, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("pg backup target [%s] has empty dsn", id)
	}
	if schema == "" {
		schema = "public"
	}
	if !validIdentifier(schema) {
		return nil, fmt.Errorf("pg backup target [%s] has invalid schema [%s]", id, schema)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect pg backup target [%s]: %w", id, err)
	}
	target := &PGTarget{id: id, db: db, schema: schema}
	target.table = fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent("siyuan_backup_objects"))
	stmt := fmt.Sprintf(`create table if not exists %s (
		path text primary key,
		data bytea not null,
		size bigint not null,
		updated_at timestamptz not null default now()
	)`, target.table)
	if _, err = db.ExecContext(ctx, stmt); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize pg backup target [%s]: %w", id, err)
	}
	return target, nil
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func (target *PGTarget) ID() string { return target.id }

func (target *PGTarget) Has(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := target.db.QueryRowContext(ctx,
		"select exists(select 1 from "+target.table+" where path = $1)", key).Scan(&exists)
	return exists, err
}

func (target *PGTarget) Put(ctx context.Context, key string, data []byte) error {
	_, err := target.db.ExecContext(ctx,
		"insert into "+target.table+" (path, data, size, updated_at) values ($1, $2, $3, now()) "+
			"on conflict (path) do update set data = excluded.data, size = excluded.size, updated_at = now()",
		key, data, len(data))
	return err
}

func (target *PGTarget) Get(ctx context.Context, key string) ([]byte, error) {
	var data []byte
	err := target.db.QueryRowContext(ctx, "select data from "+target.table+" where path = $1", key).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup object not found: %s", key)
	}
	return data, err
}

func (target *PGTarget) Delete(ctx context.Context, key string) error {
	_, err := target.db.ExecContext(ctx, "delete from "+target.table+" where path = $1", key)
	return err
}

func (target *PGTarget) List(ctx context.Context, prefix string) ([]string, error) {
	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix) + "%"
	rows, err := target.db.QueryContext(ctx,
		"select path from "+target.table+" where path like $1 escape '\\' order by path", pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ConnectionString prefers the explicit DSN and otherwise builds one from the
// structured settings shown in the settings UI.
func (pg *PG) ConnectionString() string {
	if pg == nil {
		return ""
	}
	if strings.TrimSpace(pg.DSN) != "" {
		return pg.DSN
	}
	host := strings.TrimSpace(pg.Host)
	if host == "" || strings.TrimSpace(pg.Database) == "" {
		return ""
	}
	port := pg.Port
	if port <= 0 {
		port = 5432
	}
	sslmode := pg.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + strings.TrimPrefix(pg.Database, "/")}
	if pg.User != "" {
		if pg.Password != "" {
			u.User = url.UserPassword(pg.User, pg.Password)
		} else {
			u.User = url.User(pg.User)
		}
	}
	query := url.Values{}
	query.Set("sslmode", sslmode)
	u.RawQuery = query.Encode()
	return u.String()
}
