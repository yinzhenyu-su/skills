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
	ErrCodeParse          = -32700
	ErrCodeInvalidReq     = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInternal       = -32603
	ErrCodeDaemon         = 1
	ErrCodeMount          = 3
	ErrCodeConfig         = 4
	ErrCodeSync           = 5
	ErrCodeCache          = 6
	ErrCodeBusy           = 7
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

// MountSummary is a serialisable summary of a mount instance.
type MountSummary struct {
	Name       string     `json:"name"`
	State      MountState `json:"state"`
	MountPoint string     `json:"mount_point"`
	DriveType  string     `json:"drive_type"`
	Enabled    bool       `json:"enabled"`
	Uptime     string     `json:"uptime,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

// Daemon status
type DaemonStatus struct {
	Version    string         `json:"version"`
	Uptime     string         `json:"uptime"`
	MountPoint string         `json:"mount_point,omitempty"`
	MountState MountState     `json:"mount_state"`
	DriveType  string         `json:"drive_type,omitempty"`
	LastError  string         `json:"last_error,omitempty"`
	Mounts     []MountSummary `json:"mounts,omitempty"`
}

// ReloadResult is returned after a successful reload_config.
type ReloadResult struct {
	Status   string   `json:"status"`
	Started  []string `json:"started,omitempty"`
	Stopped  []string `json:"stopped,omitempty"`
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

// Event type
type EventType string

const (
	EventMountStateChanged EventType = "mount_state_changed"
	EventSyncProgress      EventType = "sync_progress"
	EventSyncCompleted     EventType = "sync_completed"
	EventSyncFailed        EventType = "sync_failed"
	EventDiskSpaceLow      EventType = "disk_space_low"
	EventDaemonError       EventType = "error"
)

// Event
type Event struct {
	Type      EventType   `json:"type"`
	Mount     string      `json:"mount,omitempty"`
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

// PushStartParams is the request body for push_start RPC.
type PushStartParams struct {
	MountName string `json:"mount_name"`
	Source    string `json:"source"`
	Remote    string `json:"remote"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
	Transfers int    `json:"transfers"`
	Update    bool   `json:"update"`
	DryRun    bool   `json:"dry_run"`
	PlainSize int64  `json:"plain_size"` // -1 for stdin (unknown)
}

// PushStartResult is the response for push_start RPC.
type PushStartResult struct {
	TaskID    string `json:"task_id"`
	FileCount int    `json:"file_count"` // -1 for stdin
}

// PushProgressData is the payload for sync_progress events during push.
type PushProgressData struct {
	TaskID    string  `json:"task_id"`
	Mount     string  `json:"mount"`
	File      string  `json:"file"`
	FileNo    int     `json:"file_no"`
	FileTotal int     `json:"file_total"`
	Bytes     int64   `json:"bytes"`
	Total     int64   `json:"total"`
	Speed     float64 `json:"speed,omitempty"`
	State     string  `json:"state"` // "uploading" | "completed" | "failed"
	Error     string  `json:"error,omitempty"`
}

// ListDirParams is the request body for list_dir RPC.
type ListDirParams struct {
	MountName string `json:"mount_name"`
	Path      string `json:"path"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
}

// ListEntryItem is one entry in ListDirResult.
type ListEntryItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	DecName   string `json:"dec_name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	PlainSize int64  `json:"plain_size"`
	ModTime   int64  `json:"mod_time"`
}

// ListDirResult is the response for list_dir RPC.
type ListDirResult struct {
	Path    string         `json:"path"`
	Entries []ListEntryItem `json:"entries"`
}

// MkdirParams is the request body for mkdir RPC.
type MkdirParams struct {
	MountName string `json:"mount_name"`
	Path      string `json:"path"`
	Parents   bool   `json:"parents"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
}

// MkdirResult is the response for mkdir RPC.
type MkdirResult struct {
	Fid string `json:"fid"`
}

// RemoveParams is the request body for remove RPC.
type RemoveParams struct {
	MountName string `json:"mount_name"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	Force     bool   `json:"force"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
}

// RemoveResult is the response for remove RPC.
type RemoveResult struct {
	Status string `json:"status"`
}

// MoveParams is the request body for move RPC.
type MoveParams struct {
	MountName string `json:"mount_name"`
	SrcPath   string `json:"src_path"`
	DstPath   string `json:"dst_path"`
	NoClobber bool   `json:"no_clobber"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
}

// MoveResult is the response for move RPC.
type MoveResult struct {
	Status string `json:"status"`
}

// PullStartParams is the request body for pull_start RPC.
type PullStartParams struct {
	MountName string `json:"mount_name"`
	Remote    string `json:"remote"`
	Local     string `json:"local"`
	Password  string `json:"password,omitempty"`
	Salt      string `json:"salt,omitempty"`
	Transfers int    `json:"transfers"`
	Update    bool   `json:"update"`
	DryRun    bool   `json:"dry_run"`
}

// PullStartResult is the response for pull_start RPC.
type PullStartResult struct {
	TaskID    string `json:"task_id"`
	FileCount int    `json:"file_count"`
}
