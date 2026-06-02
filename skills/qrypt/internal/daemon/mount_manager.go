package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/fs"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func (mm *MountManager) publishMountEvent(name string, state protocol.MountState, errMsg string) {
	if mm.eventMgr == nil {
		return
	}
	data := map[string]interface{}{
		"mount":  name,
		"state":  state,
	}
	if errMsg != "" {
		data["error"] = errMsg
	}
	mm.eventMgr.Publish(&protocol.Event{
		Type:      protocol.EventMountStateChanged,
		Mount:     name,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	})
}

// PendingUploads returns the number of files not yet uploaded for this mount.
func (inst *MountInstance) PendingUploads() int {
	if inst.Cache == nil {
		return 0
	}
	return len(inst.Cache.GetPendingNodes())
}

// IsBusy returns true if the mount has pending uploads.
func (inst *MountInstance) IsBusy() bool {
	return inst.PendingUploads() > 0
}

// MountManager manages the lifecycle of multiple FUSE mount instances.
type MountManager struct {
	mu           sync.RWMutex
	cfg          *config.Config
	sessionMgr   *SessionManager
	mounts       map[string]*MountInstance
	orchestrator *Orchestrator
	eventMgr     *EventManager
}

// SetOrchestrator attaches a shared upload queue to all future mounts.
func (mm *MountManager) SetOrchestrator(o *Orchestrator) {
	mm.orchestrator = o
}

// MountInstance is one running mount with all its resources.
type MountInstance struct {
	Name    string
	State   protocol.MountState
	Driver  drive.Driver
	Cipher  *crypt.RcloneCipher
	Cache   *cache.CacheManager
	Backend mountBackend

	sessionKey  SessionKey // for releasing the session on stop
	ResolvedCfg *config.ResolvedMountConfig
	StartedAt   time.Time
	LastError   string
}

func NewMountManager(cfg *config.Config, sm *SessionManager, em *EventManager) *MountManager {
	if sm == nil {
		sm = NewSessionManager()
	}
	return &MountManager{
		cfg:        cfg,
		sessionMgr: sm,
		mounts:     make(map[string]*MountInstance),
		eventMgr:   em,
	}
}

// List returns a summary of all configured mounts.
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

// Get returns a running mount instance by name.
func (mm *MountManager) Get(name string) (*MountInstance, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	inst, ok := mm.mounts[name]
	if !ok {
		return nil, fmt.Errorf("mount %q not found or not started", name)
	}
	return inst, nil
}

// LookupByPath finds which mount instance owns the given absolute path.
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

// ResolvedConfig returns the resolved config for a mount name.
func (mm *MountManager) ResolvedConfig(name string) (*config.ResolvedMountConfig, bool) {
	for _, m := range mm.cfg.Mounts {
		if m.Name == name {
			return mm.cfg.MergeInstanceConfig(m), true
		}
	}
	return nil, false
}

// Start initialises and mounts one instance.
func (mm *MountManager) Start(ctx context.Context, name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return mm.startLocked(ctx, name)
}

