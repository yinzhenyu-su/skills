package fs

import (
	"testing"
)

func TestStoreNode_DirWithChildren(t *testing.T) {
	fs := newTestFS(t)

	dir := newNode("fid_dir", "0", "mydir", "/mydir", true)
	fs.storeNode("/mydir", dir)

	child := newNode("fid_child", "fid_dir", "child.txt", "/mydir/child.txt", false)
	fs.storeNode("/mydir/child.txt", child)

	// Verify parent-child relationship
	if v, ok := fs.nodes.Load("/mydir/child.txt"); !ok {
		t.Error("child node not stored")
	} else if v.(*Node).name != "child.txt" {
		t.Errorf("expected child.txt, got %s", v.(*Node).name)
	}

	// Verify fid index
	if v, ok := fs.fidNodes.Load("fid_child"); !ok {
		t.Error("child fid not indexed")
	} else if v.(*Node).fid != "fid_child" {
		t.Errorf("expected fid_child, got %s", v.(*Node).fid)
	}

	// Verify parent's children map
	dir.mu.RLock()
	if dir.children == nil {
		dir.mu.RUnlock()
		t.Fatal("dir children map is nil")
	}
	childFromParent, exists := dir.children["child.txt"]
	dir.mu.RUnlock()
	if !exists {
		t.Error("child not in parent's children map")
	} else if childFromParent.name != "child.txt" {
		t.Errorf("expected child.txt, got %s", childFromParent.name)
	}
}

func TestStoreNode_RootNotInFidIndex(t *testing.T) {
	fs := newTestFS(t)

	// Root's fid "0" should still be indexed in fs_test's setup
	if _, ok := fs.fidNodes.Load("0"); !ok {
		t.Error("root fid should be indexed")
	}
}

func TestStoreNode_LocalFidNotIndexed(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("local_abc123", "0", "local.txt", "/local.txt", false)
	fs.storeNode("/local.txt", n)

	// local_ prefixed fids should not be in fidNodes
	if _, ok := fs.fidNodes.Load("local_abc123"); ok {
		t.Error("local_ fids should not be indexed in fidNodes")
	}
}

func TestReplaceNodePath_SameParent(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_rn", "0", "old.txt", "/old.txt", false)
	fs.storeNode("/old.txt", n)

	fs.replaceNodePath("/old.txt", "/renamed.txt", n)

	if _, ok := fs.nodes.Load("/old.txt"); ok {
		t.Error("old path should be removed")
	}
	if v, ok := fs.nodes.Load("/renamed.txt"); !ok {
		t.Error("new path should exist")
	} else if v.(*Node).fid != "fid_rn" {
		t.Errorf("expected fid_rn, got %s", v.(*Node).fid)
	}
}

func TestReplaceNodePath_DifferentParent(t *testing.T) {
	fs := newTestFS(t)

	dir1 := newNode("fid_dir1", "0", "dir1", "/dir1", true)
	dir2 := newNode("fid_dir2", "0", "dir2", "/dir2", true)
	fs.storeNode("/dir1", dir1)
	fs.storeNode("/dir2", dir2)

	child := newNode("fid_child", "fid_dir1", "file.txt", "/dir1/file.txt", false)
	fs.storeNode("/dir1/file.txt", child)

	// Move child between parents
	fs.replaceNodePath("/dir1/file.txt", "/dir2/file.txt", child)

	if _, ok := fs.nodes.Load("/dir1/file.txt"); ok {
		t.Error("old path should be removed")
	}
	if _, ok := fs.nodes.Load("/dir2/file.txt"); !ok {
		t.Error("new path should exist")
	}

	// Old parent should no longer have child
	dir1.mu.RLock()
	_, existsInOld := dir1.children["file.txt"]
	dir1.mu.RUnlock()
	if existsInOld {
		t.Error("old parent should not have child after cross-parent move")
	}

	// New parent should have child
	dir2.mu.RLock()
	_, existsInNew := dir2.children["file.txt"]
	dir2.mu.RUnlock()
	if !existsInNew {
		t.Error("new parent should have child after cross-parent move")
	}
}

func TestReplaceNodePath_SamePath(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_sp", "0", "same.txt", "/same.txt", false)
	fs.storeNode("/same.txt", n)

	fs.replaceNodePath("/same.txt", "/same.txt", n)

	if v, ok := fs.nodes.Load("/same.txt"); !ok {
		t.Error("node should still exist")
	} else if v.(*Node).fid != "fid_sp" {
		t.Errorf("expected fid_sp, got %s", v.(*Node).fid)
	}
}

func TestDeleteNodePath_WithChildCleanup(t *testing.T) {
	fs := newTestFS(t)

	dir := newNode("fid_del_dir", "0", "deldir", "/deldir", true)
	child := newNode("fid_del_child", "fid_del_dir", "child.txt", "/deldir/child.txt", false)
	child.isFolder = false

	fs.storeNode("/deldir", dir)
	fs.storeNode("/deldir/child.txt", child)

	fs.deleteNodePath("/deldir/child.txt", child)

	if _, ok := fs.nodes.Load("/deldir/child.txt"); ok {
		t.Error("child should be deleted from nodes")
	}

	// Parent should no longer have child
	dir.mu.RLock()
	_, exists := dir.children["child.txt"]
	dir.mu.RUnlock()
	if exists {
		t.Error("child should be removed from parent's children map")
	}
}

func TestDeleteNodePath_DirItself(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_del_self", "0", "self.txt", "/self.txt", false)
	fs.storeNode("/self.txt", n)

	fs.deleteNodePath("/self.txt", n)

	if _, ok := fs.nodes.Load("/self.txt"); ok {
		t.Error("node should be deleted")
	}
}
