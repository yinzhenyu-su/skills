package vfs

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TreeManager manages the VFSNode tree in memory.
// All operations are thread-safe.
type TreeManager struct {
	mu       sync.RWMutex
	nodes    sync.Map // path → *VFSNode
	fidNodes sync.Map // fid → *VFSNode
	rootFid  string
}

// NewTreeManager creates a TreeManager with the given root fid.
func NewTreeManager(rootFid string) *TreeManager {
	return &TreeManager{
		rootFid: rootFid,
	}
}

// Store inserts or updates a node in the tree by path and fid.
// It adds the node to its parent's children map.
func (tm *TreeManager) Store(path string, n *VFSNode) {
	if n == nil {
		return
	}
	path = filepath.Clean(path)

	n.mu.Lock()
	n.currentPath = path
	if n.isFolder && n.children == nil {
		n.children = make(map[string]*VFSNode)
	}
	n.mu.Unlock()

	tm.nodes.Store(path, n)

	// Add to parent's children map
	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := tm.nodes.Load(parentPath); ok {
			p := v.(*VFSNode)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*VFSNode)
			}
			// Don't add self-reference
			if n.fid != "" && n.fid == p.fid {
				p.mu.Unlock()
				return
			}
			p.children[filepath.Base(path)] = n
			p.mtime = time.Now()
			p.mu.Unlock()
		}
	}

	// Store by fid
	n.mu.RLock()
	fid := n.fid
	n.mu.RUnlock()
	if fid != "" && !strings.HasPrefix(fid, "local_") {
		tm.fidNodes.Store(fid, n)
	}
}

// Lookup returns the node for the given path, or nil if not found.
func (tm *TreeManager) Lookup(path string) *VFSNode {
	path = filepath.Clean(path)
	if v, ok := tm.nodes.Load(path); ok {
		return v.(*VFSNode)
	}
	return nil
}

// ReplacePath moves a node from oldPath to newPath in the tree.
func (tm *TreeManager) ReplacePath(oldPath, newPath string, n *VFSNode) {
	if n == nil {
		return
	}
	oldPath = filepath.Clean(oldPath)
	newPath = filepath.Clean(newPath)

	if oldPath != newPath {
		tm.nodes.Delete(oldPath)
		oldParent := filepath.Dir(oldPath)
		newParent := filepath.Dir(newPath)
		if oldParent != newParent {
			// Moving to a different directory: remove from old parent
			if v, ok := tm.nodes.Load(oldParent); ok {
				p := v.(*VFSNode)
				p.mu.Lock()
				if p.children != nil {
					delete(p.children, filepath.Base(oldPath))
				}
				p.mu.Unlock()
			}
		} else if oldPath != "" {
			// Rename within same directory
			if v, ok := tm.nodes.Load(oldParent); ok {
				p := v.(*VFSNode)
				p.mu.Lock()
				if p.children != nil {
					delete(p.children, filepath.Base(oldPath))
					p.children[filepath.Base(newPath)] = n
				}
				p.mu.Unlock()
			}
		}
	}

	// Update the node's currentPath
	n.mu.Lock()
	n.currentPath = newPath
	if n.isFolder && n.children == nil {
		n.children = make(map[string]*VFSNode)
	}
	n.mu.Unlock()

	// Store at new path
	tm.nodes.Store(newPath, n)

	// Add to new parent's children map
	if newPath != "/" {
		parentPath := filepath.Dir(newPath)
		if v, ok := tm.nodes.Load(parentPath); ok {
			p := v.(*VFSNode)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*VFSNode)
			}
			p.children[filepath.Base(newPath)] = n
			p.mu.Unlock()
		}
	}
}

// Delete removes a node from the tree by path and fid.
func (tm *TreeManager) Delete(path string, n *VFSNode) {
	path = filepath.Clean(path)
	tm.nodes.Delete(path)

	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := tm.nodes.Load(parentPath); ok {
			p := v.(*VFSNode)
			p.mu.Lock()
			if p.children != nil {
				delete(p.children, filepath.Base(path))
			}
			p.mtime = time.Now()
			p.mu.Unlock()
		}
	}

	if n != nil {
		n.mu.RLock()
		fid := n.fid
		altPath := n.currentPath
		n.mu.RUnlock()

		if altPath == path {
			n.mu.Lock()
			n.currentPath = ""
			n.mu.Unlock()
		} else if altPath != "" {
			// Node has a stale phantom entry at altPath (e.g. from a
			// buggy Store call). Remove it so it doesn't appear as a
			// file in the wrong parent directory.
			tm.nodes.Delete(altPath)
			if altParent := filepath.Dir(altPath); altParent != "" {
				if v, ok := tm.nodes.Load(altParent); ok {
					p := v.(*VFSNode)
					p.mu.Lock()
					delete(p.children, filepath.Base(altPath))
					p.mu.Unlock()
				}
			}
			n.mu.Lock()
			n.currentPath = ""
			n.mu.Unlock()
		}

		if fid != "" && !strings.HasPrefix(fid, "local_") {
			tm.fidNodes.Delete(fid)
		}
	}
}

// LookupByFid returns the node for the given backend fid, or nil if not found.
func (tm *TreeManager) LookupByFid(fid string) *VFSNode {
	if v, ok := tm.fidNodes.Load(fid); ok {
		return v.(*VFSNode)
	}
	return nil
}

// UpdateFid replaces the old FID with newFid in the fidNodes map.
// oldFid may be empty or "local_..." — those are not stored in fidNodes
// and are silently ignored. This does NOT change the node's tree position.
func (tm *TreeManager) UpdateFid(oldFid, newFid string, n *VFSNode) {
	if oldFid != "" && !strings.HasPrefix(oldFid, "local_") {
		tm.fidNodes.Delete(oldFid)
	}
	if newFid != "" && !strings.HasPrefix(newFid, "local_") {
		tm.fidNodes.Store(newFid, n)
	}
}

// RootFid returns the root directory's fid.
func (tm *TreeManager) RootFid() string {
	return tm.rootFid
}
