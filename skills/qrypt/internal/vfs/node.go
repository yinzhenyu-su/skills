package vfs

import (
	"sync"
	"time"
)

// VFSNode is the in-memory representation of a filesystem node.
// It contains only tree structure and metadata fields.
// Upload/dirty/cache state is managed by separate components
// (UploadManager, CacheLayer).
type VFSNode struct {
	mu sync.RWMutex

	// Tree structure
	fid         string
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

// FID returns the backend file identifier.
func (n *VFSNode) FID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.fid
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
