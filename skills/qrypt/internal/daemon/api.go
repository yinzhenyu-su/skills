package daemon

import (
	"context"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// Service is the main daemon API interface.
// All methods are designed to be wrapped by the JSON-RPC layer.
type Service interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() (*protocol.DaemonStatus, error)

	// Mount management
	Mount(ctx context.Context) error
	Unmount(ctx context.Context) error
	MountStatus() (protocol.MountState, error)

	// Config management
	GetConfig() (*config.Config, error)
	InitConfig(ctx context.Context, path string) (string, error)
	UpdateConfig(ctx context.Context, patch protocol.ConfigPatch) error
	ValidateConfig(path string) (*config.ValidationResult, error)
	ExportConfig(path string) error
	ImportConfig(path string) error

	// Auth
	LoginCookie(ctx context.Context, cookie string) error
	LoginQR(ctx context.Context) (qrURL string, expiresAt int64, err error)
	Logout(ctx context.Context) error
	IsLoggedIn() bool
	GetAccountInfo(ctx context.Context) (*protocol.AccountInfo, error)

	// Sync
	SyncStatus() (*protocol.SyncStats, error)
	GetSyncTaskList() ([]protocol.SyncTaskInfo, error)
	PauseSync(ctx context.Context) error
	ResumeSync(ctx context.Context) error

	// Cache
	CacheUsage() (*protocol.CacheUsage, error)
	ClearCache(ctx context.Context) error
	ClearStaging(ctx context.Context) error

	// Events - subscribe for event stream
	SubscribeEvents(ctx context.Context, senderID string) (<-chan *protocol.Event, error)
	UnsubscribeEvents(senderID string)
}
