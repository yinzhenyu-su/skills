// Package transfer provides cross-mount file copy capabilities.
// Note: Full implementation requires the file nonce from QryptFS metadata,
// which is only available when both mounts are running. For now, this is a
// placeholder — use `qrypt pull` / `qrypt push` with manual staging, or
// mount both drives and use OS-level cp between mount points.
package transfer

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

// Manager handles file transfers between mount instances.
type Manager struct {
	mountMgr *daemon.MountManager
}

func NewManager(mountMgr *daemon.MountManager) *Manager {
	return &Manager{mountMgr: mountMgr}
}

// Copy copies a file from src mount/path to dst mount/path.
// Currently requires both mounts to be running.
func (m *Manager) Copy(ctx context.Context, srcMount, srcPath, dstMount, dstPath string) error {
	_, err := m.mountMgr.Get(srcMount)
	if err != nil {
		return fmt.Errorf("src mount %q not running: %w", srcMount, err)
	}
	_, err = m.mountMgr.Get(dstMount)
	if err != nil {
		return fmt.Errorf("dst mount %q not running: %w", dstMount, err)
	}
	return fmt.Errorf("cross-mount copy not yet implemented — use pull+push or OS-level cp between mounted paths")
}
