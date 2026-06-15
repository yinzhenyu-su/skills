package vfs

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// stagingID generates a short unique identifier for local staging files.
// Format: 20 random bytes hex-encoded = 40 characters, fixed-length.
func stagingID() string {
	b := make([]byte, 20)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// VFSNode is the in-memory representation of a filesystem node.
// It contains only tree structure and metadata fields.
// Upload/dirty/cache state is managed by separate components
// (UploadManager, CacheLayer).
type VFSNode struct {
	mu sync.RWMutex

	// Tree structure
	fid         string // staging ID (local UUID, never changes)
	remoteFid   string // remote FID (set after first upload, empty until then)
	isUploaded  bool   // true after at least one successful upload
	parentFid   string
	name        string
	currentPath string
	isFolder    bool
	children    map[string]*VFSNode // only for directories

	// File metadata (plaintext)
	size    int64
	encSize int64
	mtime   time.Time
}

// NewVFSNode creates a new VFSNode with the given identity fields.
// fid is the staging ID (UUID) — use stagingID() to generate one.
func NewVFSNode(fid, parentFid, name, currentPath string, isFolder bool) *VFSNode {
	return &VFSNode{
		fid:         fid,
		parentFid:   parentFid,
		name:        name,
		currentPath: currentPath,
		isFolder:    isFolder,
		mtime:       time.Now(),
	}
}

// FID returns the staging identifier. This is NOT the remote FID —
// use RemoteFID() for that. The staging ID is stable for the node's
// lifetime and is used for coalescer dedup, staging file paths, etc.
func (n *VFSNode) FID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.fid
}

// RemoteFID returns the remote file identifier, or "" if the file has
// not been uploaded yet.
func (n *VFSNode) RemoteFID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.remoteFid
}

// IsUploaded returns true if this node has been uploaded to remote at
// least once and has a valid remote FID.
func (n *VFSNode) IsUploaded() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isUploaded
}

// SetUploaded marks the node as successfully uploaded and records its
// remote FID. The staging ID (n.fid) stays unchanged.
func (n *VFSNode) SetUploaded(remoteFid string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.remoteFid = remoteFid
	n.isUploaded = true
}

// ParentFID returns the parent's backend file identifier.
func (n *VFSNode) ParentFID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.parentFid
}

// Name returns the plaintext filename.
func (n *VFSNode) Name() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.name
}

// CurrentPath returns the node's current path.
func (n *VFSNode) CurrentPath() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.currentPath
}

// IsFolder returns true if the node is a directory.
func (n *VFSNode) IsFolder() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isFolder
}

// Size returns the plaintext file size.
func (n *VFSNode) Size() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.size
}

// EncSize returns the encrypted file size.
func (n *VFSNode) EncSize() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.encSize
}

// SetSize sets the plaintext size.
func (n *VFSNode) SetSize(size int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.size = size
}

// SetEncSize sets the encrypted size.
func (n *VFSNode) SetEncSize(encSize int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.encSize = encSize
}

// ModTime returns the last modification time.
func (n *VFSNode) ModTime() time.Time {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.mtime
}

// SetModTime sets the last modification time.
func (n *VFSNode) SetModTime(t time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.mtime = t
}

// Children returns the child nodes for directories, nil for files.
// Callers must not modify the returned map.
func (n *VFSNode) Children() map[string]*VFSNode {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if !n.isFolder {
		return nil
	}
	return n.children
}

// AddChild adds a child node to this directory node under the given name.
func (n *VFSNode) AddChild(name string, child *VFSNode) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.isFolder {
		return
	}
	if n.children == nil {
		n.children = make(map[string]*VFSNode)
	}
	n.children[name] = child
}

// RemoveChild removes a child node from this directory node.
func (n *VFSNode) RemoveChild(name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.children != nil {
		delete(n.children, name)
	}
}
