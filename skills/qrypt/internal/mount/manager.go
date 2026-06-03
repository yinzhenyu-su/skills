package mount

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/fusefs"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func (mm *MountManager) publishMountEvent(name string, state protocol.MountState, errMsg string) {
	if mm.mountBus == nil {
		return
	}
	data := map[string]interface{}{
		"mount": name,
		"state": state,
	}
	if errMsg != "" {
		data["error"] = errMsg
	}
	mm.mountBus.Publish(&protocol.Event{
		Type:      protocol.EventMountStateChanged,
		Mount:     name,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	})
}

func (inst *MountInstance) PendingUploads() int {
	if inst.Cache == nil {
		return 0
	}
	return len(inst.Cache.GetPendingNodes())
}

func (inst *MountInstance) IsBusy() bool {
	return inst.PendingUploads() > 0
}

type MountManager struct {
	mu            sync.RWMutex
	cfg           *config.Config
	sessionMgr    qrypt.SessionManager
	driverFactory *DriverFactory
	mounts        map[string]*MountInstance
	orchestrator  qrypt.Orchestrator
	mountBus      *MountEventBus
}

func (mm *MountManager) SetOrchestrator(o qrypt.Orchestrator) {
	mm.orchestrator = o
}

func (mm *MountManager) GetOrchestrator() qrypt.Orchestrator {
	return mm.orchestrator
}

type MountInstance struct {
	Name    string
	State   protocol.MountState
	Driver  backend.Driver
	Cipher  *qrypt.RcloneCipher
	Cache   *qrypt.CacheManager
	Backend mountBackend

	sessionKey  qrypt.SessionKey
	ResolvedCfg *config.ResolvedMountConfig
	StartedAt   time.Time
	LastError   string
}

func NewMountManager(cfg *config.Config, sm qrypt.SessionManager, factory *DriverFactory, mb *MountEventBus) *MountManager {
	if factory == nil {
		factory = NewDriverFactory()
	}
	if sm == nil {
		sm = qrypt.NewSessionManager(factory)
	}
	return &MountManager{
		cfg:           cfg,
		sessionMgr:    sm,
		driverFactory: factory,
		mounts:        make(map[string]*MountInstance),
		mountBus:      mb,
	}
}

func (mm *MountManager) List() []MountSummary {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	summaries := make([]MountSummary, 0, len(mm.cfg.Mounts))
	for _, m := range mm.cfg.Mounts {
		rc := mm.cfg.MergeInstanceConfig(m)
		inst, running := mm.mounts[m.Name]
		state := protocol.MountStateUnmounted
		uptime := ""
		lastErr := ""
		if running {
			state = inst.State
			if !inst.StartedAt.IsZero() {
				uptime = time.Since(inst.StartedAt).Round(time.Second).String()
			}
			lastErr = inst.LastError
		}
		summaries = append(summaries, MountSummary{
			Name:       m.Name,
			State:      state,
			MountPoint: rc.MountPoint,
			DriveType:  m.Type,
			Enabled:    rc.Enabled,
			Uptime:     uptime,
			LastError:  lastErr,
		})
	}
	return summaries
}

func (mm *MountManager) Get(name string) (*MountInstance, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	inst, ok := mm.mounts[name]
	if !ok {
		return nil, fmt.Errorf("mount %q not found or not started", name)
	}
	return inst, nil
}

func (mm *MountManager) LookupByPath(abspath string) (*MountInstance, string, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	for _, m := range mm.cfg.Mounts {
		mp := config.ExpandHome(m.MountPoint)
		if len(abspath) < len(mp) || abspath[:len(mp)] != mp {
			continue
		}
		if len(abspath) > len(mp) && abspath[len(mp)] != '/' {
			continue
		}
		inst, ok := mm.mounts[m.Name]
		if !ok {
			continue
		}
		rel := ""
		if len(abspath) > len(mp) {
			rel = abspath[len(mp):]
		}
		return inst, rel, nil
	}
	return nil, "", fmt.Errorf("no mount found for path %q", abspath)
}

func (mm *MountManager) ResolvedConfig(name string) (*config.ResolvedMountConfig, bool) {
	for _, m := range mm.cfg.Mounts {
		if m.Name == name {
			return mm.cfg.MergeInstanceConfig(m), true
		}
	}
	return nil, false
}

func (mm *MountManager) Start(ctx context.Context, name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return mm.startLocked(ctx, name)
}

