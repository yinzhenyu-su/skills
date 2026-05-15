package fs

import (
	"sync/atomic"
	"testing"
)

func TestNewNodeBasic(t *testing.T) {
	n := newNode("fid_1", "parent_0", "test.txt", "/test.txt", false)
	if n.fid != "fid_1" {
		t.Errorf("expected fid_1, got %s", n.fid)
	}
	if n.name != "test.txt" {
		t.Errorf("expected test.txt, got %s", n.name)
	}
	if n.currentPath != "/test.txt" {
		t.Errorf("expected /test.txt, got %s", n.currentPath)
	}
	if n.isFolder {
		t.Error("expected isFolder=false")
	}
	if n.source != "remote" {
		t.Errorf("expected source=remote, got %s", n.source)
	}
	if n.mtime.IsZero() {
		t.Error("expected non-zero mtime")
	}
}

func TestNewNode_Directory(t *testing.T) {
	n := newNode("fid_dir", "parent_0", "subdir", "/subdir", true)
	if !n.isFolder {
		t.Error("expected isFolder=true")
	}
}

func TestNodeCancelViaMethod(t *testing.T) {
	n := newNode("fid_c", "p", "c.txt", "/c.txt", false)
	if n.IsCancelled() {
		t.Error("should not be cancelled initially")
	}
	n.Cancel()
	if !n.IsCancelled() {
		t.Error("should be cancelled after Cancel()")
	}
}

func TestNodeCancel_Atomic(t *testing.T) {
	n := newNode("fid_atomic", "p", "a.txt", "/a.txt", false)

	// Verify cancelled uses atomic store/load
	n.Cancel()
	if atomic.LoadInt32(&n.cancelled) != 1 {
		t.Error("cancelled should be 1 after Cancel()")
	}
}

func TestNodeChildrenEmpty_DirNode(t *testing.T) {
	n := newNode("fid_ce", "p", "dir", "/dir", true)
	if !n.isChildrenEmpty() {
		t.Error("new directory node should have empty children")
	}

	n.mu.Lock()
	n.children = map[string]*Node{"child": newNode("fc", "fid_ce", "child", "/dir/child", false)}
	n.mu.Unlock()

	if n.isChildrenEmpty() {
		t.Error("should not be empty after adding child")
	}
}

func TestNodeChildrenEmpty_NonDir(t *testing.T) {
	n := newNode("f", "p", "f.txt", "/f.txt", false)
	// Non-directory nodes have nil children, accessor should handle it
	if !n.isChildrenEmpty() {
		t.Error("nil children should be considered empty")
	}
}

func TestNodeCancel_Twice(t *testing.T) {
	n := newNode("fid_2c", "p", "2c.txt", "/2c.txt", false)
	n.Cancel()
	n.Cancel() // should not panic
	if !n.IsCancelled() {
		t.Error("should remain cancelled")
	}
}

func TestNodeFields(t *testing.T) {
	n := newNode("fid_fields", "parent_fields", "fields.txt", "/fields.txt", false)

	// Set sync state
	n.mu.Lock()
	n.isDirty = true
	n.syncQueued = true
	n.localPath = "/tmp/staging/abcdef"
	n.encSize = 1000
	n.uploadID = "upload_123"
	n.lastPart = 3
	n.mu.Unlock()

	n.mu.RLock()
	if !n.isDirty {
		t.Error("expected isDirty")
	}
	if !n.syncQueued {
		t.Error("expected syncQueued")
	}
	if n.localPath != "/tmp/staging/abcdef" {
		t.Errorf("unexpected localPath: %s", n.localPath)
	}
	if n.encSize != 1000 {
		t.Errorf("unexpected encSize: %d", n.encSize)
	}
	if n.uploadID != "upload_123" {
		t.Errorf("unexpected uploadID: %s", n.uploadID)
	}
	if n.lastPart != 3 {
		t.Errorf("unexpected lastPart: %d", n.lastPart)
	}
	n.mu.RUnlock()
}
