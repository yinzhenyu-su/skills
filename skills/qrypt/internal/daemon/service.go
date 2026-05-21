package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	qryptsync "github.com/yinzhenyu/skills/qrypt/internal/sync"
)


// Daemon manages the daemon lifecycle and delegates mount ops to MountManager.
type Daemon struct {
	cfg       *config.Config
	cfgPath   string
	version   string
	eventMgr  *EventManager
	sessionMgr *SessionManager
	manager   *MountManager
	progress   *ProgressHub
	rateLimit  *RateLimiter

	mu         sync.RWMutex
	startedAt  time.Time
	mountState protocol.MountState
	lastError  string
}

func NewDaemon(cfg *config.Config, version string) *Daemon {
	sm := NewSessionManager()
	em := NewEventManager()
	ph := NewProgressHub(em)
	mm := NewMountManager(cfg, sm)
	orch := NewOrchestrator(3, NewRateLimiter(0), ph)
	mm.SetOrchestrator(orch)
	d := &Daemon{
		cfg:        cfg,
		version:    version,
		eventMgr:   em,
		sessionMgr: sm,
		manager:    mm,
		progress:   ph,
		rateLimit:  NewRateLimiter(0),
	}
	NewCacheInvalidator(mm, em)
	return d
}

func NewDaemonWithPath(cfg *config.Config, cfgPath, version string) *Daemon {
	sm := NewSessionManager()
	em := NewEventManager()
	ph := NewProgressHub(em)
	mm := NewMountManager(cfg, sm)
	orch := NewOrchestrator(3, NewRateLimiter(0), ph)
	mm.SetOrchestrator(orch)
	d := &Daemon{
		cfg:        cfg,
		cfgPath:    cfgPath,
		version:    version,
		eventMgr:   em,
		sessionMgr: sm,
		manager:    mm,
		progress:   ph,
		rateLimit:  NewRateLimiter(0),
	}
	NewCacheInvalidator(mm, em)
	return d
}

// deriveState computes the aggregate daemon state from individual mount states.
func deriveState(mounts []MountSummary) protocol.MountState {
	if len(mounts) == 0 {
		return protocol.MountStateUnmounted
	}
	var hasMounted, hasError, hasMounting bool
	for _, m := range mounts {
		switch m.State {
		case protocol.MountStateMounted:
			hasMounted = true
		case protocol.MountStateError:
			hasError = true
		case protocol.MountStateMounting, protocol.MountStateUnmounting:
			hasMounting = true
		}
	}
	switch {
	case hasMounting:
		return protocol.MountStateMounting
	case hasMounted && !hasError:
		return protocol.MountStateMounted
	case hasMounted && hasError:
		return protocol.MountStateMounted // partial failure
	case hasError:
		return protocol.MountStateError
	default:
		return protocol.MountStateUnmounted
	}
}

func (d *Daemon) Status() (*protocol.DaemonStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	uptime := "not started"
	if !d.startedAt.IsZero() {
		uptime = time.Since(d.startedAt).Round(time.Second).String()
	}

	mounts := d.manager.List()
	state := d.mountState
	if state == "" {
		state = deriveState(mounts)
	}

	firstMountPoint := ""
	firstDriveType := ""
	if len(mounts) > 0 {
		firstMountPoint = mounts[0].MountPoint
		firstDriveType = mounts[0].DriveType
	}

	return &protocol.DaemonStatus{
		Version:    d.version,
		Uptime:     uptime,
		MountPoint: firstMountPoint,
		MountState: state,
		DriveType:  firstDriveType,
		LastError:  d.lastError,
		Mounts:     mounts,
	}, nil
}

// Start starts one or all mounts. If name is empty, starts all enabled.
func (d *Daemon) Start(ctx context.Context, name string) error {
	if name != "" {
		return d.manager.Start(ctx, name)
	}
	return d.manager.StartAll(ctx)
}

// DaemonShutdown stops all mounts and the orchestrator, then waits for cleanup.
func (d *Daemon) DaemonShutdown(ctx context.Context) error {
	return d.manager.Shutdown(ctx)
}

func (d *Daemon) orchestrator() *Orchestrator {
	return d.manager.orchestrator
}

// Stop stops one or all mounts. If name is empty, stops all running.
func (d *Daemon) Stop(ctx context.Context, name string) error {
	if name != "" {
		return d.manager.Stop(ctx, name)
	}
	return d.manager.StopAll(ctx)
}

func (d *Daemon) GetConfig() (*config.Config, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg, nil
}

