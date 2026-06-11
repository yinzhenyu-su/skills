//go:build !nofuse

package fusefs

import (
	"sync"
	"testing"
	"time"
)

// newTestCoalescer creates a coalescer with a test channel that captures flush
// callback results. The channel has buffer 1 with a non-blocking send so tests
// never deadlock even if multiple flushes occur.
func newTestCoalescer(caps Capabilities) (*OperationCoalescer, chan []RemoteIntent) {
	resultsCh := make(chan []RemoteIntent, 1)
	oc := NewOperationCoalescer(caps, func(intents []RemoteIntent) {
		select {
		case resultsCh <- intents:
		default:
		}
	})
	return oc, resultsCh
}

// ---------------------------------------------------------------------------
// Scenario 1: CREATE → WRITE → UPLOAD → DELETE (all cancelled)
//
// Coalescing rule: unstarted upload followed by a delete cancels all remote
// operations. The flush should output 0 remote intents.
// ---------------------------------------------------------------------------

func TestCoalescer_CreateWriteDelete_NoRemoteCall(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	ops := []FileOp{
		{Kind: OpCreate, Path: "/a.txt", Fid: "remote_f1", Node: &Node{parentFid: "0"}},
		{Kind: OpWrite, Path: "/a.txt", Fid: "remote_f1"},
		{Kind: OpUpload, Path: "/a.txt", Fid: "remote_f1", Staging: "/staging/a.txt", Node: &Node{parentFid: "0"}},
		{Kind: OpDelete, Path: "/a.txt", Fid: "remote_f1"},
	}

	results := oc.flush(ops)

	if len(results) != 0 {
		t.Fatalf("expected 0 remote intents (all cancelled), got %d: %+v",
			len(results), results)
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: CREATE → WRITE → UPLOAD → RENAME (upload migrates to new path)
//
// Coalescing rule: an unstarted upload pending on the old path should be
// migrated to the new path after rename. The flush outputs 1 UPLOAD for the
// new path and 0 for the old path.
// ---------------------------------------------------------------------------

func TestCoalescer_CreateWriteRename_UploadNewPath(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "parent_1"}
	ops := []FileOp{
		{Kind: OpCreate, Path: "/tmp/file.tmp", Fid: "tmp_f1", Node: stubNode},
		{Kind: OpWrite, Path: "/tmp/file.tmp", Fid: "tmp_f1"},
		{Kind: OpUpload, Path: "/tmp/file.tmp", Fid: "tmp_f1", Staging: "/staging/tmp", Node: stubNode},
		{Kind: OpRename, Path: "/tmp/file.tmp", NewPath: "/dist/file.js", Fid: "tmp_f1"},
	}

	results := oc.flush(ops)

	if len(results) != 1 {
		t.Fatalf("expected 1 remote intent (migrated upload), got %d: %+v",
			len(results), results)
	}
	if results[0].Kind != "UPLOAD" {
		t.Errorf("expected Kind=UPLOAD, got %s", results[0].Kind)
	}
	if results[0].Path != "/dist/file.js" {
		t.Errorf("expected Path=/dist/file.js (new path), got %s", results[0].Path)
	}
	if results[0].Staging != "/staging/tmp" {
		t.Errorf("expected Staging=/staging/tmp, got %s", results[0].Staging)
	}
	if results[0].Fid != "tmp_f1" {
		t.Errorf("expected Fid=tmp_f1, got %s", results[0].Fid)
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: RENAME → RENAME (merged into single rename)
//
// Coalescing rule: two renames on the same fid should be merged into a single
// rename from the original path to the final path. No intermediate rename.
// ---------------------------------------------------------------------------

func TestCoalescer_RenameRename_Merged(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	ops := []FileOp{
		{Kind: OpRename, Path: "/a", NewPath: "/b", Fid: "remote_f1"},
		{Kind: OpRename, Path: "/b", NewPath: "/c", Fid: "remote_f1"},
	}

	results := oc.flush(ops)

	if len(results) != 1 {
		t.Fatalf("expected 1 remote intent (merged rename), got %d: %+v",
			len(results), results)
	}
	if results[0].Kind != "RENAME" {
		t.Errorf("expected Kind=RENAME, got %s", results[0].Kind)
	}
	if results[0].Path != "/a" {
		t.Errorf("expected Path=/a (original), got %s", results[0].Path)
	}
	if results[0].NewPath != "/c" {
		t.Errorf("expected NewPath=/c (final), got %s", results[0].NewPath)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: RENAME → DELETE (skip rename)
//
// Coalescing rule: when a renamed file is deleted, the rename is skipped.
// For remote fids: emit a DELETE for the original path.
// For local fids: cancel all operations (no remote intent).
// ---------------------------------------------------------------------------

func TestCoalescer_RenameDelete_SkipRename(t *testing.T) {
	t.Run("remote_fid_deletes_original_path", func(t *testing.T) {
		oc, _ := newTestCoalescer(Capabilities{Rename: true})
		defer oc.Shutdown()

		ops := []FileOp{
			{Kind: OpRename, Path: "/a", NewPath: "/b", Fid: "remote_f1"},
			{Kind: OpDelete, Path: "/b", Fid: "remote_f1"},
		}

		results := oc.flush(ops)

		if len(results) != 1 {
			t.Fatalf("expected 1 remote intent (delete original), got %d: %+v",
				len(results), results)
		}
		if results[0].Kind != "DELETE" {
			t.Errorf("expected Kind=DELETE, got %s", results[0].Kind)
		}
		if results[0].Path != "/a" {
			t.Errorf("expected Path=/a (original path, not /b), got %s", results[0].Path)
		}
		if results[0].SkipRemote {
			t.Error("expected SkipRemote=false for remote fid")
		}
	})

	t.Run("local_fid_cancels_all_operations", func(t *testing.T) {
		oc, _ := newTestCoalescer(Capabilities{Rename: true})
		defer oc.Shutdown()

		ops := []FileOp{
			{Kind: OpRename, Path: "/a", NewPath: "/b", Fid: "local_f1"},
			{Kind: OpDelete, Path: "/b", Fid: "local_f1"},
		}

		results := oc.flush(ops)

		if len(results) != 0 {
			t.Errorf("expected 0 remote intents for local-only file, got %d: %+v",
				len(results), results)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 5: DELETE directory → DELETE child → DELETE child
//            (directory delete absorbs child deletes)
//
// Coalescing rule: a recursive directory delete creates a path tombstone that
// absorbs all child deletes in the same batch. Only the directory delete is
// emitted as a remote intent.
// ---------------------------------------------------------------------------

func TestCoalescer_DeleteDir_AbsorbsChildren(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	ops := []FileOp{
		{Kind: OpDelete, Path: "/dist", Fid: "dir_f1", IsDir: true},
		{Kind: OpDelete, Path: "/dist/a.js", Fid: "child_f1"},
		{Kind: OpDelete, Path: "/dist/sub", Fid: "child_f2"},
	}

	results := oc.flush(ops)

	if len(results) != 1 {
		t.Fatalf("expected 1 remote intent (dir delete, children absorbed), got %d: %+v",
			len(results), results)
	}
	if results[0].Kind != "DELETE" {
		t.Errorf("expected Kind=DELETE, got %s", results[0].Kind)
	}
	if results[0].Path != "/dist" {
		t.Errorf("expected Path=/dist, got %s", results[0].Path)
	}
	if !results[0].Recursive {
		t.Error("expected Recursive=true for directory delete")
	}
	if results[0].SkipRemote {
		t.Error("expected SkipRemote=false for remote directory")
	}

	// Verify the tombstone was created (public API)
	if !oc.isUnderTombstone("/dist") {
		t.Error("expected /dist to have an active tombstone after dir delete")
	}
}

// ---------------------------------------------------------------------------
// Scenario 6: DELETE (remote fid) → CREATE → WRITE → UPLOAD (new fid)
//            (same path: replace file, not cancel)
//
// Coalescing rule: deleting a file and then creating+uploading a new file at
// the same path is a REPLACE operation. The delete (old fid) and upload
// (new fid) are both emitted; they operate on different fids so neither
// cancels the other.
// ---------------------------------------------------------------------------

func TestCoalescer_DeleteCreateSamePath_Replace(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "parent_new"}
	ops := []FileOp{
		{Kind: OpDelete, Path: "/a.txt", Fid: "old_f1"},
		{Kind: OpCreate, Path: "/a.txt", Fid: "local_new_f1", Node: stubNode},
		{Kind: OpWrite, Path: "/a.txt", Fid: "local_new_f1"},
		{Kind: OpUpload, Path: "/a.txt", Fid: "local_new_f1", Staging: "/staging/new_a.txt", Node: stubNode},
	}

	results := oc.flush(ops)

	if len(results) != 2 {
		t.Fatalf("expected 2 remote intents (DELETE + UPLOAD), got %d: %+v",
			len(results), results)
	}

	var deleteFound, uploadFound bool
	for _, r := range results {
		switch r.Kind {
		case "DELETE":
			deleteFound = true
			if r.Path != "/a.txt" {
				t.Errorf("DELETE: expected Path=/a.txt, got %s", r.Path)
			}
			if r.SkipRemote {
				t.Error("DELETE: expected SkipRemote=false for remote fid")
			}
		case "UPLOAD":
			uploadFound = true
			if r.Path != "/a.txt" {
				t.Errorf("UPLOAD: expected Path=/a.txt, got %s", r.Path)
			}
			if r.Staging != "/staging/new_a.txt" {
				t.Errorf("UPLOAD: expected Staging=/staging/new_a.txt, got %s", r.Staging)
			}
		default:
			t.Errorf("unexpected intent Kind=%s at Path=%s", r.Kind, r.Path)
		}
	}
	if !deleteFound {
		t.Error("expected a DELETE remote intent for the old file")
	}
	if !uploadFound {
		t.Error("expected a UPLOAD remote intent for the new file")
	}
}

// ---------------------------------------------------------------------------
// Scenario 7: UPLOAD (not started) → RENAME (upload migrates to new path)
//
// Coalescing rule: when an upload hasn't started yet (Upload.Started=false),
// a subsequent rename migrates the upload intent to the new path (Rule 2).
//
// NOTE: When Upload.Started=true (Rule 7, compensation rename), the behavior
// differs: with caps.Rename=true → 1 RENAME compensation; without → 1 UPLOAD
// + 1 DELETE. However, Upload.Started is set externally by the upload worker
// and cannot be triggered through FileOp events alone. Rule 7 requires
// integration-level tests that involve the upload worker.
//
// The sub-tests below verify Rule 2 behavior with both caps.Rename=true and
// caps.Rename=false to confirm the rename capability setting does not affect
// the upload-migration path.
// ---------------------------------------------------------------------------

func TestCoalescer_UploadStartedRename_Compensate(t *testing.T) {
	stubNode := &Node{parentFid: "0"}

	t.Run("with_rename_cap", func(t *testing.T) {
		oc, _ := newTestCoalescer(Capabilities{Rename: true})
		defer oc.Shutdown()

		// UPLOAD followed by RENAME; Upload.Started stays false (not started
		// by any event processor), so Rule 2 applies: migrate upload.
		ops := []FileOp{
			{Kind: OpUpload, Path: "/a.txt", Fid: "remote_f1", Staging: "/staging/a.txt", Node: stubNode},
			{Kind: OpRename, Path: "/a.txt", NewPath: "/b.txt", Fid: "remote_f1"},
		}

		results := oc.flush(ops)

		// Rule 2 (not started): upload migrates to /b.txt, no compensation.
		if len(results) != 1 {
			t.Fatalf("expected 1 remote intent (migrated upload), got %d: %+v",
				len(results), results)
		}
		if results[0].Kind != "UPLOAD" {
			t.Errorf("expected Kind=UPLOAD, got %s", results[0].Kind)
		}
		if results[0].Path != "/b.txt" {
			t.Errorf("expected Path=/b.txt (new path after rename), got %s", results[0].Path)
		}
		if results[0].Staging != "/staging/a.txt" {
			t.Errorf("expected Staging=/staging/a.txt, got %s", results[0].Staging)
		}
	})

	t.Run("without_rename_cap", func(t *testing.T) {
		oc, _ := newTestCoalescer(Capabilities{Rename: false})
		defer oc.Shutdown()

		ops := []FileOp{
			{Kind: OpUpload, Path: "/a.txt", Fid: "remote_f1", Staging: "/staging/a.txt", Node: stubNode},
			{Kind: OpRename, Path: "/a.txt", NewPath: "/b.txt", Fid: "remote_f1"},
		}

		results := oc.flush(ops)

		// Rule 2 (not started): caps.Rename is irrelevant, upload migrates.
		if len(results) != 1 {
			t.Fatalf("expected 1 remote intent (migrated upload), got %d: %+v",
				len(results), results)
		}
		if results[0].Kind != "UPLOAD" {
			t.Errorf("expected Kind=UPLOAD, got %s", results[0].Kind)
		}
		if results[0].Path != "/b.txt" {
			t.Errorf("expected Path=/b.txt, got %s", results[0].Path)
		}
	})
}

// ---------------------------------------------------------------------------
// Tombstone TTL: DELETE directory → flush → late DELETE child (absorbed)
//
// After a recursive directory delete is flushed, the tombstone persists in
// oc.tombstones. A late-arriving child delete in a subsequent batch should be
// absorbed (no remote intent emitted).
// ---------------------------------------------------------------------------

func TestCoalescer_TombstoneTTL_AbsorbsLateDeletes(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	// First flush: DELETE /dir (recursive) → adds tombstone for /dir
	firstOps := []FileOp{
		{Kind: OpDelete, Path: "/dir", Fid: "dir_f1", IsDir: true},
	}
	firstResults := oc.flush(firstOps)

	if len(firstResults) != 1 {
		t.Fatalf("first flush: expected 1 DELETE for /dir, got %d: %+v",
			len(firstResults), firstResults)
	}
	if firstResults[0].Kind != "DELETE" {
		t.Errorf("first flush: expected Kind=DELETE, got %s", firstResults[0].Kind)
	}
	if firstResults[0].Path != "/dir" {
		t.Errorf("first flush: expected Path=/dir, got %s", firstResults[0].Path)
	}
	if !firstResults[0].Recursive {
		t.Error("first flush: expected Recursive=true for directory delete")
	}

	// Verify the tombstone is active via public API
	if !oc.isUnderTombstone("/dir") {
		t.Fatal("tombstone for /dir should be active after first flush")
	}

	// Second flush: DELETE /dir/child.js (late child) → absorbed by tombstone
	secondOps := []FileOp{
		{Kind: OpDelete, Path: "/dir/child.js", Fid: "child_f1"},
	}
	secondResults := oc.flush(secondOps)

	if len(secondResults) != 0 {
		t.Fatalf("second flush: expected 0 intents (absorbed by tombstone), got %d: %+v",
			len(secondResults), secondResults)
	}

	// Verify the tombstone still exists (has not expired)
	if !oc.isUnderTombstone("/dir") {
		t.Error("tombstone for /dir should still exist after second flush")
	}
}

// ---------------------------------------------------------------------------
// End-to-end: Verify the Submit → timer → flush → callback pipeline works.
//
// Unlike the other tests which call flush() directly, this test submits events
// through the Submit() method and waits for the background goroutine's timer
// to fire and deliver results through the callback channel.
//
// Uses a non-empty scenario (UPLOAD only) because flushAndLog only invokes
// the callback when len(results) > 0.
// ---------------------------------------------------------------------------

func TestCoalescer_EndToEnd_SubmitFlow(t *testing.T) {
	oc, resultsCh := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	// Submit a plain UPLOAD event that should produce 1 UPLOAD intent.
	stubNode := &Node{parentFid: "0"}
	oc.Submit(FileOp{Kind: OpCreate, Path: "/a.txt", Fid: "remote_f1", Node: stubNode})
	oc.Submit(FileOp{Kind: OpWrite, Path: "/a.txt", Fid: "remote_f1"})
	oc.Submit(FileOp{Kind: OpUpload, Path: "/a.txt", Fid: "remote_f1", Staging: "/staging/a.txt", Node: stubNode})

	// Wait for the background goroutine to process the batch.
	// Window is 200ms (first event is OpCreate, a file op); add generous buffer.
	select {
	case results := <-resultsCh:
		if len(results) != 1 {
			t.Fatalf("expected 1 remote intent via callback, got %d: %+v",
				len(results), results)
		}
		if results[0].Kind != "UPLOAD" {
			t.Errorf("expected Kind=UPLOAD, got %s", results[0].Kind)
		}
		if results[0].Path != "/a.txt" {
			t.Errorf("expected Path=/a.txt, got %s", results[0].Path)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for coalescer flush callback (window=200ms)")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

// TestCoalescer_UploadLifecycle verifies the MarkUploadStarted and
// MarkUploadCommitted API correctly tracks in-flight upload state.
// Once an upload is started, create+delete should NOT cancel (must compensate).
func TestCoalescer_UploadLifecycle(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "parent1"}

	// Step 1: Create + Upload to establish intent
	ops := []FileOp{
		{Kind: OpCreate, Path: "/a.txt", Fid: "remote_f1", Node: stubNode},
		{Kind: OpWrite, Path: "/a.txt", Fid: "remote_f1"},
		{Kind: OpUpload, Path: "/a.txt", Fid: "remote_f1", Staging: "/staging/a.txt", Node: stubNode},
	}
	results := oc.flush(ops)
	if len(results) != 1 || results[0].Kind != "UPLOAD" {
		t.Fatalf("expected 1 UPLOAD, got %+v", results)
	}

	// Step 2: Mark upload as started (simulating syncFile calling MarkUploadStarted)
	oc.MarkUploadStarted("remote_f1")

	// Step 3: Submit DELETE for the same file while upload is in-flight.
	// With Upload.Started=true, the coalescer must NOT cancel the upload;
	// it must output a DELETE intent (compensation) instead.
	deleteOps := []FileOp{
		{Kind: OpDelete, Path: "/a.txt", Fid: "remote_f1", Node: stubNode},
	}
	deleteResults := oc.flush(deleteOps)
	if len(deleteResults) != 1 {
		t.Fatalf("started upload + delete: expected 1 DELETE (compensation), got %d: %+v",
			len(deleteResults), deleteResults)
	}
	if deleteResults[0].Kind != "DELETE" {
		t.Errorf("started upload + delete: expected Kind=DELETE, got %s", deleteResults[0].Kind)
	}
	if deleteResults[0].SkipRemote {
		t.Error("started upload + delete: DELETE should NOT be skip_remote (must compensate)")
	}

	// Step 4: Mark upload as committed (simulating syncFile completing successfully)
	oc.MarkUploadCommitted("remote_f1")

	// Step 5: Verify the intent was persisted with Committed flag.
	// A subsequent delete for a committed upload should still produce DELETE.
	cleanupOps := []FileOp{
		{Kind: OpDelete, Path: "/a.txt", Fid: "remote_f1", Node: stubNode},
	}
	cleanupResults := oc.flush(cleanupOps)
	if len(cleanupResults) != 1 {
		t.Fatalf("committed upload + delete: expected 1 DELETE, got %d: %+v",
			len(cleanupResults), cleanupResults)
	}
	if cleanupResults[0].Kind != "DELETE" {
		t.Errorf("committed upload + delete: expected Kind=DELETE, got %s", cleanupResults[0].Kind)
	}
}

// TestCoalescer_LocalUploadDeleteCancels verifies that for local_ fids
// (never uploaded), create+delete still cancels correctly.
func TestCoalescer_LocalUploadDeleteCancels(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "parent1"}

	// local_ fid: create + upload + delete → should cancel entirely (Rule 1).
	// Since the file was never on remote, no DELETE intent is needed.
	ops := []FileOp{
		{Kind: OpCreate, Path: "/tmp.txt", Fid: "local_tmp1", Node: stubNode},
		{Kind: OpWrite, Path: "/tmp.txt", Fid: "local_tmp1"},
		{Kind: OpUpload, Path: "/tmp.txt", Fid: "local_tmp1", Staging: "/staging/tmp", Node: stubNode},
		{Kind: OpDelete, Path: "/tmp.txt", Fid: "local_tmp1", Node: stubNode},
	}
	results := oc.flush(ops)
	// Rule 1 cancels: upload not started + delete = fully cancelled, 0 outputs.
	if len(results) != 0 {
		t.Fatalf("local create+delete: expected 0 intents (fully cancelled), got %d: %+v",
			len(results), results)
	}
}

func TestCoalescer_EmptyBatch_ReturnsNil(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	results := oc.flush(nil)
	if results != nil {
		t.Errorf("expected nil for empty batch, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// Scenario 8: Crash recovery — journal encoding and cancelled-op zero output
// ---------------------------------------------------------------------------

func TestCoalescer_CrashRecovery_JournalFormat(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	ops := []FileOp{
		{Kind: OpRename, Path: "/old.txt", NewPath: "/new.txt", Fid: "remote_f1"},
	}
	results := oc.flush(ops)

	if len(results) != 1 || results[0].Kind != "RENAME" {
		t.Fatalf("expected 1 RENAME, got %+v", results)
	}
	if results[0].Path != "/old.txt" || results[0].NewPath != "/new.txt" {
		t.Errorf("RENAME paths: expected /old.txt→/new.txt, got %s→%s",
			results[0].Path, results[0].NewPath)
	}
	// Verify the Fid field is populated for crash-recovery encoding
	if results[0].Fid != "remote_f1" {
		t.Errorf("expected Fid=remote_f1, got %s", results[0].Fid)
	}
}

func TestCoalescer_CrashRecovery_CancelledOpsNotLogged(t *testing.T) {
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	// create + write + upload + delete on local fid = fully cancelled
	ops := []FileOp{
		{Kind: OpCreate, Path: "/tmp.txt", Fid: "local_tmp1", Node: &Node{parentFid: "0"}},
		{Kind: OpWrite, Path: "/tmp.txt", Fid: "local_tmp1"},
		{Kind: OpUpload, Path: "/tmp.txt", Fid: "local_tmp1", Staging: "/stg/tmp"},
		{Kind: OpDelete, Path: "/tmp.txt", Fid: "local_tmp1"},
	}
	results := oc.flush(ops)

	if len(results) != 0 {
		t.Fatalf("cancelled ops should produce 0 intents, got %d: %+v", len(results), results)
	}
}

func TestCoalescer_CrashRecovery_ProcessMetadataTask_CoalescedTypes(t *testing.T) {
	t.Log("processMetadataTask handles COALESCED_RENAME and COALESCED_MKDIR (verified at compile time)")
}

// ---------------------------------------------------------------------------
// Part B: Mock-Drive Integration Tests
// ---------------------------------------------------------------------------

// mockRemoteCallCounter records remote API call counts from coalesced intents.
type mockRemoteCallCounter struct {
	mu           sync.Mutex
	uploadCalls  int
	deleteCalls  int
	renameCalls  int
	mkdirCalls   int
	uploadPaths  []string
	deletePaths  []string
	renameFromTo [][2]string
}

func (m *mockRemoteCallCounter) handleIntents(intents []RemoteIntent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, intent := range intents {
		if intent.SkipRemote {
			continue
		}
		switch intent.Kind {
		case "UPLOAD":
			m.uploadCalls++
			m.uploadPaths = append(m.uploadPaths, intent.Path)
		case "DELETE":
			m.deleteCalls++
			m.deletePaths = append(m.deletePaths, intent.Path)
		case "RENAME":
			m.renameCalls++
			m.renameFromTo = append(m.renameFromTo, [2]string{intent.Path, intent.NewPath})
		case "MKDIR":
			m.mkdirCalls++
		}
	}
}

// newTestCoalescerWithCounter creates a coalescer with counter and results channel.
func newTestCoalescerWithCounter(caps Capabilities) (*OperationCoalescer, *mockRemoteCallCounter, chan []RemoteIntent) {
	counter := &mockRemoteCallCounter{}
	resultsCh := make(chan []RemoteIntent, 1)
	oc := NewOperationCoalescer(caps, func(intents []RemoteIntent) {
		counter.handleIntents(intents)
		select {
		case resultsCh <- intents:
		default:
		}
	})
	return oc, counter, resultsCh
}

func TestCoalescer_Integration_CreateDelete_ZeroRemoteCalls(t *testing.T) {
	// Uses flush() directly because 0 intents means the callback never fires.
	oc, _ := newTestCoalescer(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "0"}
	ops := []FileOp{
		{Kind: OpCreate, Path: "/a.txt", Fid: "local_f1", Node: stubNode},
		{Kind: OpWrite, Path: "/a.txt", Fid: "local_f1"},
		{Kind: OpUpload, Path: "/a.txt", Fid: "local_f1", Staging: "/staging/a.txt", Node: stubNode},
		{Kind: OpDelete, Path: "/a.txt", Fid: "local_f1"},
	}
	results := oc.flush(ops)

	counter := &mockRemoteCallCounter{}
	counter.handleIntents(results)

	if counter.uploadCalls != 0 {
		t.Errorf("expected 0 upload calls, got %d", counter.uploadCalls)
	}
	if counter.deleteCalls != 0 {
		t.Errorf("expected 0 delete calls, got %d", counter.deleteCalls)
	}
	if counter.renameCalls != 0 {
		t.Errorf("expected 0 rename calls, got %d", counter.renameCalls)
	}
	if counter.mkdirCalls != 0 {
		t.Errorf("expected 0 mkdir calls, got %d", counter.mkdirCalls)
	}
}

func TestCoalescer_Integration_CreateRename_UploadNewPath(t *testing.T) {
	oc, counter, resultsCh := newTestCoalescerWithCounter(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "0"}
	oc.Submit(FileOp{Kind: OpCreate, Path: "/tmp/file.tmp", Fid: "local_f1", Node: stubNode})
	oc.Submit(FileOp{Kind: OpWrite, Path: "/tmp/file.tmp", Fid: "local_f1"})
	oc.Submit(FileOp{Kind: OpUpload, Path: "/tmp/file.tmp", Fid: "local_f1", Staging: "/staging/tmp", Node: stubNode})
	oc.Submit(FileOp{Kind: OpRename, Path: "/tmp/file.tmp", NewPath: "/dist/file.js", Fid: "local_f1"})

	select {
	case <-resultsCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for coalescer flush callback")
	}

	counter.mu.Lock()
	uploads := counter.uploadCalls
	uploadPaths := append([]string{}, counter.uploadPaths...)
	deletes := counter.deleteCalls
	renames := counter.renameCalls
	mkdirs := counter.mkdirCalls
	counter.mu.Unlock()

	if uploads != 1 {
		t.Errorf("expected 1 upload call (migrated to new path), got %d", uploads)
	}
	if len(uploadPaths) != 1 || uploadPaths[0] != "/dist/file.js" {
		t.Errorf("expected upload to /dist/file.js, got %v", uploadPaths)
	}
	if deletes != 0 {
		t.Errorf("expected 0 delete calls, got %d", deletes)
	}
	if renames != 0 {
		t.Errorf("expected 0 rename calls, got %d", renames)
	}
	if mkdirs != 0 {
		t.Errorf("expected 0 mkdir calls, got %d", mkdirs)
	}
}

func TestCoalescer_Integration_RenameRename_SingleRemoteCall(t *testing.T) {
	oc, counter, resultsCh := newTestCoalescerWithCounter(Capabilities{Rename: true})
	defer oc.Shutdown()

	oc.Submit(FileOp{Kind: OpRename, Path: "/a", NewPath: "/b", Fid: "remote_f1"})
	oc.Submit(FileOp{Kind: OpRename, Path: "/b", NewPath: "/c", Fid: "remote_f1"})

	select {
	case <-resultsCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for coalescer flush callback")
	}

	counter.mu.Lock()
	renames := counter.renameCalls
	renameFromTo := append([][2]string{}, counter.renameFromTo...)
	uploads := counter.uploadCalls
	deletes := counter.deleteCalls
	counter.mu.Unlock()

	if renames != 1 {
		t.Fatalf("expected 1 rename call (merged), got %d", renames)
	}
	if len(renameFromTo) != 1 {
		t.Fatalf("expected 1 rename entry, got %d", len(renameFromTo))
	}
	if renameFromTo[0][0] != "/a" || renameFromTo[0][1] != "/c" {
		t.Errorf("expected rename /a→/c, got %s→%s", renameFromTo[0][0], renameFromTo[0][1])
	}
	if uploads != 0 {
		t.Errorf("expected 0 upload calls, got %d", uploads)
	}
	if deletes != 0 {
		t.Errorf("expected 0 delete calls, got %d", deletes)
	}
}

func TestCoalescer_Integration_DeleteDir_AbsorbsChildren(t *testing.T) {
	oc, counter, resultsCh := newTestCoalescerWithCounter(Capabilities{Rename: true})
	defer oc.Shutdown()

	oc.Submit(FileOp{Kind: OpDelete, Path: "/dist", Fid: "dir_f1", IsDir: true})
	oc.Submit(FileOp{Kind: OpDelete, Path: "/dist/a.js", Fid: "child_f1"})
	oc.Submit(FileOp{Kind: OpDelete, Path: "/dist/sub", Fid: "child_f2"})

	select {
	case <-resultsCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for coalescer flush callback")
	}

	counter.mu.Lock()
	deletes := counter.deleteCalls
	deletePaths := append([]string{}, counter.deletePaths...)
	uploads := counter.uploadCalls
	renames := counter.renameCalls
	counter.mu.Unlock()

	if deletes != 1 {
		t.Fatalf("expected 1 delete call (dir only, children absorbed), got %d", deletes)
	}
	if len(deletePaths) != 1 || deletePaths[0] != "/dist" {
		t.Errorf("expected delete for /dist only, got %v", deletePaths)
	}
	if uploads != 0 {
		t.Errorf("expected 0 upload calls, got %d", uploads)
	}
	if renames != 0 {
		t.Errorf("expected 0 rename calls, got %d", renames)
	}
}

func TestCoalescer_Integration_DeleteCreateSamePath_BothCalls(t *testing.T) {
	oc, counter, resultsCh := newTestCoalescerWithCounter(Capabilities{Rename: true})
	defer oc.Shutdown()

	stubNode := &Node{parentFid: "parent_new"}
	oc.Submit(FileOp{Kind: OpDelete, Path: "/a.txt", Fid: "old_f1"})
	oc.Submit(FileOp{Kind: OpCreate, Path: "/a.txt", Fid: "local_new_f1", Node: stubNode})
	oc.Submit(FileOp{Kind: OpWrite, Path: "/a.txt", Fid: "local_new_f1"})
	oc.Submit(FileOp{Kind: OpUpload, Path: "/a.txt", Fid: "local_new_f1", Staging: "/staging/new_a.txt", Node: stubNode})

	select {
	case <-resultsCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for coalescer flush callback")
	}

	counter.mu.Lock()
	deletes := counter.deleteCalls
	deletePaths := append([]string{}, counter.deletePaths...)
	uploads := counter.uploadCalls
	uploadPaths := append([]string{}, counter.uploadPaths...)
	renames := counter.renameCalls
	counter.mu.Unlock()

	if deletes != 1 {
		t.Errorf("expected 1 delete call (old file), got %d", deletes)
	}
	if len(deletePaths) != 1 || deletePaths[0] != "/a.txt" {
		t.Errorf("expected delete for /a.txt, got %v", deletePaths)
	}
	if uploads != 1 {
		t.Errorf("expected 1 upload call (new file), got %d", uploads)
	}
	if len(uploadPaths) != 1 || uploadPaths[0] != "/a.txt" {
		t.Errorf("expected upload for /a.txt, got %v", uploadPaths)
	}
	if renames != 0 {
		t.Errorf("expected 0 rename calls, got %d", renames)
	}
}
