// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	BlobPrefix          = "blobs/"
	EncryptedBlobPrefix = "blobs-e2e/"
	ManifestPrefix      = "manifests/"
	LatestRef           = "refs/latest.json"
)

type Entry struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	ModTime int64  `json:"mtime"`
}

type Manifest struct {
	ID         string              `json:"id"`
	CreatedAt  time.Time           `json:"createdAt"`
	Roots      []string            `json:"roots"`
	Files      []Entry             `json:"files"`
	TotalBytes int64               `json:"totalBytes"`
	Encryption *ManifestEncryption `json:"encryption,omitempty"`
}

type Engine struct {
	WorkspaceDir string
	Config       *Config
	Now          func() time.Time
	// 测试注入:非空时替代按配置构建的目标列表与对应 retention,绕过 conf 目标类型校验
	InjectedTargets   []Store
	InjectedRetention map[string]Retention
	// Takeover 接管已被其它实例认领的目标归属
	Takeover bool
}

type TargetResult struct {
	ID            string `json:"id"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	UploadedBlobs int    `json:"uploadedBlobs"`
	SkippedBlobs  int    `json:"skippedBlobs"`
	UploadedBytes int64  `json:"uploadedBytes"`
	Manifest      string `json:"manifest,omitempty"`
	// ClaimedBy/ClaimedHost/ClaimedAt 非空表示该目标被其它实例认领而拒绝备份
	ClaimedBy   string `json:"claimedBy,omitempty"`
	ClaimedHost string `json:"claimedHost,omitempty"`
	ClaimedAt   string `json:"claimedAt,omitempty"`
}

type RunResult struct {
	Snapshot   string         `json:"snapshot"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt time.Time      `json:"finishedAt"`
	Files      int            `json:"files"`
	TotalBytes int64          `json:"totalBytes"`
	Targets    []TargetResult `json:"targets"`
}

type TargetState struct {
	LastSnapshot  string    `json:"lastSnapshot"`
	LastOK        bool      `json:"lastOk"`
	LastError     string    `json:"lastError,omitempty"`
	LastRun       time.Time `json:"lastRun"`
	UploadedBlobs int       `json:"uploadedBlobs"`
	UploadedBytes int64     `json:"uploadedBytes"`
}

type State struct {
	UpdatedAt time.Time               `json:"updatedAt"`
	Last      *RunResult              `json:"last,omitempty"`
	Targets   map[string]*TargetState `json:"targets"`
}

func NewEngine(workspaceDir string, cfg *Config) *Engine {
	return &Engine{WorkspaceDir: workspaceDir, Config: cfg, Now: time.Now}
}