func (d *Daemon) InitConfig(_ context.Context, path string) (string, error) {
	if path == "" {
		path = filepath.Join(config.WorkDir(), "qrypt.toml")
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("config file already exists: %s", path)
	}
	if err := config.WriteDefaultConfig(path); err != nil {
		return "", fmt.Errorf("write default config: %w", err)
	}
	return path, nil
}

func (d *Daemon) ReloadConfig(ctx context.Context) (*protocol.ReloadResult, error) {
	_, cfg, vr, err := config.LoadConfigAuto(d.cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if !vr.Valid {
		return nil, fmt.Errorf("invalid config")
	}

	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()

	return d.manager.Reload(ctx, cfg)
}

func (d *Daemon) ValidateConfig(path string) (*config.ValidationResult, error) {
	if path != "" {
		return config.ValidateConfigFile(path), nil
	}
	d.mu.RLock()
	cfg := d.cfg
	d.mu.RUnlock()
	return config.ValidateConfig(cfg), nil
}

func (d *Daemon) SyncStatus() (*protocol.SyncStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var totalPending, totalBytes int64
	for _, m := range d.manager.List() {
		if m.State != protocol.MountStateMounted {
			continue
		}
		inst, _ := d.manager.Get(m.Name)
		if inst == nil || inst.Cache == nil {
			continue
		}
		nodes := inst.Cache.GetPendingNodes()
		totalPending += int64(len(nodes))
		for _, n := range nodes {
			totalBytes += n.Size
		}
	}

	return &protocol.SyncStats{
		PendingUploads:    int(totalPending),
		TotalBytesPending: totalBytes,
	}, nil
}

func (d *Daemon) GetSyncTaskList() ([]protocol.SyncTaskInfo, error) {
	var tasks []protocol.SyncTaskInfo
	for _, m := range d.manager.List() {
		if m.State != protocol.MountStateMounted {
			continue
		}
		inst, _ := d.manager.Get(m.Name)
		if inst == nil || inst.Cache == nil {
			continue
		}
		nodes := inst.Cache.GetPendingNodes()
		for _, n := range nodes {
			tasks = append(tasks, protocol.SyncTaskInfo{
				Fid:   n.Fid,
				Name:  m.Name + ":" + n.Name,
				Size:  n.Size,
				State: string(protocol.EventSyncProgress),
			})
		}
	}
	return tasks, nil
}

func (d *Daemon) CacheUsage() (*protocol.CacheUsage, error) {
	var totalStagingCount int
	for _, m := range d.manager.List() {
		if m.State != protocol.MountStateMounted {
			continue
		}
		inst, _ := d.manager.Get(m.Name)
		if inst == nil || inst.Cache == nil {
			continue
		}
		files, _ := inst.Cache.Staging().ListStagingFiles()
		totalStagingCount += len(files)
	}
	return &protocol.CacheUsage{
		StagingCount: totalStagingCount,
	}, nil
}

// PushStart starts a push operation in a background goroutine.
// Returns a task ID immediately. Progress is published via EventManager.
func (d *Daemon) PushStart(ctx context.Context, params protocol.PushStartParams) (*protocol.PushStartResult, error) {
	mountName := d.resolveMountName(params.MountName)
	if mountName == "" {
		return nil, fmt.Errorf("no enabled mount found")
	}

	mountCfg := config.FindMount(d.cfg, mountName)
	if mountCfg == nil {
		return nil, fmt.Errorf("mount %q not found in config", mountName)
	}

	rc := d.cfg.MergeInstanceConfig(*mountCfg)

	cipher, err := d.makeCipher(rc, params.Password, params.Salt)
	if err != nil {
		return nil, err
	}

	sk, _ := SessionKeyForMount(rc)
	s, err := d.sessionMgr.Acquire(ctx, sk, rc.Params)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}

	transfers := params.Transfers
	if transfers <= 0 {
		transfers = 4
	}
	taskID := fmt.Sprintf("push_%d", time.Now().UnixNano())

	go d.runPushTask(ctx, taskID, s, sk, cipher, rc, params, transfers)

	return &protocol.PushStartResult{TaskID: taskID, FileCount: -1}, nil
}

