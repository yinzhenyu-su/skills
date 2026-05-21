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

	// Push/pull operations
	PushStart(ctx context.Context, params protocol.PushStartParams) (*protocol.PushStartResult, error)
	PullStart(ctx context.Context, params protocol.PullStartParams) (*protocol.PullStartResult, error)

	// Remote file operations
	ListDir(ctx context.Context, params protocol.ListDirParams) (*protocol.ListDirResult, error)
	Mkdir(ctx context.Context, params protocol.MkdirParams) (*protocol.MkdirResult, error)
	Remove(ctx context.Context, params protocol.RemoveParams) (*protocol.RemoveResult, error)
	Move(ctx context.Context, params protocol.MoveParams) (*protocol.MoveResult, error)

	// Events
	SubscribeEvents(ctx context.Context, senderID string) (<-chan *protocol.Event, error)
	UnsubscribeEvents(senderID string)
}
