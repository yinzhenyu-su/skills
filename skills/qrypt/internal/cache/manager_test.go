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

func TestPendingJournal_DirtyThenLoad(t *testing.T) {
	m := newTestManager(t)
	stgPath := filepath.Join(m.StagingDir(), "stub.staging")
	os.WriteFile(stgPath, []byte("data"), 0o644)
	m.SavePendingNode("/a.txt", "fid_a", "p", "a.txt", stgPath, 100, false, nil, 0, 0, "", 0)

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := recovered["/a.txt"]
	if !ok {
		t.Fatal("expected /a.txt to be recovered")
	}
	if pn.Fid != "fid_a" || pn.Size != 100 {
		t.Errorf("unexpected pending node: fid=%s size=%d", pn.Fid, pn.Size)
	}
}

func TestPendingJournal_DirtyThenClean(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/b.txt", "fid_b", "p", "b.txt", "", 50, false, nil, 0, 0, "", 0)
	m.RemovePendingNode("/b.txt")

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := recovered["/b.txt"]; ok {
		t.Error("expected /b.txt to NOT be recovered after clean")
	}
}

func TestPendingJournal_DirtyThenUpdate(t *testing.T) {
	m := newTestManager(t)
	m.SavePendingNode("/c.txt", "fid_c", "p", "c.txt", "", 10, false, nil, 0, 0, "", 0)
	m.UpdatePendingNodeUpload("/c.txt", "up_42")
	m.UpdatePendingNodeLastPart("/c.txt", 7)

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := recovered["/c.txt"]
	if !ok {
		t.Fatal("expected /c.txt to be recovered")
	}
	if pn.UploadID != "up_42" {
		t.Errorf("expected upload_id up_42, got %s", pn.UploadID)
	}
	if pn.LastPart != 7 {
		t.Errorf("expected last_part 7, got %d", pn.LastPart)
	}
}

func TestPendingJournal_RecoveryMissingStagingFile(t *testing.T) {
	m := newTestManager(t)
	stagingFile := filepath.Join(m.CacheDir(), "staging", "missing.staging")
	m.SavePendingNode("/missing.txt", "fid_m", "p", "missing.txt", stagingFile, 100, false, nil, 0, 0, "", 0)

	// Don't create the staging file → recovery should drop this entry.
	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := recovered["/missing.txt"]; ok {
		t.Error("expected /missing.txt to be dropped when staging file is missing")
	}
}

