package vfs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newCoalescer creates an OperationCoalescer whose flushFn sends intents to
// the returned channel. Shutdown is registered as a test cleanup.
func newCoalescer(t *testing.T) (*OperationCoalescer, chan []RemoteIntent) {
	t.Helper()
	ch := make(chan []RemoteIntent, 10)
	caps := Capabilities{
		BatchDelete: false,
		Rename:      true,
		Move:        true,
	}
	c := NewOperationCoalescer(caps, func(intents []RemoteIntent) {
		ch <- intents
	})
	t.Cleanup(c.Shutdown)
	return c, ch
}

// makeNode creates a VFSNode with parentFid="parent_fid" and path="/<name>".
func makeNode(fid, name string) *VFSNode {
	return NewVFSNode(fid, "parent_fid", name, "/"+name, false)
}

// makeDirNode creates a directory VFSNode.
func makeDirNode(fid, name string) *VFSNode {
	return NewVFSNode(fid, "parent_fid", name, "/"+name, true)
}

// recvIntents waits up to timeout for flush results and returns them.
// Fatals the test on timeout.
func recvIntents(t *testing.T, ch chan []RemoteIntent, timeout time.Duration) []RemoteIntent {
	t.Helper()
	select {
	case intents := <-ch:
		return intents
	case <-time.After(timeout):
		t.Fatal("timeout waiting for coalescer flush")
		return nil
	}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestCoalescer_SubmitDelete(t *testing.T) {
	c, ch := newCoalescer(t)

	c.Submit(FileOp{
		Kind: OpDelete,
		Path: "/a.txt",
		Fid:  "fid_a",
		Time: time.Now(),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "DELETE", intents[0].Kind)
	assert.Equal(t, "/a.txt", intents[0].Path)
	assert.Equal(t, "fid_a", intents[0].Fid)
}

func TestCoalescer_SubmitRename(t *testing.T) {
	c, ch := newCoalescer(t)

	node := NewVFSNode("staging_fid_a", "parent", "old.txt", "/old.txt", false)
	node.SetUploaded("remote_fid_a")

	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/old.txt",
		NewPath: "/new.txt",
		Fid:     "staging_fid_a",
		Time:    time.Now(),
		Node:    node,
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "RENAME", intents[0].Kind)
	assert.Equal(t, "/old.txt", intents[0].Path)
	assert.Equal(t, "/new.txt", intents[0].NewPath)
}

func TestCoalescer_SubmitMkdir(t *testing.T) {
	c, ch := newCoalescer(t)

	c.Submit(FileOp{
		Kind:  OpMkdir,
		Path:  "/newdir",
		Fid:   "local_dir",
		IsDir: true,
		Time:  time.Now(),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "MKDIR", intents[0].Kind)
	assert.Equal(t, "/newdir", intents[0].Path)
}

func TestCoalescer_SubmitUpload(t *testing.T) {
	c, ch := newCoalescer(t)

	c.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/file.txt",
		Fid:   "local_a",
		Time:  time.Now(),
		Node:  makeNode("local_a", "file.txt"),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "UPLOAD", intents[0].Kind)
	assert.Equal(t, "/file.txt", intents[0].Path)
	assert.Equal(t, "local_a", intents[0].Fid)
}

func TestCoalescer_DeleteReplacedByCreate(t *testing.T) {
	c, ch := newCoalescer(t)

	c.Submit(FileOp{
		Kind: OpDelete,
		Path: "/a.txt",
		Fid:  "remote_a",
		Time: time.Now(),
	})

	c.Submit(FileOp{
		Kind:      OpCreate,
		Path:      "/a.txt",
		Fid:       "local_b",
		ParentFid: "parent_fid",
		Time:      time.Now(),
	})

	c.Submit(FileOp{
		Kind: OpUpload,
		Path: "/a.txt",
		Fid:  "local_b",
		Time: time.Now(),
		Node: makeNode("local_b", "a.txt"),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 2)

	assert.Equal(t, "DELETE", intents[0].Kind)
	assert.Equal(t, "/a.txt", intents[0].Path)
	assert.Equal(t, "remote_a", intents[0].Fid)

	assert.Equal(t, "UPLOAD", intents[1].Kind)
	assert.Equal(t, "/a.txt", intents[1].Path)
	assert.Equal(t, "local_b", intents[1].Fid)
}

func TestCoalescer_DirDeleteAbsorbsChildOps(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	c.Submit(FileOp{
		Kind:  OpDelete,
		Path:  "/dir",
		Fid:   "dir_fid",
		IsDir: true,
		Fids:  []string{"child1_fid", "child2_fid"},
		Time:  time.Now(),
	})

	c.Submit(FileOp{
		Kind: OpDelete,
		Path: "/dir/file.txt",
		Fid:  "child3_fid",
		Time: time.Now(),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "DELETE", intents[0].Kind)
	assert.Equal(t, "/dir", intents[0].Path)
	assert.Equal(t, "dir_fid", intents[0].Fid)
	assert.ElementsMatch(t, []string{"child1_fid", "child2_fid"}, intents[0].Fids)
}

func TestCoalescer_MultipleOpsBatch(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	c.Submit(FileOp{Kind: OpDelete, Path: "/a.txt", Fid: "fid_a", Time: time.Now()})
	c.Submit(FileOp{Kind: OpDelete, Path: "/b.txt", Fid: "fid_b", Time: time.Now()})
	c.Submit(FileOp{Kind: OpDelete, Path: "/c.txt", Fid: "fid_c", Time: time.Now()})

	intents := recvIntents(t, ch, 2*time.Second)

	require.Len(t, intents, 3)
	for _, r := range intents {
		assert.Equal(t, "DELETE", r.Kind)
	}

	assert.Equal(t, "/a.txt", intents[0].Path)
	assert.Equal(t, "fid_a", intents[0].Fid)
	assert.Equal(t, "/b.txt", intents[1].Path)
	assert.Equal(t, "fid_b", intents[1].Fid)
	assert.Equal(t, "/c.txt", intents[2].Path)
	assert.Equal(t, "fid_c", intents[2].Fid)
}

func TestCoalescer_Shutdown(t *testing.T) {
	t.Parallel()

	ch := make(chan []RemoteIntent, 10)
	caps := Capabilities{Rename: true, Move: true}
	c := NewOperationCoalescer(caps, func(intents []RemoteIntent) {
		ch <- intents
	})

	c.Submit(FileOp{
		Kind: OpDelete,
		Path: "/a.txt",
		Fid:  "fid_a",
		Time: time.Now(),
	})

	c.Shutdown()

	select {
	case intents := <-ch:
		require.Len(t, intents, 1)
		assert.Equal(t, "DELETE", intents[0].Kind)
		assert.Equal(t, "/a.txt", intents[0].Path)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: flush was not called during shutdown drain")
	}
}
