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
	if !n.isChildrenEmpty() {
		t.Error("nil children should be considered empty")
	}
}

func TestNodeCancel_Twice(t *testing.T) {
	n := newNode("fid_2c", "p", "2c.txt", "/2c.txt", false)
	n.Cancel()
	n.Cancel()
	if !n.IsCancelled() {
		t.Error("should remain cancelled")
	}
}

func TestNodeFields(t *testing.T) {
	n := newNode("fid_fields", "parent_fields", "fields.txt", "/fields.txt", false)
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

func TestNodeUploadingChildren_Default(t *testing.T) {
	n := newNode("fid_uc1", "p", "dir", "/dir", true)
	if atomic.LoadInt32(&n.uploadingChildren) != 0 {
		t.Error("uploadingChildren should be 0 initially")
	}
}

func TestNodeUploadingChildren_IncrementDecrement(t *testing.T) {
	n := newNode("fid_uc2", "p", "dir", "/dir", true)
	atomic.AddInt32(&n.uploadingChildren, 1)
	if atomic.LoadInt32(&n.uploadingChildren) != 1 {
		t.Error("uploadingChildren should be 1 after increment")
	}
	atomic.AddInt32(&n.uploadingChildren, 2)
	if atomic.LoadInt32(&n.uploadingChildren) != 3 {
		t.Errorf("uploadingChildren should be 3, got %d", atomic.LoadInt32(&n.uploadingChildren))
	}
	atomic.AddInt32(&n.uploadingChildren, -3)
	if atomic.LoadInt32(&n.uploadingChildren) != 0 {
		t.Error("uploadingChildren should be 0 after decrement")
	}
}

func TestNodeUploadingChildren_MultipleChildren(t *testing.T) {
	parent := newNode("fid_par", "0", "parent", "/parent", true)
	c1 := newNode("fid_c1", "fid_par", "c1.txt", "/parent/c1.txt", false)
	c2 := newNode("fid_c2", "fid_par", "c2.txt", "/parent/c2.txt", false)
	atomic.AddInt32(&parent.uploadingChildren, 1)
	atomic.AddInt32(&parent.uploadingChildren, 1)
	if atomic.LoadInt32(&parent.uploadingChildren) != 2 {
		t.Errorf("two children uploading should give 2, got %d", atomic.LoadInt32(&parent.uploadingChildren))
	}
	_ = c1
	_ = c2
}