func TestPendingJournal_CrossSessionPersistence(t *testing.T) {
	dir := t.TempDir()

	// Session 1: create and save a pending node
	{
		m1, err := NewCacheManager(dir, 100*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		m1.SavePendingNode("/persist.txt", "fid_p", "0", "persist.txt",
			filepath.Join(dir, "staging", "persist.staging"), 200, false, nil, 0, 0, "", 0)
		// Create the staging file so recovery succeeds
		os.WriteFile(filepath.Join(dir, "staging", "persist.staging"), []byte("data"), 0o644)
		_ = m1
	}

	// Session 2: new CacheManager should recover the pending node from journal
	{
		m2, err := NewCacheManager(dir, 100*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		nodes := m2.GetPendingNodes()
		if len(nodes) != 1 {
			t.Fatalf("expected 1 recovered pending node, got %d", len(nodes))
		}
		if nodes[0].Path != "/persist.txt" {
			t.Errorf("expected /persist.txt, got %s", nodes[0].Path)
		}
		if nodes[0].Size != 200 {
			t.Errorf("expected size 200, got %d", nodes[0].Size)
		}
	}
}

func TestPendingJournal_CrossSessionCleanAfterUpload(t *testing.T) {
	dir := t.TempDir()

	// Session 1: create, then remove (simulates completed upload)
	{
		m1, err := NewCacheManager(dir, 100*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		m1.SavePendingNode("/done.txt", "fid_d", "0", "done.txt",
			filepath.Join(dir, "staging", "done.staging"), 50, false, nil, 0, 0, "", 0)
		m1.RemovePendingNode("/done.txt")
		_ = m1
	}

	// Session 2: should have no pending nodes
	{
		m2, err := NewCacheManager(dir, 100*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		nodes := m2.GetPendingNodes()
		if len(nodes) != 0 {
			t.Errorf("expected 0 pending nodes after clean, got %d", len(nodes))
		}
	}
}

func TestPendingJournal_Compact(t *testing.T) {
	m := newTestManager(t)

	m.SavePendingNode("/compact_a.txt", "fid_ca", "0", "compact_a.txt", "", 10, false, nil, 0, 0, "", 0)
	m.SavePendingNode("/compact_b.txt", "fid_cb", "0", "compact_b.txt", "", 20, false, nil, 0, 0, "", 0)
	m.RemovePendingNode("/compact_b.txt")

	// Before compact: 4 entries (2 dirty + 2 clean)
	// After compact: 1 entry (only /compact_a.txt)
	if err := m.compactPendingJournal(); err != nil {
		t.Fatal(err)
	}

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 {
		t.Errorf("expected 1 entry after compact, got %d", len(recovered))
	}
	if _, ok := recovered["/compact_b.txt"]; ok {
		t.Error("/compact_b.txt should not exist after clean")
	}
}

func TestPendingJournal_CompactWithUpdates(t *testing.T) {
	m := newTestManager(t)

	m.SavePendingNode("/upd.txt", "fid_up", "0", "upd.txt", "", 10, false, nil, 0, 0, "", 0)
	m.UpdatePendingNodeUpload("/upd.txt", "upload_99")
	m.UpdatePendingNodeLastPart("/upd.txt", 5)

	if err := m.compactPendingJournal(); err != nil {
		t.Fatal(err)
	}

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := recovered["/upd.txt"]
	if !ok {
		t.Fatal("expected /upd.txt after compact")
	}
	if pn.UploadID != "upload_99" {
		t.Errorf("expected upload_99 after compact, got %s", pn.UploadID)
	}
	if pn.LastPart != 5 {
		t.Errorf("expected last_part 5 after compact, got %d", pn.LastPart)
	}
}

func TestPendingJournal_RemoveThenSaveSamePath(t *testing.T) {
	m := newTestManager(t)

	m.SavePendingNode("/flip.txt", "fid_a", "0", "flip.txt", "", 1, false, nil, 0, 0, "", 0)
	m.RemovePendingNode("/flip.txt")
	m.SavePendingNode("/flip.txt", "fid_b", "0", "flip.txt", "", 2, false, nil, 0, 0, "", 0)

	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := recovered["/flip.txt"]
	if !ok {
		t.Fatal("expected /flip.txt after remove+save")
	}
	if pn.Fid != "fid_b" {
		t.Errorf("expected fid_b (latest), got %s", pn.Fid)
	}
	if pn.Size != 2 {
		t.Errorf("expected size 2, got %d", pn.Size)
	}
}

func TestBatchDeleteNodeState_NoJournalClean(t *testing.T) {
	m := newTestManager(t)

	m.SavePendingNode("/batch.txt", "fid_batch", "0", "batch.txt", "", 1, false, nil, 0, 0, "", 0)
	m.BatchDeleteNodeState([]string{"fid_batch"}, []string{"/batch.txt"})

	// BatchDeleteNodeState removes from memory but does NOT write journal clean.
	// The journal still has the dirty entry.
	recovered, err := m.LoadPendingJournal()
	if err != nil {
		t.Fatal(err)
	}
	// Without a cross-check (staging file existence), the entry would be recovered.
	// In production, LoadPendingJournal's os.Stat cross-check drops it when staging is gone.
	pn, ok := recovered["/batch.txt"]
	if !ok {
		t.Error("expected /batch.txt to remain in journal (batch ops skip clean append)")
	}
	_ = pn
}
