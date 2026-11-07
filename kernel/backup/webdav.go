// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

type WebDAVTarget struct {
	id     string
	client *gowebdav.Client
}

func NewWebDAVTarget(target *Target) (*WebDAVTarget, error) {
	if target == nil || target.WebDAV == nil {
		return nil, errors.New("webdav backup target is empty")
	}
	conf := target.WebDAV
	client := gowebdav.NewClient(conf.Endpoint, conf.Username, conf.Password)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: conf.SkipTLSVerify}
	client.SetTransport(transport)
	timeout := time.Duration(conf.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	client.SetTimeout(timeout)
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return &WebDAVTarget{id: target.ID, client: client}, nil
}

func (target *WebDAVTarget) ID() string { return target.id }

func webdavPath(key string) string {
	return "/" + strings.TrimPrefix(path.Clean("/"+key), "/")
}

func (target *WebDAVTarget) Has(_ context.Context, key string) (bool, error) {
	_, err := target.client.Stat(webdavPath(key))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (target *WebDAVTarget) Put(_ context.Context, key string, data []byte) error {
	p := webdavPath(key)
	if err := target.client.MkdirAll(path.Dir(p), 0755); err != nil {
		return err
	}
	return target.client.Write(p, data, 0644)
}

func (target *WebDAVTarget) Get(_ context.Context, key string) ([]byte, error) {
	data, err := target.client.Read(webdavPath(key))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, errors.New("backup object not found: " + key)
		}
		return nil, err
	}
	return data, nil
}

func (target *WebDAVTarget) Delete(_ context.Context, key string) error {
	err := target.client.Remove(webdavPath(key))
	if err != nil && gowebdav.IsErrNotFound(err) {
		return nil
	}
	return err
}

func (target *WebDAVTarget) List(_ context.Context, prefix string) ([]string, error) {
	root := webdavPath(prefix)
	info, err := target.client.Stat(root)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}
	if info != nil && !info.IsDir() {
		return []string{path.Clean(strings.TrimPrefix(root, "/"))}, nil
	}
	var keys []string
	var walk func(string) error
	walk = func(dir string) error {
		entries, readErr := target.client.ReadDir(dir)
		if readErr != nil {
			if gowebdav.IsErrNotFound(readErr) {
				return nil
			}
			return readErr
		}
		for _, entry := range entries {
			child := path.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := walk(child); err != nil {
					return err
				}
				continue
			}
			keys = append(keys, strings.TrimPrefix(child, "/"))
		}
		return nil
	}
	if err = walk(root); err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}