func (mm *MountManager) startLocked(ctx context.Context, name string) error {
	if _, ok := mm.mounts[name]; ok {
		return fmt.Errorf("mount %q is already running", name)
	}

	rc, ok := mm.ResolvedConfig(name)
	if !ok {
		return fmt.Errorf("mount %q not found in config", name)
	}

	inst := &MountInstance{
		Name:        name,
		State:       protocol.MountStateMounting,
		ResolvedCfg: rc,
		StartedAt:   time.Now(),
	}

	mm.publishMountEvent(name, protocol.MountStateMounting, "")

	rcloneCipher, err := config.MakeCipher(rc.Encryption, mm.cfg.Defaults.Encryption, "", "")
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q cipher init: %w", name, err)
	}
	inst.Cipher = rcloneCipher

	sk := SessionKeyForMount(rc)
	inst.sessionKey = sk
	mm.driverFactory.Register(name, rc.Params)
	sessCfg := SessionConfigForMount(name, rc)
	s, err := mm.sessionMgr.Acquire(ctx, sk, sessCfg)
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q session: %w", name, err)
	}
	inst.Driver = s.Drv

	if setter, ok := inst.Driver.(interface{ SetCipher(*qrypt.RcloneCipher) }); ok {
		setter.SetCipher(rcloneCipher)
	}

	cacheMaxSize := int64(10 * 1024 * 1024 * 1024)
	if maxSize, err := config.ParseSize(rc.Cache.MaxSize); err == nil {
		cacheMaxSize = maxSize
	}
	cacheMgr, err := qrypt.NewCacheManager(rc.CacheDir, cacheMaxSize)
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q cache init: %w", name, err)
	}
	inst.Cache = cacheMgr

	backend := newMountBackend()
	if err := backend.mount(ctx, rc, inst.Driver, rcloneCipher, cacheMgr); err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q fuse: %w", name, err)
	}
	inst.Backend = backend

	if orch := mm.orchestrator; orch != nil {
		if vfs, ok := backend.VFS().(interface{ SetUploadQueue(fusefs.UploadQueue) }); ok {
			vfs.SetUploadQueue(orch)
		}
	}

	inst.State = protocol.MountStateMounted
	mm.mounts[name] = inst
	mm.publishMountEvent(name, protocol.MountStateMounted, "")
	logging.L.Infof("MountManager: mounted %q at %s\n", name, rc.MountPoint)
	return nil
}

func (mm *MountManager) Stop(ctx context.Context, name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	inst, ok := mm.mounts[name]
	if !ok {
		return fmt.Errorf("mount %q is not running", name)
	}

	inst.State = protocol.MountStateUnmounting
	mm.publishMountEvent(name, protocol.MountStateUnmounting, "")
	logging.L.Infof("MountManager: stopping %q\n", name)

	if inst.Backend != nil {
		inst.Backend.unmount()
	}
	if inst.Cache != nil {
		inst.Cache.Close()
	}
	if inst.Driver != nil {
		mm.sessionMgr.Release(ctx, inst.sessionKey)
	}
	if inst.Cipher != nil {
		inst.Cipher = nil
	}

	inst.State = protocol.MountStateUnmounted
	inst.StartedAt = time.Time{}
	delete(mm.mounts, name)
	mm.publishMountEvent(name, protocol.MountStateUnmounted, "")
	logging.L.Infof("MountManager: stopped %q\n", name)
	return nil
}

func (mm *MountManager) StartAll(ctx context.Context) error {
	var lastErr error
	for _, m := range mm.cfg.Mounts {
		rc := mm.cfg.MergeInstanceConfig(m)
		if !rc.Enabled {
			continue
		}
		if err := mm.Start(ctx, m.Name); err != nil {
			logging.L.Errorf("MountManager: failed to start %q: %v\n", m.Name, err)
			lastErr = err
		}
	}
	return lastErr
}

func (mm *MountManager) Reload(ctx context.Context, newCfg *config.Config) (*protocol.ReloadResult, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	var busy []string
	for name, inst := range mm.mounts {
		if n := inst.PendingUploads(); n > 0 {
			busy = append(busy, fmt.Sprintf("%s (%d pending)", name, n))
		}
	}
	if len(busy) > 0 {
		return nil, fmt.Errorf("mount(s) busy, cannot reload: %s", strings.Join(busy, ", "))
	}

	oldNames := make(map[string]bool)
	for name := range mm.mounts {
		oldNames[name] = true
	}

	for name := range mm.mounts {
		inst := mm.mounts[name]
		inst.State = protocol.MountStateUnmounting
		mm.publishMountEvent(name, protocol.MountStateUnmounting, "")
		if inst.Backend != nil {
			inst.Backend.unmount()
		}
		if inst.Cache != nil {
			inst.Cache.Close()
		}
		if inst.Driver != nil {
			mm.sessionMgr.Release(ctx, inst.sessionKey)
		}
		inst.State = protocol.MountStateUnmounted
		mm.publishMountEvent(name, protocol.MountStateUnmounted, "")
		logging.L.Infof("MountManager: stopped %q for reload\n", name)
	}

	mm.cfg = newCfg
	mm.mounts = make(map[string]*MountInstance)

	var started, stopped []string
	for name := range oldNames {
		stopped = append(stopped, name)
	}

	for _, m := range newCfg.Mounts {
		rc := newCfg.MergeInstanceConfig(m)
		if !rc.Enabled {
			continue
		}
		if err := mm.startLocked(ctx, m.Name); err != nil {
			logging.L.Errorf("MountManager: failed to start %q after reload: %v\n", m.Name, err)
		} else {
			started = append(started, m.Name)
		}
	}

	logging.L.Infof("MountManager: reload complete: stopped %d, started %d\n", len(stopped), len(started))
	return &protocol.ReloadResult{Status: "reloaded", Started: started, Stopped: stopped}, nil
}

func (mm *MountManager) StopAll(ctx context.Context) error {
	mm.mu.RLock()
	names := make([]string, 0, len(mm.mounts))
	for name := range mm.mounts {
		names = append(names, name)
	}
	mm.mu.RUnlock()

	var lastErr error
	for _, name := range names {
		if err := mm.Stop(ctx, name); err != nil {
			logging.L.Errorf("MountManager: failed to stop %q: %v\n", name, err)
			lastErr = err
		}
	}
	return lastErr
}

func (mm *MountManager) Shutdown(ctx context.Context) error {
	err := mm.StopAll(ctx)
	if mm.orchestrator != nil {
		mm.orchestrator.Shutdown()
	}
	return err
}

func (mm *MountManager) ForEachRunningMount(fn func(name string, inst *MountInstance)) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	for name, inst := range mm.mounts {
		fn(name, inst)
	}
}

type MountSummary = protocol.MountSummary
