package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *CacheManager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewCacheManager(dir, 100*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewCacheManager(t *testing.T) {
	m := newTestManager(t)
	if m.CacheDir() == "" {
		t.Error("expected non-empty cache dir")
	}
	if m.Staging() == nil {
		t.Error("expected staging store")
	}
}

func TestPendingNodeSaveAndGet(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/test/file.txt", "fid123", "parent1", "file.txt",
		"/staging/path", 100, false, []byte("nonce1234"), 0, 0, "", 0)

	nodes := m.GetPendingNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 pending node, got %d", len(nodes))
	}
	if nodes[0].Path != "/test/file.txt" {
		t.Errorf("expected /test/file.txt, got %s", nodes[0].Path)
	}
	if nodes[0].Fid != "fid123" {
		t.Errorf("expected fid123, got %s", nodes[0].Fid)
	}
}

func TestPendingNodeRemove(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/test/a.txt", "fid_a", "p1", "a.txt", "", 10, false, nil, 0, 0, "", 0)
	m.SavePendingNode("/test/b.txt", "fid_b", "p1", "b.txt", "", 20, false, nil, 0, 0, "", 0)

	nodes := m.GetPendingNodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2, got %d", len(nodes))
	}

	m.RemovePendingNode("/test/b.txt")
	nodes = m.GetPendingNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 after remove, got %d", len(nodes))
	}
}

func TestPendingNodeRemoveByPrefix(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/test/a/1.txt", "f1", "p", "1.txt", "", 1, false, nil, 0, 0, "", 0)
	m.SavePendingNode("/test/a/2.txt", "f2", "p", "2.txt", "", 2, false, nil, 0, 0, "", 0)
	m.SavePendingNode("/test/b/3.txt", "f3", "p", "3.txt", "", 3, false, nil, 0, 0, "", 0)

	m.RemovePendingNodesByPrefix("/test/a")
	nodes := m.GetPendingNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1, got %d", len(nodes))
	}
}

func TestPendingNodeRemoveByFid(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/a.txt", "fid_x", "p", "a.txt", "", 1, false, nil, 0, 0, "", 0)
	m.SavePendingNode("/b.txt", "fid_y", "p", "b.txt", "", 2, false, nil, 0, 0, "", 0)

	m.RemovePendingNodesByFid("fid_x")
	nodes := m.GetPendingNodes()
	if len(nodes) != 1 || nodes[0].Fid == "fid_x" {
		t.Errorf("expected fid_y only")
	}
}

func TestStagingMeta(t *testing.T) {
	m := newTestManager(t)
	m.SaveStagingMeta("fid_s", "/path/to/staging", 42)

	meta := m.GetStagingMeta("fid_s")
	if meta == nil {
		t.Fatal("expected meta")
	}
	if meta.Fid != "fid_s" || meta.Size != 42 {
		t.Errorf("unexpected meta: %+v", meta)
	}

	m.UpdateStagingMeta("fid_s", 99)
	meta = m.GetStagingMeta("fid_s")
	if meta.Size != 99 {
		t.Errorf("expected 99, got %d", meta.Size)
	}

	m.RemoveStagingMeta("fid_s")
	if m.GetStagingMeta("fid_s") != nil {
		t.Error("expected nil after remove")
	}
}

func TestChunkCache(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	chunkPath := filepath.Join(dir, "chunk.dat")
	os.WriteFile(chunkPath, []byte("chunk data"), 0o644)

	m.PutChunk("fid_c", 0, []byte("chunk data"), false)

	data, err := m.GetChunk("fid_c", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected chunk data")
	}

	data, err = m.GetChunk("fid_c", 999)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Error("expected nil for non-existent chunk")
	}
}

func TestChunkHasAndMarkDirty(t *testing.T) {
	m := newTestManager(t)
	m.PutChunk("fid_d", 0, []byte("data"), false)

	ok, err := m.HasChunk("fid_d", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected HasChunk true")
	}

	ok, err = m.HasChunk("fid_d", 999)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected HasChunk false for non-existent")
	}
}

func TestRemoveChunksByFid(t *testing.T) {
	m := newTestManager(t)
	m.PutChunk("fid_r", 0, []byte("a"), false)
	m.PutChunk("fid_r", 1, []byte("b"), false)

	m.RemoveChunksByFid("fid_r")
	ok, _ := m.HasChunk("fid_r", 0)
	if ok {
		t.Error("expected chunk to be removed")
	}
}

func TestOpsLog(t *testing.T) {
	m := newTestManager(t)

	m.AppendOpsLog(&OpsLogEntry{OpType: "DELETE", Path: "/a.txt", Fid: "f1"})
	m.AppendOpsLog(&OpsLogEntry{OpType: "DELETE", Path: "/b.txt", Fid: "f2"})

	entries, err := m.LoadOpsLog()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	m.MarkOpsLogDone("/a.txt")
	m.MarkOpsLogDone("/b.txt")
	entries, _ = m.LoadOpsLog()
	for _, e := range entries {
		if !e.Done {
			t.Errorf("expected %s to be marked done", e.Path)
		}
	}

	m.PurgeOpsLog(0 * time.Nanosecond)
	entries, _ = m.LoadOpsLog()
	if len(entries) != 0 {
		t.Errorf("expected 0 after purge, got %d", len(entries))
	}
}

func TestBatchDeleteNodeState(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/a.txt", "f1", "p", "a.txt", "", 1, false, nil, 0, 0, "", 0)
	m.PutChunk("f1", 0, []byte("data"), false)

	m.BatchDeleteNodeState([]string{"f1"}, []string{"/a.txt"})
	nodes := m.GetPendingNodes()
	if len(nodes) != 0 {
		t.Errorf("expected 0 pending nodes, got %d", len(nodes))
	}
}

func TestEvictIfNeeded(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 10; i++ {
		m.PutChunk("fid_evict", int64(i), []byte("data data data data data"), false)
	}

	if err := m.EvictIfNeeded(0); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenance(t *testing.T) {
	m := newTestManager(t)
	m.AppendOpsLog(&OpsLogEntry{OpType: "DELETE", Path: "/old.txt", Fid: "fold", Done: true})
	m.PutChunk("f_maint", 0, []byte("some data"), false)

	if err := m.Maintenance(); err != nil {
		t.Fatal(err)
	}
}

func TestGetTotalChunkSize(t *testing.T) {
	m := newTestManager(t)
	m.PutChunk("f_sz", 0, []byte("aaaa"), false)

	total := m.getTotalChunkSize()
	if total == 0 {
		t.Error("expected non-zero total chunk size")
	}
}

func TestCleanupStagingMetas(t *testing.T) {
	m := newTestManager(t)
	m.SaveStagingMeta("orphan_fid", "/tmp/staging/file", 100)
	time.Sleep(10 * time.Millisecond)

	m.CleanupStagingMetas(1 * time.Millisecond)
	meta := m.GetStagingMeta("orphan_fid")
	if meta != nil {
		t.Error("expected orphan meta to be cleaned")
	}
}
