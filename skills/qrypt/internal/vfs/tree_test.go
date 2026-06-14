package vfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTree creates a fresh TreeManager with rootFid "0" and root node at "/".
func setupTree(t *testing.T) *TreeManager {
	t.Helper()
	tm := NewTreeManager("0")
	root := NewVFSNode("0", "", "", "/", true)
	tm.Store("/", root)
	return tm
}

func TestTreeManager_StoreAndLookup(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	node := NewVFSNode("f1", "0", "test.txt", "/test.txt", false)
	tm.Store("/test.txt", node)

	// Lookup by path
	got := tm.Lookup("/test.txt")
	assert.NotNil(t, got)
	assert.Equal(t, "f1", got.FID())
	assert.Equal(t, "test.txt", got.Name())
	assert.False(t, got.IsFolder())

	// Root lookup works
	root := tm.Lookup("/")
	assert.NotNil(t, root)
	assert.True(t, root.IsFolder())
}

func TestTreeManager_LookupByFid(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	node := NewVFSNode("f1", "0", "test.txt", "/test.txt", false)
	tm.Store("/test.txt", node)

	// Find by fid
	got := tm.LookupByFid("f1")
	assert.NotNil(t, got)
	assert.Equal(t, "/test.txt", got.CurrentPath())

	// Nonexistent fid
	assert.Nil(t, tm.LookupByFid("nonexistent"))
}

func TestTreeManager_LocalFidNotInFidNodes(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	node := NewVFSNode("local_abc123", "0", "local.txt", "/local.txt", false)
	tm.Store("/local.txt", node)

	// local_ fids are not indexed
	assert.Nil(t, tm.LookupByFid("local_abc123"))

	// Path-based lookup still works
	got := tm.Lookup("/local.txt")
	assert.NotNil(t, got)
	assert.Equal(t, "local_abc123", got.FID())
}

func TestTreeManager_ReplacePath_SameDir(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	node := NewVFSNode("f1", "0", "old.txt", "/old.txt", false)
	tm.Store("/old.txt", node)
	tm.ReplacePath("/old.txt", "/new.txt", node)

	// Old path gone
	assert.Nil(t, tm.Lookup("/old.txt"))

	// New path exists with updated currentPath
	got := tm.Lookup("/new.txt")
	assert.NotNil(t, got)
	assert.Equal(t, "/new.txt", got.CurrentPath())
}

func TestTreeManager_ReplacePath_CrossDir(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	dir := NewVFSNode("dir1", "0", "dir", "/dir", true)
	tm.Store("/dir", dir)

	file := NewVFSNode("f1", "0", "old.txt", "/old.txt", false)
	tm.Store("/old.txt", file)
	tm.ReplacePath("/old.txt", "/dir/old.txt", file)

	// Old path gone
	assert.Nil(t, tm.Lookup("/old.txt"))

	// New path exists
	got := tm.Lookup("/dir/old.txt")
	assert.NotNil(t, got)
	assert.Equal(t, "/dir/old.txt", got.CurrentPath())
}

func TestTreeManager_Delete(t *testing.T) {
	t.Parallel()

	tm := setupTree(t)
	node := NewVFSNode("f1", "0", "test.txt", "/test.txt", false)
	tm.Store("/test.txt", node)
	tm.Delete("/test.txt", node)

	// Path lookup returns nil
	assert.Nil(t, tm.Lookup("/test.txt"))

	// Fid lookup returns nil
	assert.Nil(t, tm.LookupByFid("f1"))
}
