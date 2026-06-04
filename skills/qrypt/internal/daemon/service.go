package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	factory "github.com/yinzhenyu/skills/qrypt/drivers/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/mount"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)


// Daemon manages the daemon lifecycle and delegates mount ops to MountManager.
type Daemon struct {
	cfg        *config.Config
	cfgPath    string
	version    string
	mountBus   *mount.MountEventBus
	manager    *mount.MountManager
	progress   qrypt.ProgressHub
	syncEvents qrypt.EventBus
	rateLimit  qrypt.RateLimiter
	fileAPI    *qrypt.FileAPI

	mu         sync.RWMutex
	startedAt  time.Time
	mountState protocol.MountState
	lastError  string
}

func NewDaemon(cfg *config.Config, version string) *Daemon {
	mountBus := mount.NewMountEventBus()
	se := qrypt.NewEventBus()
	ph := qrypt.NewProgressHub(se)
	df := mount.NewDriverFactory()
	sm := qrypt.NewSessionManager(df)
	mm := mount.NewMountManager(cfg, sm, df, mountBus)
	rl := qrypt.NewRateLimiter(0)
	orch := qrypt.NewOrchestrator(3, rl, ph)
	mm.SetOrchestrator(orch)
	d := &Daemon{
		cfg:        cfg,
		version:    version,
		mountBus:   mountBus,
		manager:    mm,
		progress:   ph,
		syncEvents: se,
		rateLimit:  rl,
	}
	mount.NewCacheInvalidator(mm, mountBus)
	return d
}

func NewDaemonWithPath(cfg *config.Config, cfgPath, version string) *Daemon {
	mountBus := mount.NewMountEventBus()
	se := qrypt.NewEventBus()
	ph := qrypt.NewProgressHub(se)
	df := mount.NewDriverFactory()
	sm := qrypt.NewSessionManager(df)
	mm := mount.NewMountManager(cfg, sm, df, mountBus)
	rl := qrypt.NewRateLimiter(0)
	orch := qrypt.NewOrchestrator(3, rl, ph)
	mm.SetOrchestrator(orch)
	d := &Daemon{
		cfg:        cfg,
		cfgPath:    cfgPath,
		version:    version,
		mountBus:   mountBus,
		manager:    mm,
		progress:   ph,
		syncEvents: se,
		rateLimit:  rl,
	}
	mount.NewCacheInvalidator(mm, mountBus)
	return d
}

// deriveState computes the aggregate daemon state from individual mount states.
func deriveState(mounts []mount.MountSummary) protocol.MountState {
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

type daemonDirResolver struct{}

func (daemonDirResolver) CacheDir() string  { return config.WorkDir() + "/cache" }
func (daemonDirResolver) DataDir() string   { return config.WorkDir() }
func (daemonDirResolver) ConfigDir() string { return config.WorkDir() }

type daemonCredentialStore struct{}

func (daemonCredentialStore) Get(key string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (daemonCredentialStore) Set(key, value string) error { return nil }
func (daemonCredentialStore) Delete(key string) error     { return nil }

// initFileAPI creates a core/qrypt.FileAPI from the daemon's config.
// Returns nil on failure (daemon can still function without it).
func (d *Daemon) initFileAPI() *qrypt.FileAPI {
	mountCfg := config.FindDefaultMount(d.cfg)
	if mountCfg == nil {
		return nil
	}
	rc := d.cfg.MergeInstanceConfig(*mountCfg)
	ciph, err := config.MakeCipher(rc.Encryption, d.cfg.Defaults.Encryption, "", "")
	if err != nil {
		return nil
	}
	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		return nil
	}
	if err := drv.Init(context.Background()); err != nil {
		return nil
	}
	api, err := qrypt.NewFileAPI(qrypt.Options{
		Cipher:        ciph,
		Dirs:          daemonDirResolver{},
		Creds:         daemonCredentialStore{},
		DriverFactory: qrypt.SingleDriverFactory(drv),
	})
	if err != nil {
		return nil
	}
	return api
}

// ensureFileAPI returns the cached FileAPI, creating it lazily if needed.
func (d *Daemon) ensureFileAPI() *qrypt.FileAPI {
	d.mu.RLock()
	if d.fileAPI != nil {
		d.mu.RUnlock()
		return d.fileAPI
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fileAPI != nil {
		return d.fileAPI
	}
	d.fileAPI = d.initFileAPI()
	return d.fileAPI
}

// SetStartupError records a startup error in the daemon and marks it as errored.
func (d *Daemon) SetStartupError(errMsg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mountState = protocol.MountStateError
	d.lastError = errMsg
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
		ConfigPath: d.cfgPath,
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

func (d *Daemon) orchestrator() qrypt.Orchestrator {
	return d.manager.GetOrchestrator()
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

// ActiveTransfers returns all in-progress uploads and downloads.
func (d *Daemon) ActiveTransfers() *protocol.ActiveTransfersResult {
	entries := d.progress.Active()
	result := &protocol.ActiveTransfersResult{}
	for _, e := range entries {
		pct := 0
		if e.Total > 0 {
			pct = int(e.Bytes * 100 / e.Total)
		}
		result.Transfers = append(result.Transfers, protocol.ActiveTransfer{
			TaskID:    e.TaskID,
			Mount:     e.Mount,
			Direction: e.Direction,
			File:      e.File,
			Bytes:     e.Bytes,
			Total:     e.Total,
			Progress:  pct,
			State:     e.State,
			Error:     e.Error,
			UpdatedAt: e.UpdatedAt.UnixMilli(),
		})
	}
	return result
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

// Dashboard returns aggregated daemon status, sync stats, cache usage, and active transfers.
func (d *Daemon) Dashboard() *protocol.DashboardData {
	status, _ := d.Status()
	stats, _ := d.SyncStatus()
	usage, _ := d.CacheUsage()
	return &protocol.DashboardData{
		Status:          status,
		SyncStats:       stats,
		CacheUsage:      usage,
		ActiveTransfers: d.ActiveTransfers(),
	}
}

func (d *Daemon) MountList() []protocol.MountSummary {
	return d.manager.List()
}

func (d *Daemon) FileAPI() *qrypt.FileAPI {
	return d.ensureFileAPI()
}

func (d *Daemon) SubscribeMountEvents(id string) <-chan *protocol.Event {
	return d.mountBus.Subscribe(id)
}

func (d *Daemon) UnsubscribeMountEvents(id string) {
	d.mountBus.Unsubscribe(id)
}

func (d *Daemon) SubscribeSyncEvents(id string) <-chan *qrypt.Event {
	return d.syncEvents.Subscribe(id)
}

func (d *Daemon) UnsubscribeSyncEvents(id string) {
	d.syncEvents.Unsubscribe(id)
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

func (d *Daemon) makeCipher(rc *config.ResolvedMountConfig, pwd, salt string) (*cipher.RcloneCipher, error) {
	return config.MakeCipher(rc.Encryption, d.cfg.Defaults.Encryption, pwd, salt)
}

func baseOf(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

func (d *Daemon) dirOf(path string) string {
	idx := strings.LastIndex(strings.TrimRight(path, "/"), "/")
	if idx < 0 {
		return "/"
	}
	return path[:idx+1]
}