func (d *Daemon) runPushTask(ctx context.Context, taskID string, s *Session, sk SessionKey, cipher *crypt.RcloneCipher, rc *config.ResolvedMountConfig, params protocol.PushStartParams, transfers int) {
	drv := s.Drv
	defer d.sessionMgr.Release(ctx, sk)

	mountCfg := config.FindMount(d.cfg, rc.Name)
	rootPath := "/"
	if mountCfg != nil {
		rootPath = config.RootPathForMount(*mountCfg)
	}
	fullRemotePath := config.ResolveFullPath(rootPath, params.Remote)

	mountName := rc.Name

	d.publishPushEvent(taskID, mountName, "started", "", 0, 0, "")

	localInfo, err := os.Stat(params.Source)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("stat source: %v", err))
		return
	}

	if localInfo.IsDir() {
		d.pushDirectory(ctx, taskID, mountName, drv, cipher, rc, params, transfers, localInfo, fullRemotePath)
	} else {
		d.pushSingleFile(ctx, taskID, mountName, drv, cipher, fullRemotePath, params, localInfo)
	}
}

func (d *Daemon) pushSingleFile(ctx context.Context, taskID, mountName string, drv drive.Driver, cipher *crypt.RcloneCipher, fullRemotePath string, params protocol.PushStartParams, localInfo os.FileInfo) {
	parentFid, err := d.resolveParent(ctx, drv, fullRemotePath)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("resolve path: %v", err))
		return
	}

	remoteFileName := baseOf(fullRemotePath)
	uploader := qryptsync.NewUploader(drv, cipher)
	req := qryptsync.Request{
		Name:      remoteFileName,
		ParentFid: parentFid,
		PlainSize: localInfo.Size(),
		DataReader: func() (io.ReadCloser, error) {
			return os.Open(params.Source)
		},
		ProgressFn: func(partNumber int) {
			d.publishPushEvent(taskID, mountName, "uploading", params.Source, int64(partNumber), localInfo.Size(), "")
		},
	}

	result, err := uploader.Upload(ctx, req)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", params.Source, 0, 0, fmt.Sprintf("upload: %v", err))
		return
	}

	d.publishPushEvent(taskID, mountName, "completed", params.Source, result.EncryptedSize, result.EncryptedSize, "")
}

func (d *Daemon) pushDirectory(ctx context.Context, taskID, mountName string, drv drive.Driver, cipher *crypt.RcloneCipher, rc *config.ResolvedMountConfig, params protocol.PushStartParams, transfers int, localInfo os.FileInfo, fullRemotePath string) {
	parentFid, err := d.resolveParent(ctx, drv, fullRemotePath)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("resolve path: %v", err))
		return
	}

	dirName := baseOf(fullRemotePath)
	w, hasMkdir := drv.(drive.Writer)
	if hasMkdir && !params.DryRun {
		entries, _ := drv.List(ctx, parentFid)
		encDirName := cipher.EncryptSegment(dirName)
		found := false
		for _, e := range entries {
			if e.IsDir && e.Name == encDirName {
				parentFid = e.ID
				found = true
				break
			}
		}
		if !found {
			ne, err := w.Mkdir(ctx, parentFid, encDirName)
			if err != nil {
				d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("mkdir: %v", err))
				return
			}
			parentFid = ne.ID
		}
	}

	orch := d.orchestrator()
	uploader := qryptsync.NewUploader(drv, cipher)
	dirFids := make(map[string]string)
	dirFids["."] = parentFid
	remoteCache := make(map[string][]drive.Entry)

	var wg sync.WaitGroup
	var walkErr error

	walkErr = filepath.Walk(params.Source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(params.Source, path)
		if relPath == "." {
			return nil
		}
		parentRel := filepath.Dir(relPath)
		currentParent := dirFids[parentRel]

		if _, ok := remoteCache[currentParent]; !ok {
			entries, _ := drv.List(ctx, currentParent)
			remoteCache[currentParent] = entries
		}

		if info.IsDir() {
			encName := cipher.EncryptSegment(info.Name())
			var currentFid string
			found := false
			for _, e := range remoteCache[currentParent] {
				if e.IsDir && e.Name == encName {
					currentFid = e.ID
					found = true
					break
				}
			}
			if !found && hasMkdir && !params.DryRun {
				ne, mkerr := w.Mkdir(ctx, currentParent, encName)
				if mkerr == nil {
					currentFid = ne.ID
				}
			} else if params.DryRun && !found {
				currentFid = "mock-" + encName
			}
			dirFids[relPath] = currentFid
			return nil
		}

		if params.Update {
			encName := cipher.EncryptSegment(info.Name())
			shouldSkip := false
			for _, e := range remoteCache[currentParent] {
				if !e.IsDir && e.Name == encName {
					plainSize, derr := cipher.DecryptedSize(e.Size)
					if derr == nil && plainSize == info.Size() {
						shouldSkip = true
						break
					}
				}
			}
			if shouldSkip {
				return nil
			}
		}

		filePath := path
		remoteParent := currentParent
		remoteName := info.Name()
		fileSize := info.Size()
		wg.Add(1)
		orch.Submit(func(ctx context.Context) error {
			defer wg.Done()
			req := qryptsync.Request{
				Name:      remoteName,
				ParentFid: remoteParent,
				PlainSize: fileSize,
				DataReader: func() (io.ReadCloser, error) {
					return os.Open(filePath)
				},
				ProgressFn: func(partNumber int) {
					d.publishPushEvent(taskID, mountName, "uploading", filePath, int64(partNumber), fileSize, "")
				},
			}
			if params.DryRun {
				return nil
			}
			_, uerr := uploader.Upload(ctx, req)
			return uerr
		})
		return nil
	})

	if walkErr != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("scan: %v", walkErr))
		return
	}

	wg.Wait()
	if params.DryRun {
		d.publishPushEvent(taskID, mountName, "completed", "", 0, 0, "")
	} else {
		d.publishPushEvent(taskID, mountName, "completed", "", 0, 0, "")
	}
}

