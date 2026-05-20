package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// Daemon manages the daemon lifecycle and delegates mount ops to MountManager.
type Daemon struct {
	cfg       *config.Config
	cfgPath   string
	version   string
	eventMgr  *EventManager
	manager   *MountManager

	mu         sync.RWMutex
	startedAt  time.Time
	mountState protocol.MountState
	lastError  string
}

func NewDaemon(cfg *config.Config, version string) *Daemon {
	return &Daemon{
		cfg:      cfg,
		version:  version,
		eventMgr: NewEventManager(),
		manager:  NewMountManager(cfg),
	}
}

func NewDaemonWithPath(cfg *config.Config, cfgPath, version string) *Daemon {
	return &Daemon{
		cfg:     cfg,
		cfgPath: cfgPath,
		version: version,
		eventMgr: NewEventManager(),
		manager: NewMountManager(cfg),
	}
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
	path := d.cfgPath
	if path == "" {
		path = config.FindConfigFile()
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found")
	}

	cfg, vr, err := config.LoadConfig(path)
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