func (engine *Engine) Run(ctx context.Context, onlyTargets ...string) (*RunResult, error) {
	if engine == nil || engine.Config == nil {
		return nil, errors.New("backup engine has no config")
	}
	if err := engine.Config.Normalize(); err != nil {
		return nil, err
	}
	if !engine.Config.Enabled {
		return nil, errors.New("backup is disabled")
	}
	lock := flock.New(filepath.Join(engine.WorkspaceDir, "backup.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("another backup is already running")
	}
	defer func() { _ = lock.Unlock() }()
	MarkRunStarted()
	defer MarkRunFinished()
	selected := map[string]bool{}
	for _, id := range onlyTargets {
		selected[id] = true
	}
	var targets []Store
	var initFailures []TargetResult
	if len(engine.InjectedTargets) > 0 {
		targets = engine.InjectedTargets
	}
	for _, cfgTarget := range engine.Config.Targets {
		if len(engine.InjectedTargets) > 0 {
			break
		}
		if !cfgTarget.Enabled {
			continue
		}
		if len(selected) > 0 && !selected[cfgTarget.ID] {
			continue
		}
		target, err := NewTarget(cfgTarget)
		if err != nil {
			initFailures = append(initFailures, TargetResult{ID: cfgTarget.ID, Error: err.Error()})
			continue
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		if len(initFailures) > 0 {
			return nil, fmt.Errorf("no usable backup targets: %s", initFailures[0].Error)
		}
		return nil, errors.New("no enabled backup targets")
	}

	// 归属检查:同一存储可能被多个实例配置,只有最近认领的实例能写入
	var denied []DeniedTarget
	if instanceID := loadInstanceID(engine.WorkspaceDir); instanceID != "" {
		outcome, claimErr := claimTargets(ctx, targets, instanceID, engine.Takeover)
		if claimErr != nil {
			return nil, claimErr
		}
		denied = outcome.Denied
	}
	key, encryptionInfo, err := engine.encryption()
	if err != nil {
		return nil, err
	}
	entries, roots, err := engine.collect()
	if err != nil {
		return nil, err
	}
	MarkRunTotals(int64(len(entries)))
	now := engine.now()
	snapshotID := now.UTC().Format("20060102T150405Z") + "-" + randomSuffix()
	manifest := Manifest{ID: snapshotID, CreatedAt: now.UTC(), Roots: roots, Files: entries, Encryption: encryptionInfo}
	for _, entry := range entries {
		manifest.TotalBytes += entry.Size
	}
	result := &RunResult{
		Snapshot: snapshotID, StartedAt: now, Files: len(entries), TotalBytes: manifest.TotalBytes,
	}
	result.Targets = append(result.Targets, initFailures...)
	deniedByID := map[string]DeniedTarget{}
	for _, d := range denied {
		deniedByID[d.TargetID] = d
	}
	for _, target := range targets {
		if d, blocked := deniedByID[target.ID()]; blocked {
			result.Targets = append(result.Targets, TargetResult{ID: target.ID(),
				Error:     fmt.Sprintf("claimed by instance %s (%s) since %s", d.Owner.InstanceID, d.Owner.Hostname, d.Owner.ClaimedAt.Format(time.RFC3339)),
				ClaimedBy: d.Owner.InstanceID, ClaimedHost: d.Owner.Hostname,
				ClaimedAt: d.Owner.ClaimedAt.Format(time.RFC3339)})
			continue
		}
		MarkRunTarget(target.ID())
		targetResult := engine.upload(ctx, target, snapshotID, manifest, entries, key)
		result.Targets = append(result.Targets, targetResult)
	}
	result.FinishedAt = engine.now()
	engine.saveState(result)
	return result, nil
}

func (engine *Engine) now() time.Time {
	if engine.Now != nil {
		return engine.Now()
	}
	return time.Now()
}

func randomSuffix() string {
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(buf[:])
}

func (engine *Engine) collect() ([]Entry, []string, error) {
	var roots []string
	var entries []Entry
	type root struct {
		name string
		path string
	}
	candidates := []root{
		{"data", filepath.Join(engine.WorkspaceDir, "data")},
		{"conf", filepath.Join(engine.WorkspaceDir, "conf")},
		{"history", filepath.Join(engine.WorkspaceDir, "history")},
	}
	for _, candidate := range candidates {
		enabled := false
		switch candidate.name {
		case "data":
			enabled = engine.Config.Include.Data
		case "conf":
			enabled = engine.Config.Include.Conf
		case "history":
			enabled = engine.Config.Include.History
		}
		if !enabled {
			continue
		}
		info, err := os.Stat(candidate.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		if !info.IsDir() {
			continue
		}
		roots = append(roots, candidate.name)
		err = filepath.WalkDir(candidate.path, func(p string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return nil
			}
			if shouldSkipFile(entry.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(candidate.path, p)
			if relErr != nil {
				return relErr
			}
			key := path.Join(candidate.name, filepath.ToSlash(rel))
			if key == "conf/backup.json" {
				// Backup target credentials must never be uploaded to the targets themselves.
				return nil
			}
			if key == "data/storage/local.json" && !engine.Config.Include.LocalStorage {
				return nil
			}
			data, readErr := os.ReadFile(p)
			if readErr != nil {
				return readErr
			}
			info, statErr := entry.Info()
			if statErr != nil {
				return statErr
			}
			sum := sha256.Sum256(data)
			entries = append(entries, Entry{
				Path: key, Hash: hex.EncodeToString(sum[:]), Size: int64(len(data)),
				Mode: uint32(info.Mode().Perm()), ModTime: info.ModTime().Unix(),
			})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, roots, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".svn", "node_modules":
		return true
	}
	return false
}

func shouldSkipFile(name string) bool {
	if name == ".DS_Store" {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tmp") || strings.HasSuffix(lower, ".lock") ||
		strings.HasSuffix(lower, ".pid") || strings.HasPrefix(name, ".backup-")
}

func (engine *Engine) encryption() ([]byte, *ManifestEncryption, error) {
	if engine.Config.Encryption == nil || !engine.Config.Encryption.Enabled {
		return nil, nil, nil
	}
	info, key, err := newEncryptionInfo(engine.Config.Encryption.Passphrase)
	return key, info, err
}

func (engine *Engine) upload(ctx context.Context, target Store, snapshotID string, manifest Manifest,
	entries []Entry, key []byte) TargetResult {
	result := TargetResult{ID: target.ID()}
	for _, entry := range entries {
		if result.Error != "" {
			break
		}
		blobKey := BlobPrefix + entry.Hash
		if len(key) > 0 {
			blobKey = EncryptedBlobPrefix + entry.Hash
		}
		exists, err := target.Has(ctx, blobKey)
		if err != nil {
			result.Error = err.Error()
			break
		}
		if exists {
			result.SkippedBlobs++
			AddRunDone()
			continue
		}
		data, err := os.ReadFile(filepath.Join(engine.WorkspaceDir, filepath.FromSlash(entry.Path)))
		if err != nil {
			result.Error = err.Error()
			break
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != entry.Hash {
			result.Error = fmt.Sprintf("file changed during backup: %s", entry.Path)
			break
		}
		payload := data
		if len(key) > 0 {
			payload, err = sealBlob(key, data)
			if err != nil {
				result.Error = err.Error()
				break
			}
		}
		if err = target.Put(ctx, blobKey, payload); err != nil {
			result.Error = err.Error()
			break
		}
		result.UploadedBlobs++
		result.UploadedBytes += int64(len(payload))
	}
	if result.Error != "" {
		return result
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	manifestKey := ManifestPrefix + snapshotID + ".json"
	if err = target.Put(ctx, manifestKey, data); err != nil {
		result.Error = err.Error()
		return result
	}
	latest, _ := json.Marshal(map[string]any{
		"snapshot": snapshotID, "createdAt": manifest.CreatedAt, "files": len(entries),
		"totalBytes": manifest.TotalBytes,
	})
	if err = target.Put(ctx, LatestRef, latest); err != nil {
		result.Error = err.Error()
		return result
	}
	retention := engine.Config.RetentionFor(target.ID())
	if engine.InjectedRetention != nil {
		if injected, ok := engine.InjectedRetention[target.ID()]; ok {
			retention = injected
		}
	}
	if err = engine.prune(ctx, target, retention); err != nil {
		result.Error = "prune failed: " + err.Error()
		return result
	}
	result.OK = true
	result.Manifest = snapshotID
	return result
}

func (engine *Engine) saveState(result *RunResult) {
	statePath := filepath.Join(engine.WorkspaceDir, "backup-state.json")
	state := &State{Targets: map[string]*TargetState{}}
	if data, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(data, state)
	}
	if state.Targets == nil {
		state.Targets = map[string]*TargetState{}
	}
	state.UpdatedAt = result.FinishedAt
	state.Last = result
	for _, target := range result.Targets {
		state.Targets[target.ID] = &TargetState{
			LastSnapshot: result.Snapshot, LastOK: target.OK, LastError: target.Error,
			LastRun: result.FinishedAt, UploadedBlobs: target.UploadedBlobs, UploadedBytes: target.UploadedBytes,
		}
	}
	if data, err := json.MarshalIndent(state, "", "  "); err == nil {
		_ = os.WriteFile(statePath, data, 0644)
	}
}

func LoadState(workspaceDir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "backup-state.json"))
	if err != nil {
		return nil, err
	}
	state := &State{Targets: map[string]*TargetState{}}
	if err = json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	return state, nil
}

// ListSnapshots returns manifests stored on the target, newest first.
// Broken manifests (unreadable/unparseable) are skipped rather than failing
// the whole listing so one damaged object cannot hide healthy snapshots.
func ListSnapshots(ctx context.Context, target Store) ([]*Manifest, error) {
	ids, err := target.List(ctx, ManifestPrefix)
	if err != nil {
		return nil, err
	}
	var manifests []*Manifest
	for _, id := range ids {
		if !strings.HasSuffix(id, ".json") {
			continue
		}
		data, err := target.Get(ctx, id)
		if err != nil {
			continue
		}
		manifest := &Manifest{}
		if err = json.Unmarshal(data, manifest); err != nil {
			continue
		}
		if manifest.ID == "" {
			continue
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
	})
	return manifests, nil
}

// RestorePlan is a dry-run summary of a restore: what would be written, where.
type RestorePlan struct {
	Snapshot   string   `json:"snapshot"`
	CreatedAt  string   `json:"createdAt"`
	Roots      []string `json:"roots"`
	Files      int      `json:"files"`
	TotalBytes int64    `json:"totalBytes"`
	Encrypted  bool     `json:"encrypted"`
	// Conflicts 是目标中会被快照覆盖的已存在非空文件数;RemoveCount/RemoveBytes
	// 是 force 还原将清理的快照外文件规模
	Conflicts   int   `json:"conflicts"`
	RemoveCount int   `json:"removeCount"`
	RemoveBytes int64 `json:"removeBytes"`
}