func (d *Daemon) publishPushEvent(taskID, mount, state, file string, bytes, total int64, errMsg string) {
	d.progress.Publish(&ProgressEntry{
		TaskID:    taskID,
		Mount:     mount,
		Direction: "push",
		File:      file,
		Bytes:     bytes,
		Total:     total,
		State:     state,
		Error:     errMsg,
	})
}

func (d *Daemon) resolveParent(ctx context.Context, drv drive.Driver, fullRemotePath string) (string, error) {
	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		return "", fmt.Errorf("driver does not support path resolution")
	}
	parentPath := d.dirOf(fullRemotePath)
	return resolver.ResolvePath(ctx, parentPath)
}

func (d *Daemon) resolveMountName(name string) string {
	if name != "" {
		for _, m := range d.cfg.Mounts {
			if m.Name == name {
				return name
			}
		}
		return ""
	}
	for _, m := range d.cfg.Mounts {
		rc := d.cfg.MergeInstanceConfig(m)
		if rc.Enabled {
			return m.Name
		}
	}
	return ""
}

func (d *Daemon) makeCipher(rc *config.ResolvedMountConfig, pwd, salt string) (*crypt.RcloneCipher, error) {
	return config.MakeCipher(rc.Encryption, d.cfg.Defaults.Encryption, pwd, salt)
}

func (d *Daemon) dirOf(path string) string {
	idx := strings.LastIndex(strings.TrimRight(path, "/"), "/")
	if idx < 0 {
		return "/"
	}
	return path[:idx+1]
}

