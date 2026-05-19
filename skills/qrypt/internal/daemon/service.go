package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	factory "github.com/yinzhenyu/skills/qrypt/internal/drive/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// Daemon is the core daemon instance.
type Daemon struct {
	cfg      *config.Config
	cipher   *crypt.RcloneCipher
	cacheMgr *cache.CacheManager
	mount    mountBackend

	mu         sync.RWMutex
	startedAt  time.Time
	mountState protocol.MountState
	lastError  string

	eventMgr *EventManager

	// drv is the driver instance
	drv interface {
		Init(ctx context.Context) error
	}

	version string
}

// NewDaemon creates a new daemon instance.
func NewDaemon(cfg *config.Config, version string) *Daemon {
	return &Daemon{
		cfg:      cfg,
		version:  version,
		eventMgr: NewEventManager(),
		mount:    newMountBackend(),
	}
}

// Status returns the current dameon status.
func (d *Daemon) Status() (*protocol.DaemonStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state := d.mountState
	if state == "" {
		state = protocol.MountStateUnmounted
	}

	uptime := "not started"
	if !d.startedAt.IsZero() {
		uptime = time.Since(d.startedAt).Round(time.Second).String()
	}

	return &protocol.DaemonStatus{
		Version:    d.version,
		Uptime:     uptime,
		MountPoint: d.cfg.Mount.Point,
		MountState: state,
		DriveType:  d.cfg.Drive.Type,
		LastError:  d.lastError,
	}, nil
}

// Start initializes and mounts the filesystem.
func (d *Daemon) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.mountState == protocol.MountStateMounted || d.mountState == protocol.MountStateMounting {
		return fmt.Errorf("daemon is already running (state: %s)", d.mountState)
	}

	d.mountState = protocol.MountStateMounting
	d.startedAt = time.Now()

	// Cipher
	cipher, err := crypt.NewRcloneCipher(d.cfg.Encryption.Password, d.cfg.Encryption.Salt)
	if err != nil {
		d.mountState = protocol.MountStateError
		d.lastError = err.Error()
		return fmt.Errorf("cipher init failed: %w", err)
	}
	d.cipher = cipher

	// Driver
	drv, err := factory.NewDriverFromConfig(d.cfg.Drive)
	if err != nil {
		d.mountState = protocol.MountStateError
		d.lastError = err.Error()
		return fmt.Errorf("driver init failed: %w", err)
	}
	if err := drv.Init(ctx); err != nil {
		d.mountState = protocol.MountStateError
		d.lastError = err.Error()
		return fmt.Errorf("driver auth failed: %w", err)
	}
	d.drv = drv

	// Cache manager
	cacheMaxSize := int64(10 * 1024 * 1024 * 1024)
	if maxSize, err := config.ParseSize(d.cfg.Cache.MaxSize); err == nil {
		cacheMaxSize = maxSize
	}
	cacheMgr, err := cache.NewCacheManager(d.cfg.Cache.Dir, cacheMaxSize)
	if err != nil {
		d.mountState = protocol.MountStateError
		d.lastError = err.Error()
		return fmt.Errorf("cache init failed: %w", err)
	}
	d.cacheMgr = cacheMgr

	// Mount via backend (FUSE or noop)
	if err := d.mount.mount(ctx, d.cfg, drv, d.cipher, d.cacheMgr); err != nil {
		d.mountState = protocol.MountStateError
		d.lastError = err.Error()
		return fmt.Errorf("mount failed: %w", err)
	}

	d.mountState = protocol.MountStateMounted
	d.lastError = ""
	log.L.Infof("Daemon: mounted at %s\n", d.cfg.Mount.Point)
	d.eventMgr.Publish(&protocol.Event{
		Type:      protocol.EventMountStateChanged,
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]string{"state": "mounted"},
	})

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.mountState != protocol.MountStateMounted {
		return nil
	}

	d.mountState = protocol.MountStateUnmounting

	log.L.Infof("Daemon: shutting down...\n")

		// Unmount via backend
		d.mount.unmount()

		// Close cache

	// Close cache
	if d.cacheMgr != nil {
		d.cacheMgr.Close()
	}

	d.mountState = protocol.MountStateUnmounted
	d.lastError = ""
	log.L.Infof("Daemon: shutdown complete\n")
	d.eventMgr.Publish(&protocol.Event{
		Type:      protocol.EventMountStateChanged,
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]string{"state": "unmounted"},
	})

	return nil
}

// MountStatus returns the current mount state.
func (d *Daemon) MountStatus() (protocol.MountState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.mountState, nil
}

// IsLoggedIn checks if the driver is authenticated.
func (d *Daemon) IsLoggedIn() bool {
	return d.drv != nil
}

// GetConfig returns the current config.
func (d *Daemon) GetConfig() (*config.Config, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg, nil
}

// InitConfig generates a default config file at the given path.
// If path is empty, defaults to $QRYPT_WORK_DIR/qrypt.toml.
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

