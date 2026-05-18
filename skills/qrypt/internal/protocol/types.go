package protocol

// JSON-RPC request
type Request struct {
	ID     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// JSON-RPC response
type Response struct {
	ID     int64       `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *ErrorObj   `json:"error,omitempty"`
}

type ErrorObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard error codes
const (
	ErrCodeParse     = -32700
	ErrCodeInvalidReq = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInternal  = -32603
	ErrCodeDaemon    = 1
	ErrCodeAuth      = 2
	ErrCodeMount     = 3
	ErrCodeConfig    = 4
	ErrCodeSync      = 5
	ErrCodeCache     = 6
)

// Mount state
type MountState string

const (
	MountStateUnmounted  MountState = "unmounted"
	MountStateMounting   MountState = "mounting"
	MountStateMounted    MountState = "mounted"
	MountStateUnmounting MountState = "unmounting"
	MountStateError      MountState = "error"
)

// Daemon status
type DaemonStatus struct {
	Version    string     `json:"version"`
	Uptime     string     `json:"uptime"`
	MountPoint string     `json:"mount_point,omitempty"`
	MountState MountState `json:"mount_state"`
	DriveType  string     `json:"drive_type,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

// Sync stats
type SyncStats struct {
	PendingUploads    int     `json:"pending_uploads"`
	ActiveUploads     int     `json:"active_uploads"`
	CompletedUploads  int64   `json:"completed_uploads"`
	FailedUploads     int64   `json:"failed_uploads"`
	TotalBytesSync    int64   `json:"total_bytes_sync"`
	TotalBytesPending int64   `json:"total_bytes_pending"`
	InProgress        bool    `json:"in_progress"`
	LastSyncTime      string  `json:"last_sync_time,omitempty"`
}

// Sync task info
type SyncTaskInfo struct {
	Fid       string  `json:"fid"`
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	State     string  `json:"state"`
	Progress  float64 `json:"progress"`
	Error     string  `json:"error,omitempty"`
	StartedAt string  `json:"started_at,omitempty"`
}

// Cache usage
type CacheUsage struct {
	TotalSize    int64 `json:"total_size"`
	StagingSize  int64 `json:"staging_size"`
	ChunkSize    int64 `json:"chunk_size"`
	MaxSize      int64 `json:"max_size"`
	DirtyCount   int   `json:"dirty_count"`
	StagingCount int   `json:"staging_count"`
}

// Config patch for partial updates
type ConfigPatch struct {
	Cookie    *string `json:"cookie,omitempty"`
	Password  *string `json:"password,omitempty"`
	Salt      *string `json:"salt,omitempty"`
	RootPath  *string `json:"root_path,omitempty"`
	MountPoint *string `json:"mount_point,omitempty"`
	LogLevel  *string `json:"log_level,omitempty"`
}

// Account info
type AccountInfo struct {
	Username   string `json:"username"`
	UsedSpace  int64  `json:"used_space"`
	TotalSpace int64  `json:"total_space"`
	FileCount  int64  `json:"file_count"`
}

// Event type
type EventType string

const (
	EventMountStateChanged EventType = "mount_state_changed"
	EventSyncProgress      EventType = "sync_progress"
	EventSyncCompleted     EventType = "sync_completed"
	EventSyncFailed        EventType = "sync_failed"
	EventDiskSpaceLow      EventType = "disk_space_low"
	EventAuthExpired       EventType = "auth_expired"
	EventDaemonError       EventType = "error"
)

// Event
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// File entry (for ListDirectory)
type FileEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}