func baseOf(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

// withMountSession borrows a session and cipher for a mount, calls fn, then releases.
func (d *Daemon) withMountSession(ctx context.Context, mountName, password, salt string, fn func(drv drive.Driver, cipher *crypt.RcloneCipher) error) error {
	name := d.resolveMountName(mountName)
	if name == "" {
		return fmt.Errorf("no enabled mount found")
	}
	mountCfg := config.FindMount(d.cfg, name)
	if mountCfg == nil {
		return fmt.Errorf("mount %q not found", mountName)
	}
	rc := d.cfg.MergeInstanceConfig(*mountCfg)

	cipher, err := d.makeCipher(rc, password, salt)
	if err != nil {
		return err
	}

	sk, _ := SessionKeyForMount(rc)
	s, err := d.sessionMgr.Acquire(ctx, sk, rc.Params)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	defer d.sessionMgr.Release(ctx, sk)

	return fn(s.Drv, cipher)
}

// ListDir lists a remote directory via the daemon.
func (d *Daemon) ListDir(ctx context.Context, params protocol.ListDirParams) (*protocol.ListDirResult, error) {
	var result protocol.ListDirResult
	err := d.withMountSession(ctx, params.MountName, params.Password, params.Salt, func(drv drive.Driver, cipher *crypt.RcloneCipher) error {
		mountCfg := config.FindMount(d.cfg, d.resolveMountName(params.MountName))
		rootPath := "/"
		if mountCfg != nil {
			rootPath = config.RootPathForMount(*mountCfg)
		}
		fullPath := config.ResolveFullPath(rootPath, params.Path)
		result.Path = fullPath

		resolver, ok := drv.(drive.PathResolver)
		if !ok {
			return fmt.Errorf("driver does not support path resolution")
		}
		fid, err := resolver.ResolvePath(ctx, fullPath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		entries, err := drv.List(ctx, fid)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}

		for _, e := range entries {
			decName, decErr := cipher.DecryptSegment(e.Name)
			if decErr != nil {
				decName = e.Name
			}
			plainSize, decSizeErr := cipher.DecryptedSize(e.Size)
			if decSizeErr != nil {
				plainSize = e.Size
			}
			result.Entries = append(result.Entries, protocol.ListEntryItem{
				ID:        e.ID,
				Name:      e.Name,
				DecName:   decName,
				IsDir:     e.IsDir,
				Size:      e.Size,
				PlainSize: plainSize,
				ModTime:   e.ModTime.UnixMilli(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Find recursively searches for files matching a pattern.
func (d *Daemon) Find(ctx context.Context, params protocol.FindParams) (*protocol.FindResult, error) {
	var result protocol.FindResult
	err := d.withMountSession(ctx, params.MountName, params.Password, params.Salt, func(drv drive.Driver, cipher *crypt.RcloneCipher) error {
		mountCfg := config.FindMount(d.cfg, d.resolveMountName(params.MountName))
		rootPath := "/"
		if mountCfg != nil {
			rootPath = config.RootPathForMount(*mountCfg)
		}
		fullPath := config.ResolveFullPath(rootPath, params.Path)

		resolver, ok := drv.(drive.PathResolver)
		if !ok {
			return fmt.Errorf("driver does not support path resolution")
		}
		fid, err := resolver.ResolvePath(ctx, fullPath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

			maxDepth := params.MaxDepth
		if maxDepth == 0 {
			maxDepth = -1 // default: unlimited
		}

		pattern := params.Pattern
		if pattern == "" {
			return d.walkEntries(ctx, drv, cipher, fid, fullPath, 0, maxDepth, &result)
		}

		return d.walkAndMatch(ctx, drv, cipher, fid, fullPath, 0, maxDepth, params.MaxMatches, pattern, params.CaseSensitive, &result)
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *Daemon) walkAndMatch(ctx context.Context, drv drive.Driver, cipher *crypt.RcloneCipher, fid, displayPath string, depth, maxDepth, maxMatches int, pattern string, caseSensitive bool, result *protocol.FindResult) error {
	if maxDepth >= 0 && depth > maxDepth {
		return nil
	}
	if maxMatches > 0 && result.Count >= maxMatches {
		return nil
	}

	entries, err := drv.List(ctx, fid)
	if err != nil {
		return fmt.Errorf("list %s: %w", fid, err)
	}

	matchName := func(name string) bool {
		if caseSensitive {
			return strings.Contains(name, pattern)
		}
		return strings.Contains(strings.ToLower(name), strings.ToLower(pattern))
	}

	for _, e := range entries {
		decName, decErr := cipher.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}
		childPath := displayPath + "/" + decName

		if matchName(decName) {
			if maxMatches > 0 && result.Count >= maxMatches {
				break
			}
			result.Entries = append(result.Entries, protocol.FindEntry{
				Path:  childPath,
				IsDir: e.IsDir,
				Size:  e.Size,
			})
			result.Count++
		}

		if e.IsDir {
			if err := d.walkAndMatch(ctx, drv, cipher, e.ID, childPath, depth+1, maxDepth, maxMatches, pattern, caseSensitive, result); err != nil {
				continue
			}
		}
	}
	return nil
}

func (d *Daemon) walkEntries(ctx context.Context, drv drive.Driver, cipher *crypt.RcloneCipher, fid, displayPath string, depth, maxDepth int, result *protocol.FindResult) error {
	if maxDepth >= 0 && depth > maxDepth {
		return nil
	}

	entries, err := drv.List(ctx, fid)
	if err != nil {
		return fmt.Errorf("list %s: %w", fid, err)
	}

	for _, e := range entries {
		decName, decErr := cipher.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}
		childPath := displayPath + "/" + decName

		result.Entries = append(result.Entries, protocol.FindEntry{
			Path:  childPath,
			IsDir: e.IsDir,
			Size:  e.Size,
		})
		result.Count++

		if e.IsDir {
			if err := d.walkEntries(ctx, drv, cipher, e.ID, childPath, depth+1, maxDepth, result); err != nil {
				return err
			}
		}
	}
	return nil
}

// Mkdir creates a remote directory via the daemon.
func (d *Daemon) Mkdir(ctx context.Context, params protocol.MkdirParams) (*protocol.MkdirResult, error) {
	var result protocol.MkdirResult
	err := d.withMountSession(ctx, params.MountName, params.Password, params.Salt, func(drv drive.Driver, cipher *crypt.RcloneCipher) error {
		mountCfg := config.FindMount(d.cfg, d.resolveMountName(params.MountName))
		rootPath := "/"
		if mountCfg != nil {
			rootPath = config.RootPathForMount(*mountCfg)
		}
		fullPath := config.ResolveFullPath(rootPath, params.Path)

		w, ok := drv.(drive.Writer)
		if !ok {
			return fmt.Errorf("driver does not support write operations")
		}

		if params.Parents {
			fid, err := createRemoteDir(ctx, drv, w, cipher, fullPath, params.Path)
			if err != nil {
				return err
			}
			result.Fid = fid
			return nil
		}

		parentPath := d.dirOf(fullPath)
		dirName := baseOf(fullPath)

		resolver, ok := drv.(drive.PathResolver)
		if !ok {
			return fmt.Errorf("driver does not support path resolution")
		}
		parentFid, err := resolver.ResolvePath(ctx, parentPath)
		if err != nil {
			return fmt.Errorf("parent directory not found: %w", err)
		}

		entries, err := drv.List(ctx, parentFid)
		if err != nil {
			return err
		}
		encName := cipher.EncryptSegment(dirName)
		for _, e := range entries {
			if e.Name == dirName || e.Name == encName {
				return fmt.Errorf("already exists")
			}
		}

		ne, err := w.Mkdir(ctx, parentFid, encName)
		if err != nil {
			return err
		}
		result.Fid = ne.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Remove deletes a remote file or directory via the daemon.
func (d *Daemon) Remove(ctx context.Context, params protocol.RemoveParams) (*protocol.RemoveResult, error) {
	var result protocol.RemoveResult
	err := d.withMountSession(ctx, params.MountName, params.Password, params.Salt, func(drv drive.Driver, cipher *crypt.RcloneCipher) error {
		mountCfg := config.FindMount(d.cfg, d.resolveMountName(params.MountName))
		rootPath := "/"
		if mountCfg != nil {
			rootPath = config.RootPathForMount(*mountCfg)
		}
		fullPath := config.ResolveFullPath(rootPath, params.Path)

		if fullPath == "/" {
			return fmt.Errorf("cannot remove root")
		}

		parentPath := d.dirOf(fullPath)
		baseName := baseOf(fullPath)

		resolver, ok := drv.(drive.PathResolver)
		if !ok {
			return fmt.Errorf("driver does not support path resolution")
		}
		parentFid, err := resolver.ResolvePath(ctx, parentPath)
		if err != nil {
			if params.Force {
				return nil
			}
			return fmt.Errorf("resolve parent: %w", err)
		}

		entries, err := drv.List(ctx, parentFid)
		if err != nil {
			if params.Force {
				return nil
			}
			return fmt.Errorf("list parent: %w", err)
		}

		encName := cipher.EncryptSegment(baseName)
		var target drive.Entry
		found := false
		for _, e := range entries {
			if e.Name == baseName || e.Name == encName {
				target = e
				found = true
				break
			}
		}
		if !found {
			if params.Force {
				return nil
			}
			return fmt.Errorf("not found")
		}

		if target.IsDir && !params.Recursive {
			return fmt.Errorf("is a directory; use -r for recursive delete")
		}

		w, ok := drv.(drive.Writer)
		if !ok {
			return fmt.Errorf("driver does not support delete")
		}
		if err := w.Remove(ctx, target); err != nil {
			return fmt.Errorf("remove: %w", err)
		}
		result.Status = "removed"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Move moves or renames a remote file/directory via the daemon.
func (d *Daemon) Move(ctx context.Context, params protocol.MoveParams) (*protocol.MoveResult, error) {
	var result protocol.MoveResult
	err := d.withMountSession(ctx, params.MountName, params.Password, params.Salt, func(drv drive.Driver, cipher *crypt.RcloneCipher) error {
		mountCfg := config.FindMount(d.cfg, d.resolveMountName(params.MountName))
		rootPath := "/"
		if mountCfg != nil {
			rootPath = config.RootPathForMount(*mountCfg)
		}

		w, ok := drv.(drive.Writer)
		if !ok {
			return fmt.Errorf("driver does not support move")
		}
		resolver, ok := drv.(drive.PathResolver)
		if !ok {
			return fmt.Errorf("driver does not support path resolution")
		}

		fullSrcPath := config.ResolveFullPath(rootPath, params.SrcPath)
		fullDstPath := config.ResolveFullPath(rootPath, params.DstPath)

		srcFid, err := resolver.ResolvePath(ctx, fullSrcPath)
		if err != nil {
			return fmt.Errorf("resolve source: %w", err)
		}

		moveIntoDir := strings.HasSuffix(params.DstPath, "/")
		dstParentPath := d.dirOf(fullDstPath)
		dstName := baseOf(fullDstPath)

		dstParentFid, err := resolver.ResolvePath(ctx, dstParentPath)
		if err != nil {
			return fmt.Errorf("resolve dest parent: %w", err)
		}

		if !moveIntoDir {
			// Check if dst is an existing directory
			if dstParentFid != "0" {
				entries, _ := drv.List(ctx, dstParentFid)
				for _, e := range entries {
					decName, _ := cipher.DecryptSegment(e.Name)
					if decName == dstName && e.IsDir {
						dstParentFid = e.ID
						moveIntoDir = true
						break
					}
				}
			}
		}

		if moveIntoDir {
			srcParentPath := d.dirOf(fullSrcPath)
			srcParentFid, _ := resolver.ResolvePath(ctx, srcParentPath)
			entries, _ := drv.List(ctx, srcParentFid)
			for _, e := range entries {
				if e.ID == srcFid {
					decName, _ := cipher.DecryptSegment(e.Name)
					dstName = cipher.EncryptSegment(decName)
					break
				}
			}
		} else {
			dstName = cipher.EncryptSegment(dstName)
		}

		// Check no-clobber
		if params.NoClobber {
			entries, _ := drv.List(ctx, dstParentFid)
			for _, e := range entries {
				if e.Name == dstName && e.ID != srcFid {
					return fmt.Errorf("target exists (--no-clobber)")
				}
			}
		}

		srcParentPath := d.dirOf(fullSrcPath)
		srcParentFid, _ := resolver.ResolvePath(ctx, srcParentPath)

		if srcParentFid != dstParentFid {
			moveEntry := drive.Entry{ID: srcFid}
			if err := w.Move(ctx, moveEntry, dstParentFid); err != nil {
				return fmt.Errorf("move: %w", err)
			}
		}

		srcName := baseOf(fullSrcPath)
		srcEncName := cipher.EncryptSegment(srcName)
		if dstName != srcEncName {
			renameEntry := drive.Entry{ID: srcFid}
			if err := w.Rename(ctx, renameEntry, dstName); err != nil {
				return fmt.Errorf("rename: %w", err)
			}
		}

		result.Status = "moved"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// PullStart starts a pull (download) operation in the background.
// Returns a task ID immediately. Progress is published via EventManager.
func (d *Daemon) PullStart(ctx context.Context, params protocol.PullStartParams) (*protocol.PullStartResult, error) {
	mountName := d.resolveMountName(params.MountName)
	if mountName == "" {
		return nil, fmt.Errorf("no enabled mount found")
	}
	mountCfg := config.FindMount(d.cfg, mountName)
	if mountCfg == nil {
		return nil, fmt.Errorf("mount %q not found", mountName)
	}
	rc := d.cfg.MergeInstanceConfig(*mountCfg)

	cipher, err := d.makeCipher(rc, params.Password, params.Salt)
	if err != nil {
		return nil, err
	}

	sk, _ := SessionKeyForMount(rc)
	s, err := d.sessionMgr.Acquire(ctx, sk, rc.Params)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}

	transfers := params.Transfers
	if transfers <= 0 {
		transfers = 4
	}
	taskID := fmt.Sprintf("pull_%d", time.Now().UnixNano())

	go d.runPullTask(ctx, taskID, s, sk, cipher, rc, params, transfers)

	return &protocol.PullStartResult{TaskID: taskID, FileCount: -1}, nil
}

func (d *Daemon) runPullTask(ctx context.Context, taskID string, s *Session, sk SessionKey, cipher *crypt.RcloneCipher, rc *config.ResolvedMountConfig, params protocol.PullStartParams, transfers int) {
	drv := s.Drv
	defer d.sessionMgr.Release(ctx, sk)

	mountCfg := config.FindMount(d.cfg, rc.Name)
	rootPath := "/"
	if mountCfg != nil {
		rootPath = config.RootPathForMount(*mountCfg)
	}
	fullRemotePath := config.ResolveFullPath(rootPath, params.Remote)

	mountName := rc.Name
	parentPath := d.dirOf(fullRemotePath)
	baseName := baseOf(fullRemotePath)

	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, "driver does not support path resolution")
		return
	}

	d.publishPushEvent(taskID, mountName, "started", "", 0, 0, "")

	targetFid, err := resolver.ResolvePath(ctx, fullRemotePath)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("resolve path: %v", err))
		return
	}

	downloadDir := func(fid string, localBase string) {
		if !params.DryRun {
			os.MkdirAll(localBase, 0755)
		}

		var wg sync.WaitGroup
		err := qryptsync.ScanRemoteForDownload(ctx, fid, localBase, drv, cipher, params.DryRun,
			func(entry drive.Entry, targetPath string) {
				wg.Add(1)
				d.orchestrator().Submit(func(ctx context.Context) error {
					defer wg.Done()
					if params.DryRun {
						return nil
					}
					if params.Update {
						if stat, staterr := os.Stat(targetPath); staterr == nil {
							plainSize, _ := cipher.DecryptedSize(entry.Size)
							if stat.Size() == plainSize {
								return nil
							}
						}
					}
					dl := qryptsync.NewDownloader(drv, cipher)
					req := qryptsync.DownloadRequest{
						Entry:     entry,
						LocalPath: targetPath,
					}
					err := dl.Download(ctx, req)
					if err == nil {
						d.publishPushEvent(taskID, mountName, "downloading", targetPath, entry.Size, entry.Size, "")
					}
					return err
				})
			})
		if err != nil {
			d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("scan: %v", err))
			return
		}
		wg.Wait()
	}

	_, listErr := drv.List(ctx, targetFid)
	localPath := params.Local
	if listErr == nil {
		if localPath == "" {
			localPath = baseName
		}
		downloadDir(targetFid, localPath)
		d.publishPushEvent(taskID, mountName, "completed", "", 0, 0, "")
		return
	}

	if localPath == "" {
		localPath = baseName
	}

	parentFid, err := resolver.ResolvePath(ctx, parentPath)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("resolve parent: %v", err))
		return
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("list: %v", err))
		return
	}

	encName := cipher.EncryptSegment(baseName)
	var targetEntry drive.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName {
			targetEntry = e
			found = true
			break
		}
	}
	if !found {
		d.publishPushEvent(taskID, mountName, "failed", "", 0, 0, fmt.Sprintf("file not found: %s", baseName))
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	d.orchestrator().Submit(func(ctx context.Context) error {
		defer wg.Done()
		if params.DryRun {
			return nil
		}
		dl := qryptsync.NewDownloader(drv, cipher)
		return dl.Download(ctx, qryptsync.DownloadRequest{
			Entry:     targetEntry,
			LocalPath: localPath,
		})
	})
	wg.Wait()
	d.publishPushEvent(taskID, mountName, "completed", "", 0, 0, "")
}

// createRemoteDir recursively creates directories (like mkdir -p).
func createRemoteDir(ctx context.Context, drv drive.Driver, w drive.Writer, cipher *crypt.RcloneCipher, fullPath, userPath string) (string, error) {
	currentFid := "0"
	userRel := strings.TrimLeft(userPath, "/")
	segments := strings.Split(userRel, "/")
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		entries, err := drv.List(ctx, currentFid)
		if err != nil {
			return "", fmt.Errorf("list: %w", err)
		}
		encSeg := cipher.EncryptSegment(seg)
		found := false
		for _, e := range entries {
			if e.Name == seg || e.Name == encSeg {
				currentFid = e.ID
				found = true
				if !e.IsDir {
					return "", fmt.Errorf("path conflict: %s is a file", seg)
				}
				break
			}
		}
		if !found {
			ne, err := w.Mkdir(ctx, currentFid, encSeg)
			if err != nil {
				return "", fmt.Errorf("mkdir %s: %w", seg, err)
			}
			currentFid = ne.ID
		}
	}
	return currentFid, nil
}

func (d *Daemon) ClearStaging(ctx context.Context) error {
	for _, m := range d.manager.List() {
		if m.State != protocol.MountStateMounted {
			continue
		}
		inst, _ := d.manager.Get(m.Name)
		if inst == nil || inst.Cache == nil {
			continue
		}
		_, _ = inst.Cache.Staging().CleanupOrphanedStagingFiles(map[string]bool{})
	}
	return nil
}
