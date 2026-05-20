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
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Status() (*protocol.DaemonStatus, error)

	// Config management
	GetConfig() (*config.Config, error)
	InitConfig(ctx context.Context, path string) (string, error)
	ReloadConfig(ctx context.Context) (*protocol.ReloadResult, error)
	ValidateConfig(path string) (*config.ValidationResult, error)

	// Sync
	SyncStatus() (*protocol.SyncStats, error)
	GetSyncTaskList() ([]protocol.SyncTaskInfo, error)

	// Cache
	CacheUsage() (*protocol.CacheUsage, error)
	ClearStaging(ctx context.Context) error

	// Events
	SubscribeEvents(ctx context.Context, senderID string) (<-chan *protocol.Event, error)
	UnsubscribeEvents(senderID string)
}