// UpdateConfig patches the config.
func (d *Daemon) UpdateConfig(ctx context.Context, patch protocol.ConfigPatch) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if patch.Cookie != nil {
		if d.cfg.Drive.Quark != nil {
			d.cfg.Drive.Quark.Cookie = *patch.Cookie
		}
	}
	if patch.Password != nil {
		d.cfg.Encryption.Password = *patch.Password
	}
	if patch.Salt != nil {
		d.cfg.Encryption.Salt = *patch.Salt
	}
	if patch.RootPath != nil {
		if d.cfg.Drive.Quark != nil {
			d.cfg.Drive.Quark.RootPath = *patch.RootPath
		}
	}
	if patch.MountPoint != nil {
		d.cfg.Mount.Point = config.ExpandHome(*patch.MountPoint)
	}
	if patch.LogLevel != nil {
		d.cfg.Log.Level = *patch.LogLevel
	}
	return nil
}

// ValidateConfig validates a config file.
// If path is empty, validates the currently loaded config.
func (d *Daemon) ValidateConfig(path string) (*config.ValidationResult, error) {
	if path != "" {
		return config.ValidateConfigFile(path), nil
	}
	d.mu.RLock()
	cfg := d.cfg
	d.mu.RUnlock()
	return config.ValidateConfig(cfg), nil
}

// ExportConfig writes config to a file.
func (d *Daemon) ExportConfig(path string) error {
	return fmt.Errorf("export not yet implemented")
}

// ImportConfig reads config from a file.
func (d *Daemon) ImportConfig(path string) error {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
	return nil
}

// LoginCookie sets the cookie and reinitializes the driver.
func (d *Daemon) LoginCookie(ctx context.Context, cookie string) error {
	return d.UpdateConfig(ctx, protocol.ConfigPatch{Cookie: &cookie})
}

// LoginQR returns a QR login URL (placeholder — requires Quark API support).
func (d *Daemon) LoginQR(ctx context.Context) (string, int64, error) {
	return "", 0, fmt.Errorf("QR login not yet supported")
}

// Logout clears auth state.
func (d *Daemon) Logout(ctx context.Context) error {
	empty := ""
	return d.UpdateConfig(ctx, protocol.ConfigPatch{Cookie: &empty})
}

// GetAccountInfo returns account info (placeholder — requires API support).
func (d *Daemon) GetAccountInfo(ctx context.Context) (*protocol.AccountInfo, error) {
	return &protocol.AccountInfo{
		Username:   "unknown",
		UsedSpace:  0,
		TotalSpace: 0,
		FileCount:  0,
	}, nil
}

// SyncStatus returns current sync statistics.
func (d *Daemon) SyncStatus() (*protocol.SyncStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.mountState != protocol.MountStateMounted || d.cacheMgr == nil {
		return &protocol.SyncStats{}, nil
	}

	// Estimate from pending nodes
	pendingNodes := d.cacheMgr.GetPendingNodes()
	pending := len(pendingNodes)
	var pendingBytes int64
	for _, n := range pendingNodes {
		pendingBytes += n.Size
	}

	return &protocol.SyncStats{
		PendingUploads:    pending,
		ActiveUploads:     0,
		CompletedUploads:  0,
		FailedUploads:     0,
		TotalBytesSync:    0,
		TotalBytesPending: pendingBytes,
		InProgress:        pending > 0,
		LastSyncTime:      "",
	}, nil
}

// GetSyncTaskList returns a list of active sync tasks.
func (d *Daemon) GetSyncTaskList() ([]protocol.SyncTaskInfo, error) {
	if d.cacheMgr == nil {
		return nil, nil
	}
	nodes := d.cacheMgr.GetPendingNodes()
	tasks := make([]protocol.SyncTaskInfo, 0, len(nodes))
	for _, n := range nodes {
		tasks = append(tasks, protocol.SyncTaskInfo{
			Fid:   n.Fid,
			Name:  n.Name,
			Size:  n.Size,
			State: string(protocol.EventSyncProgress),
		})
	}
	return tasks, nil
}

// PauseSync pauses sync (placeholder — QryptFS controls workers internally).
func (d *Daemon) PauseSync(ctx context.Context) error {
	return fmt.Errorf("pause not yet supported")
}

// ResumeSync resumes sync.
func (d *Daemon) ResumeSync(ctx context.Context) error {
	return nil
}

// CacheUsage returns disk cache usage.
func (d *Daemon) CacheUsage() (*protocol.CacheUsage, error) {
	if d.cacheMgr == nil {
		return &protocol.CacheUsage{}, nil
	}

	stagingFiles, _ := d.cacheMgr.Staging().ListStagingFiles()
	stagingSize := int64(len(stagingFiles))

	return &protocol.CacheUsage{
		TotalSize:    0,
		StagingSize:  stagingSize,
		ChunkSize:    0,
		MaxSize:      0,
		DirtyCount:   0,
		StagingCount: len(stagingFiles),
	}, nil
}

// ClearCache clears the chunk cache.
func (d *Daemon) ClearCache(ctx context.Context) error {
	return fmt.Errorf("clear cache not yet supported")
}

// ClearStaging clears abandoned staging files.
func (d *Daemon) ClearStaging(ctx context.Context) error {
	if d.cacheMgr == nil {
		return nil
	}
	activeFids := map[string]bool{}
	_, err := d.cacheMgr.Staging().CleanupOrphanedStagingFiles(activeFids)
	return err
}
