// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"archive/zip"
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
	// LocalZipDir 非空时把本次快照落一份本地 zip 到该目录(离线备份出口)
	LocalZipDir string
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
	// ZipPath 非空表示本次快照同步落了一份本地 zip;ZipError 为落盘失败原因
	ZipPath  string `json:"zipPath,omitempty"`
	ZipError string `json:"zipError,omitempty"`
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
	// 备份是共享锁:并行备份互相独立(用户决策),进程内引用计数归零才解除只读;
	// 收集阶段只读,并发快照互不污染
	runID := AcquireRunSlot()
	defer ReleaseRunSlot(runID)
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
	// 本地 zip 是独立于目标的离线备份出口:目录已配置时,没有可用目标也允许运行
	if len(targets) == 0 && engine.LocalZipDir == "" {
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
	MarkRunTotals(runID, int64(len(entries)))
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
		MarkRunTarget(runID, target.ID())
		targetResult := engine.upload(ctx, target, snapshotID, manifest, entries, key, runID)
		result.Targets = append(result.Targets, targetResult)
	}
	// 本地 zip 落盘:加密时写入密文 blob(与目标存储一致),还原需要口令
	if engine.LocalZipDir != "" {
		if zipErr := engine.writeLocalZip(ctx, runID, snapshotID, manifest, entries, key); zipErr != nil {
			result.ZipError = zipErr.Error()
		} else {
			result.ZipPath = filepath.Join(engine.LocalZipDir, "siyuan-backup-"+snapshotID+".zip")
		}
	}
	result.FinishedAt = engine.now()
	engine.saveState(result)
	return result, nil
}

// writeLocalZip 把本次快照(manifest + 全部 blob)落成本地 zip,离线场景的
// 备份出口;zip 与目标存储对象同构,可离线还原也可回灌目标
func (engine *Engine) writeLocalZip(_ context.Context, runID int64, snapshotID string, manifest Manifest,
	entries []Entry, key []byte) error {
	if err := os.MkdirAll(engine.LocalZipDir, 0755); err != nil {
		return err
	}
	zipPath := filepath.Join(engine.LocalZipDir, "siyuan-backup-"+snapshotID+".zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	archive := zip.NewWriter(zipFile)
	defer archive.Close()
	writeEntry := func(name string, data []byte) error {
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = writeEntry(zipEntryDir+"/manifest.json", manifestData); err != nil {
		return err
	}
	meta, _ := json.Marshal(zipMeta{Version: 1, SnapshotID: snapshotID,
		CreatedAt: manifest.CreatedAt, Encrypted: len(key) > 0})
	if err = writeEntry(zipEntryDir+"/meta.json", meta); err != nil {
		return err
	}
	blobPrefix := BlobPrefix
	if len(key) > 0 {
		blobPrefix = EncryptedBlobPrefix
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(engine.WorkspaceDir, filepath.FromSlash(entry.Path)))
		if err != nil {
			return err
		}
		if len(key) > 0 {
			content, err = sealBlob(content, key)
			if err != nil {
				return err
			}
		}
		if err = writeEntry(blobPrefix+entry.Hash, content); err != nil {
			return err
		}
		MarkRunTarget(runID, "zip")
		AddRunDone(runID)
	}
	if err = archive.Close(); err != nil {
		return err
	}
	return nil
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
	entries []Entry, key []byte, runID int64) TargetResult {
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
			AddRunDone(runID)
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

// EmptyTarget 删除目标存储上的全部快照、blob 与 latest 引用,保留归属标记,
// 供在应用内清空目标后再删除配置,避免遗留孤儿快照;返回删除的快照与 blob 数量
func EmptyTarget(ctx context.Context, target Store) (int, int, error) {
	snapshots, blobs := 0, 0
	for _, prefix := range []string{ManifestPrefix, BlobPrefix, EncryptedBlobPrefix} {
		keys, err := target.List(ctx, prefix)
		if err != nil {
			return snapshots, blobs, err
		}
		for _, key := range keys {
			if err = target.Delete(ctx, key); err != nil {
				return snapshots, blobs, err
			}
			if prefix == ManifestPrefix {
				snapshots++
			} else {
				blobs++
			}
		}
	}
	if err := target.Delete(ctx, LatestRef); err != nil {
		return snapshots, blobs, err
	}
	return snapshots, blobs, nil
}

// DeleteSnapshot 删除单个快照及其不再被其它快照引用的数据块;任一兄弟快照
// 不可读时中止以防误删共享数据块;latest 引用指向被删快照时一并移除,返回
// 清理的数据块数量
func DeleteSnapshot(ctx context.Context, target Store, snapshotID string) (int, error) {
	key := ManifestPrefix + snapshotID + ".json"
	data, err := target.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	manifest := &Manifest{}
	if err = json.Unmarshal(data, manifest); err != nil {
		return 0, fmt.Errorf("parse snapshot %s: %w", snapshotID, err)
	}
	ids, err := target.List(ctx, ManifestPrefix)
	if err != nil {
		return 0, err
	}
	shared := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSuffix(strings.TrimPrefix(id, ManifestPrefix), ".json") == snapshotID {
			continue
		}
		otherData, err := target.Get(ctx, id)
		if err != nil {
			return 0, fmt.Errorf("read snapshot %s: %w", id, err)
		}
		other := &Manifest{}
		if err = json.Unmarshal(otherData, other); err != nil {
			return 0, fmt.Errorf("parse snapshot %s: %w", id, err)
		}
		prefix := BlobPrefix
		if other.Encryption != nil {
			prefix = EncryptedBlobPrefix
		}
		for _, entry := range other.Files {
			shared[prefix+entry.Hash] = true
		}
	}
	blobPrefix := BlobPrefix
	if manifest.Encryption != nil {
		blobPrefix = EncryptedBlobPrefix
	}
	removed := 0
	for _, entry := range manifest.Files {
		blobKey := blobPrefix + entry.Hash
		if shared[blobKey] {
			continue
		}
		if err = target.Delete(ctx, blobKey); err != nil {
			return 0, err
		}
		removed++
	}
	if err = target.Delete(ctx, key); err != nil {
		return 0, err
	}
	forgetSnapshotLatestRef(ctx, target, snapshotID)
	return removed, nil
}

// forgetSnapshotLatestRef 在 latest 引用指向被删快照时移除该引用;不存在或
// 指向其它快照时不动,清理失败不影响删除结果
func forgetSnapshotLatestRef(ctx context.Context, target Store, snapshotID string) {
	data, err := target.Get(ctx, LatestRef)
	if err != nil {
		return
	}
	var latest struct {
		Snapshot string `json:"snapshot"`
	}
	if json.Unmarshal(data, &latest) != nil || latest.Snapshot != snapshotID {
		return
	}
	target.Delete(ctx, LatestRef)
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
