package vfs

import (
	"fmt"
	"sync"
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

func TestCoalescer_RenameChain(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	// Submit /a → /b → /c with the same FID.
	node := NewVFSNode("fid_chain", "parent", "a", "/a", false)
	node.SetUploaded("remote_a")

	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/a",
		NewPath: "/b",
		Fid:     "fid_chain",
		Time:    time.Now(),
		Node:    node,
	})
	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/b",
		NewPath: "/c",
		Fid:     "fid_chain",
		Time:    time.Now(),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "RENAME", intents[0].Kind)
	assert.Equal(t, "/a", intents[0].Path)
	assert.Equal(t, "/c", intents[0].NewPath)
	assert.Equal(t, "fid_chain", intents[0].Fid)
}

func TestCoalescer_UploadNotStartedThenRename(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	// Submit upload with no intent to start, then rename the same FID.
	c.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/a",
		Fid:   "fid_upload_rename",
		Time:  time.Now(),
		Node:  makeNode("fid_upload_rename", "a"),
	})
	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/a",
		NewPath: "/b",
		Fid:     "fid_upload_rename",
		Time:    time.Now(),
	})

	// Both ops fall in the same batch → flush sees upload not yet started
	// and collides with rename → should emit UPLOAD at the NEW path.
	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "UPLOAD", intents[0].Kind)
	assert.Equal(t, "/b", intents[0].Path, "upload should be at the renamed path")
	assert.Equal(t, "fid_upload_rename", intents[0].Fid)
}

func TestCoalescer_UploadStartedThenRename_WithRenameCap(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	// Step 1: submit upload and let it flush.
	c.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/a",
		Fid:   "fid_started_rn",
		Time:  time.Now(),
		Node:  makeNode("fid_started_rn", "a"),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "UPLOAD", intents[0].Kind)
	assert.Equal(t, "/a", intents[0].Path)

	time.Sleep(50 * time.Millisecond) // let flush goroutine persist the intent

	// Step 2: mark upload started, then rename.
	c.MarkUploadStarted("fid_started_rn")
	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/a",
		NewPath: "/b",
		Fid:     "fid_started_rn",
		Time:    time.Now(),
	})

	intents2 := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents2, 1)
	assert.Equal(t, "RENAME", intents2[0].Kind)
	assert.Equal(t, "/a", intents2[0].Path)
	assert.Equal(t, "/b", intents2[0].NewPath)
	assert.Equal(t, "fid_started_rn", intents2[0].Fid)
}

func TestCoalescer_UploadStartedThenRename_NoRenameCap(t *testing.T) {
	t.Parallel()

	ch := make(chan []RemoteIntent, 10)
	caps := Capabilities{BatchDelete: false, Rename: false, Move: true}
	c := NewOperationCoalescer(caps, func(intents []RemoteIntent) {
		ch <- intents
	})
	t.Cleanup(c.Shutdown)

	// Step 1: submit upload.
	c.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/a",
		Fid:   "fid_nocap",
		Time:  time.Now(),
		Node:  makeNode("fid_nocap", "a"),
	})

	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "UPLOAD", intents[0].Kind)

	time.Sleep(50 * time.Millisecond)

	// Step 2: mark started and rename.
	c.MarkUploadStarted("fid_nocap")
	c.Submit(FileOp{
		Kind:    OpRename,
		Path:    "/a",
		NewPath: "/b",
		Fid:     "fid_nocap",
		Time:    time.Now(),
	})

	intents2 := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents2, 2)
	assert.Equal(t, "UPLOAD", intents2[0].Kind)
	assert.Equal(t, "/b", intents2[0].Path, "upload at new path when no rename cap")
	assert.Equal(t, "DELETE", intents2[1].Kind)
	assert.Equal(t, "/a", intents2[1].Path, "delete old path when no rename cap")
}

func TestCoalescer_LifecycleAfterFlushIsNoop(t *testing.T) {
	t.Parallel()

	c, ch := newCoalescer(t)

	node := makeNode("fid_lifecycle", "file.txt")
	c.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/file.txt",
		Fid:   "fid_lifecycle",
		Time:  time.Now(),
		Node:  node,
	})

	// Wait for flush — intent is now gone from the map.
	intents := recvIntents(t, ch, 2*time.Second)
	require.Len(t, intents, 1)
	assert.Equal(t, "UPLOAD", intents[0].Kind)

	// Give the flush goroutine time to remove the intent.
	time.Sleep(50 * time.Millisecond)

	// These should be harmless no-ops — they must not panic.
	require.NotPanics(t, func() {
		c.MarkUploadStarted("fid_lifecycle")
		c.MarkUploadCommitted("fid_lifecycle")
	}, "lifecycle calls after flush should not panic")

	// Unknown FIDs should also be harmless.
	require.NotPanics(t, func() {
		c.MarkUploadStarted("nonexistent_fid")
		c.MarkUploadCommitted("nonexistent_fid")
	}, "lifecycle calls for unknown fids should not panic")
}

// ---------------------------------------------------------------------------
// benchmarks
// ---------------------------------------------------------------------------

// runCoalescerBenchmark creates an OperationCoalescer, submits the given
// operations, flushes via Shutdown (which drains and flushes the event
// channel), and returns the flushed intents. Used by coalescer benchmarks.
func runCoalescerBenchmark(b *testing.B, ops []FileOp) []RemoteIntent {
	b.Helper()
	var mu sync.Mutex
	var result []RemoteIntent
	c := NewOperationCoalescer(
		Capabilities{Rename: true, Move: true},
		func(intents []RemoteIntent) {
			mu.Lock()
			result = append(result, intents...)
			mu.Unlock()
		},
	)
	for _, op := range ops {
		c.Submit(op)
	}
	c.Shutdown()
	return result
}

func BenchmarkCoalescerStorm(b *testing.B) {
	b.ReportAllocs()

	const numOps = 100
	ops := make([]FileOp, numOps)
	for i := 0; i < numOps; i++ {
		from := fmt.Sprintf("/f_%d.txt", i)
		to := fmt.Sprintf("/g_%d.txt", i)
		fid := fmt.Sprintf("fid_%d", i)
		node := NewVFSNode(fid, "parent", from[1:], from, false)
		node.SetUploaded("remote_" + fid)
		ops[i] = FileOp{
			Kind:    OpRename,
			Path:    from,
			NewPath: to,
			Fid:     fid,
			Time:    time.Now(),
			Node:    node,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := runCoalescerBenchmark(b, ops)
		_ = result
	}
}

func BenchmarkCoalescerDeleteCreateStorm(b *testing.B) {
	b.ReportAllocs()

	const numCycles = 50
	ops := make([]FileOp, 0, numCycles*3)
	for i := 0; i < numCycles; i++ {
		remoteFid := fmt.Sprintf("remote_%d", i)
		localFid := fmt.Sprintf("local_%d", i)
		ops = append(ops,
			FileOp{
				Kind: OpDelete,
				Path: "/a.txt",
				Fid:  remoteFid,
				Time: time.Now(),
			},
			FileOp{
				Kind:      OpCreate,
				Path:      "/a.txt",
				Fid:       localFid,
				ParentFid: "parent_fid",
				Time:      time.Now(),
			},
			FileOp{
				Kind: OpUpload,
				Path: "/a.txt",
				Fid:  localFid,
				Time: time.Now(),
				Node: NewVFSNode(localFid, "parent_fid", "a.txt", "/a.txt", false),
			},
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := runCoalescerBenchmark(b, ops)
		_ = result
	}
}
