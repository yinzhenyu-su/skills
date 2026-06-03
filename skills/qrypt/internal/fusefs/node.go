//go:build !nofuse

package fusefs

import (
	"sync"
	"sync/atomic"
	"time"
)

type Node struct {
	mu sync.RWMutex

	// Identity
	fid       string
	parentFid string
	name      string
	currentPath string
	isFolder  bool

	// File metadata
	size    int64
	encSize int64
	mtime   time.Time

	// Content
	localPath string
	fileNonce [24]byte
	hasNonce  bool

	// Sync state
	isDirty     bool
	syncQueued  bool
	source      string
	expectedFid string
	uploadedFid string

	// Upload resume
	uploadID string
	lastPart int

	// Conflict resolution
	baseServerMtime int64
	baseServerSize  int64
	lastUploadTime  time.Time

	// Cache invalidation
	lastMetadataCheck time.Time

	// Sequential read detection
	lastReadBlock int64
	readSeqCount  int

	// Dirty file persistence throttling
	lastPendingSave time.Time
	lastPendingSize int64

	// Cancellation flag (atomic)
	cancelled int32

	// Edit-protection flags (atomic, used without mu)
	// uploading = 1 when this file is being uploaded.
	// uploadingChildren = N when this directory has N descendants uploading.
	uploading         int32
	uploadingChildren int32

	// Children cache (directories only)
	children map[string]*Node
}

func newNode(fid, parentFid, name, currentPath string, isFolder bool) *Node {
	return &Node{
		fid:         fid,
		parentFid:   parentFid,
		name:        name,
		currentPath: currentPath,
		isFolder:    isFolder,
		mtime:       time.Now(),
		source:      "remote",
	}
}

func (n *Node) Cancel()        { atomic.StoreInt32(&n.cancelled, 1) }
func (n *Node) IsCancelled() bool { return atomic.LoadInt32(&n.cancelled) == 1 }

func (n *Node) isChildrenEmpty() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.children) == 0
}