// startLocked is Start without locking (caller must hold mm.mu.Lock()).
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

	cipher, err := config.MakeCipher(rc.Encryption, mm.cfg.Defaults.Encryption, "", "")
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q cipher init: %w", name, err)
	}
	inst.Cipher = cipher

	sk, _ := SessionKeyForMount(rc)
	inst.sessionKey = sk
	s, err := mm.sessionMgr.Acquire(ctx, sk, rc.Params)
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q session: %w", name, err)
	}
	inst.Driver = s.Drv

	if setter, ok := s.Drv.(interface{ SetCipher(*crypt.RcloneCipher) }); ok {
		setter.SetCipher(cipher)
	}

	cacheMaxSize := int64(10 * 1024 * 1024 * 1024)
	if maxSize, err := config.ParseSize(rc.Cache.MaxSize); err == nil {
		cacheMaxSize = maxSize
	}
	cacheMgr, err := cache.NewCacheManager(rc.CacheDir, cacheMaxSize)
	if err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q cache init: %w", name, err)
	}
	inst.Cache = cacheMgr

	backend := newMountBackend()
	if err := backend.mount(ctx, rc, inst.Driver, cipher, cacheMgr); err != nil {
		inst.State = protocol.MountStateError
		inst.LastError = err.Error()
		mm.mounts[name] = inst
		mm.publishMountEvent(name, protocol.MountStateError, err.Error())
		return fmt.Errorf("mount %q fuse: %w", name, err)
	}
	inst.Backend = backend

	// Wire daemon's orchestrator to VFS upload queue (if available).
	if orch := mm.orchestrator; orch != nil {
		if vfs, ok := backend.VFS().(interface{ SetUploadQueue(fs.UploadQueue) }); ok {
			vfs.SetUploadQueue(orch)
		}
	}

	inst.State = protocol.MountStateMounted
	mm.mounts[name] = inst
	mm.publishMountEvent(name, protocol.MountStateMounted, "")
	log.L.Infof("MountManager: mounted %q at %s\n", name, rc.MountPoint)
	return nil
}

// Stop unmounts and tears down one instance.
func (mm *MountManager) Stop(ctx context.Context, name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	inst, ok := mm.mounts[name]
	if !ok {
		return fmt.Errorf("mount %q is not running", name)
	}

	inst.State = protocol.MountStateUnmounting
	mm.publishMountEvent(name, protocol.MountStateUnmounting, "")
	log.L.Infof("MountManager: stopping %q\n", name)

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
	log.L.Infof("MountManager: stopped %q\n", name)
	return nil
}

// StartAll starts all enabled mounts. Failed mounts are recorded but don't block others.
func (mm *MountManager) StartAll(ctx context.Context) error {
	var lastErr error
	for _, m := range mm.cfg.Mounts {
		rc := mm.cfg.MergeInstanceConfig(m)
		if !rc.Enabled {
			continue
		}
		if err := mm.Start(ctx, m.Name); err != nil {
			log.L.Errorf("MountManager: failed to start %q: %v\n", m.Name, err)
			lastErr = err
		}
	}
	return lastErr
}

// Reload stops all mounts, reloads config, and starts enabled mounts.
// Returns error if any mount has pending uploads.
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

	// Stop all running mounts
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
		log.L.Infof("MountManager: stopped %q for reload\n", name)
	}

	// Replace config and mounts map
	mm.cfg = newCfg
	mm.mounts = make(map[string]*MountInstance)

	var started, stopped []string
	for name := range oldNames {
		stopped = append(stopped, name)
	}

	// Start enabled mounts from new config
	for _, m := range newCfg.Mounts {
		rc := newCfg.MergeInstanceConfig(m)
		if !rc.Enabled {
			continue
		}
		// We hold the lock; startLocked would deadlock.
		// Spin up inline instead.
		if err := mm.startLocked(ctx, m.Name); err != nil {
			log.L.Errorf("MountManager: failed to start %q after reload: %v\n", m.Name, err)
		} else {
			started = append(started, m.Name)
		}
	}

	log.L.Infof("MountManager: reload complete: stopped %d, started %d\n", len(stopped), len(started))
	return &protocol.ReloadResult{Status: "reloaded", Started: started, Stopped: stopped}, nil
}

// StopAll stops all running mounts.
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
			log.L.Errorf("MountManager: failed to stop %q: %v\n", name, err)
			lastErr = err
		}
	}
	return lastErr
}

// Shutdown stops all mounts and the shared orchestrator.
func (mm *MountManager) Shutdown(ctx context.Context) error {
	err := mm.StopAll(ctx)
	if mm.orchestrator != nil {
		mm.orchestrator.Shutdown()
	}
	return err
}

// ForEachRunningMount calls fn for each running mount while holding the read lock.
func (mm *MountManager) ForEachRunningMount(fn func(name string, inst *MountInstance)) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	for name, inst := range mm.mounts {
		fn(name, inst)
	}
}

// MountSummary alias for protocol type.
type MountSummary = protocol.MountSummary
