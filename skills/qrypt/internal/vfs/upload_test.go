package vfs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// ---------------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------------

type mockExecutor struct {
	mu          sync.Mutex
	uploadCalls []uploadCall
	uploadErr   error
	errs []error // per-call errors: errs[n] for nth call; nil means use uploadErr
}

type uploadCall struct {
	parentID string
	name     string
	size     int64
}

func (m *mockExecutor) Upload(_ context.Context, parentID, name string, size int64, _ io.Reader) (drivers.Entry, error) {
	m.mu.Lock()
	n := len(m.uploadCalls)
	m.uploadCalls = append(m.uploadCalls, uploadCall{parentID, name, size})
	err := m.uploadErr
	if n < len(m.errs) {
		err = m.errs[n]
	}
	m.mu.Unlock()
	if err != nil {
		return drivers.Entry{}, err
	}
	return drivers.Entry{ID: "fid_1", Name: name, Size: size}, nil
}

func (m *mockExecutor) calls() []uploadCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]uploadCall, len(m.uploadCalls))
	copy(out, m.uploadCalls)
	return out
}

type mockObserver struct {
	mu    sync.Mutex
	calls []observerCall
}

type observerCall struct {
	path string
	err  error
}

func (m *mockObserver) OnUploadComplete(path string, newFid string, encSize int64, err error, attempt *uploadAttempt) {
	m.mu.Lock()
	m.calls = append(m.calls, observerCall{path, err})
	m.mu.Unlock()
}

func (m *mockObserver) results() []observerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]observerCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// stagingReader returns a reader that, when read, returns the string s.
func stagingReader(s string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(s)), nil
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newUploadManager(t *testing.T, exec Uploader) (*UploadManager, *mockObserver) {
	t.Helper()
	obs := &mockObserver{}
	// Use a huge writeBack so real timers never fire — we drive via FireAllTimers.
	um := NewUploadManager(exec, time.Hour, obs)
	return um, obs
}

// fireAll fires pending timers and waits for goroutines to settle.
func fireAll(um *UploadManager) {
	um.FireAllTimers()
	time.Sleep(50 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestUploadManager_MarkDirty_TriggersUpload(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, obs := newUploadManager(t, exec)

	um.MarkDirty("/file.txt", "parent_1", "file.txt", 42, "", stagingReader("hello"), nil)

	assert.True(t, um.IsPending("/file.txt"), "should be pending before timer fires")

	fireAll(um)

	calls := exec.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "parent_1", calls[0].parentID)
	assert.Equal(t, "file.txt", calls[0].name)
	assert.Equal(t, int64(42), calls[0].size)

	obsCalls := obs.results()
	require.Len(t, obsCalls, 1)
	assert.Equal(t, "/file.txt", obsCalls[0].path)
	assert.NoError(t, obsCalls[0].err)

	assert.False(t, um.IsPending("/file.txt"), "should not be pending after upload")
}

func TestUploadManager_CancelSync_PreventsUpload(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, obs := newUploadManager(t, exec)

	um.MarkDirty("/file.txt", "parent_1", "file.txt", 100, "", stagingReader("data"), nil)
	assert.True(t, um.IsPending("/file.txt"))

	um.CancelSync("/file.txt")
	assert.False(t, um.IsPending("/file.txt"), "should not be pending after cancel")

	fireAll(um)

	assert.Empty(t, exec.calls(), "upload should not have been called")
	assert.Empty(t, obs.results(), "observer should not have been called")
}

func TestUploadManager_MultipleMarksSameFile(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, obs := newUploadManager(t, exec)

	um.MarkDirty("/file.txt", "parent_1", "old.txt", 10, "", stagingReader("old"), nil)
	um.MarkDirty("/file.txt", "parent_1", "new.txt", 20, "", stagingReader("new"), nil)

	fireAll(um)

	calls := exec.calls()
	require.Len(t, calls, 1, "only one upload should fire after coalesced marks")
	assert.Equal(t, "new.txt", calls[0].name, "should use the latest name")
	assert.Equal(t, int64(20), calls[0].size, "should use the latest size")

	obsCalls := obs.results()
	require.Len(t, obsCalls, 1)
	assert.NoError(t, obsCalls[0].err)
	assert.False(t, um.IsPending("/file.txt"))
}

func TestUploadManager_RetryOnFailure(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{
		uploadErr: assert.AnError,
	}
	um, obs := newUploadManager(t, exec)
	um.SetMaxRetries(3)

	um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("x"), nil)

	fireAll(um) // attempt 1 → fail → retry 1 timer created
	fireAll(um) // attempt 2 → fail → retry 2 timer created
	fireAll(um) // attempt 3 → fail → maxRetries=3 reached → final failure

	calls := exec.calls()
	assert.Len(t, calls, 3, "should have 3 attempts (original + 2 retries)")

	obsCalls := obs.results()
	require.Len(t, obsCalls, 1, "observer fires once on final failure")
	assert.Equal(t, "/file.txt", obsCalls[0].path)
	assert.Error(t, obsCalls[0].err)

	assert.False(t, um.IsPending("/file.txt"))
}

func TestUploadManager_UploadAfterSuccessfulRetry(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{
		errs: []error{assert.AnError, nil}, // fail once, succeed on retry
	}
	um, obs := newUploadManager(t, exec)
	um.SetMaxRetries(3)

	um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("hello"), nil)

	fireAll(um) // attempt 1 → fail → retry timer created
	fireAll(um) // attempt 2 → success

	calls := exec.calls()
	require.Len(t, calls, 2)
	assert.NoError(t, obs.results()[0].err)
	assert.False(t, um.IsPending("/file.txt"))
}

func TestUploadManager_RetryPreservesInfo(t *testing.T) {
	// Verify that the upload info (name, size) stays consistent across retries.
	t.Parallel()

	var callCount atomic.Int32

	exec := &mockExecutor{
		uploadErr: assert.AnError,
	}
	// Wrap Upload to capture the first call's params and fail, then succeed.
	wrapped := &retryCaptureExecutor{
		mockExecutor: exec,
		count:        &callCount,
	}
	um, obs := newUploadManager(t, wrapped)
	um.SetMaxRetries(3)

	um.MarkDirty("/file.txt", "p", "f.txt", 99, "", stagingReader("data"), nil)

	fireAll(um) // attempt 1
	fireAll(um) // attempt 2

	assert.Equal(t, int32(2), callCount.Load())
	assert.Equal(t, "f.txt", wrapped.name.Load().(string))
	assert.Equal(t, int64(99), wrapped.size.Load())
	assert.False(t, um.IsPending("/file.txt"))
	assert.NoError(t, obs.results()[0].err)
}

type retryCaptureExecutor struct {
	*mockExecutor
	count     *atomic.Int32
	name      atomic.Value
	size      atomic.Int64
}

func (r *retryCaptureExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	n := r.count.Add(1)
	r.name.Store(name)
	r.size.Store(size)
	if n == 2 {
		return drivers.Entry{ID: "fid_ok", Name: name, Size: size}, nil
	}
	return drivers.Entry{}, assert.AnError
}

func TestUploadManager_ConcurrentCancelAndMark(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, obs := newUploadManager(t, exec)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("x"), nil)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			um.CancelSync("/file.txt")
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Drain any pending timers.
	fireAll(um)

	// The final state depends on the interleaving, but the system shouldn't panic.
	// At minimum the mock executor should have been called at most once per pending
	// mark that survived the cancels.
	t.Logf("upload calls: %d, observer calls: %d", len(exec.calls()), len(obs.results()))
}

func TestUploadManager_InFlight(t *testing.T) {
	exec := &mockExecutor{}
	obs := &mockObserver{}
	um := NewUploadManager(exec, 50*time.Millisecond, obs)

	um.MarkDirty("/f.txt", "0", "f.txt", 10, "", stagingReader("v1"), nil)
	time.Sleep(100 * time.Millisecond)

	// Second write while first upload may be running.
	um.MarkDirty("/f.txt", "0", "f.txt", 20, "", stagingReader("v2-final"), nil)
	time.Sleep(200 * time.Millisecond)

	c := exec.calls()
	assert.GreaterOrEqual(t, len(c), 1, "should have at least one upload")
	if len(c) > 0 {
		last := c[len(c)-1]
		assert.Equal(t, int64(20), last.size, "last upload should use final size 20")
	}
}

// ---------------------------------------------------------------------------
// lifecycle integration tests — prove MarkUploadStarted / MarkUploadCommitted
// are called from the real upload path.
// ---------------------------------------------------------------------------

func TestUploadManager_LifecycleCallsUploadFlow(t *testing.T) {
	t.Parallel()

	// Set up coalescer with a flush channel.
	flushCh := make(chan []RemoteIntent, 10)
	coalescer := NewOperationCoalescer(
		Capabilities{Rename: true, Move: true},
		func(intents []RemoteIntent) { flushCh <- intents },
	)
	t.Cleanup(coalescer.Shutdown)

	// Pre-seed an upload intent so the coalescer knows about this FID.
	node := NewVFSNode("fid_upload", "parent", "file.txt", "/file.txt", false)
	coalescer.Submit(FileOp{
		Kind:  OpUpload,
		Path:  "/file.txt",
		Fid:   node.FID(),
		Time:  time.Now(),
		Node:  node,
	})

	// Drain the flush so the intent is created and tracked.
	<-flushCh
	time.Sleep(50 * time.Millisecond)

	exec := &mockExecutor{}
	obs := &mockObserver{}
	um := NewUploadManager(exec, time.Hour, obs)
	um.SetCoalescer(coalescer)
	t.Cleanup(func() { um.CancelSync("/file.txt") })

	um.MarkDirty("/file.txt", "parent", "file.txt", 42, "", stagingReader("hello"), node)

	// Fire the write-back timer
	fireAll(um)

	// Verify that both lifecycle methods updated the intent state.
	intent := coalescer.findIntent("fid_upload")
	require.NotNil(t, intent, "upload intent should still be tracked")
	require.NotNil(t, intent.Upload, "upload intent should have Upload sub-intent")
	assert.True(t, intent.Upload.Started, "MarkUploadStarted should have been called")
	assert.True(t, intent.Upload.Committed, "MarkUploadCommitted should have been called")
	assert.True(t, intent.Started, "PendingIntent.Started should be set")
	assert.True(t, intent.Committed, "PendingIntent.Committed should be set")

	uploads := exec.calls()
	require.Len(t, uploads, 1, "should have triggered exactly one upload")
}

// ---------------------------------------------------------------------------
// concurrency control tests
// ---------------------------------------------------------------------------

// blockingExecutor wraps mockExecutor and gates Upload calls so the test
// can observe how many are in-flight at once.
type blockingExecutor struct {
	mockExecutor
	started chan struct{} // unbuffered: test reads to know an Upload started
	proceed chan struct{} // close to unblock all waiting Uploads
}

func (e *blockingExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	e.started <- struct{}{}
	<-e.proceed
	return e.mockExecutor.Upload(ctx, parentID, name, size, body)
}

func TestUploadManager_ConcurrencyLimitEnforced(t *testing.T) {
	t.Parallel()

	const limit = 3
	const numPaths = 10

	be := &blockingExecutor{
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	um, _ := newUploadManager(t, be)
	um.SetConcurrencyLimit(limit)

	// Mark all paths dirty — they'll each schedule a timer.
	for i := 0; i < numPaths; i++ {
		path := fmt.Sprintf("/f_%d.txt", i)
		um.MarkDirty(path, "p", path, int64(i+1), "", stagingReader("x"), nil)
	}

	// Fire all timers at once — goroutines race through the semaphore;
	// at most 'limit' should reach the Upload call.
	um.FireAllTimers()

	// Read from started exactly 'limit' times. Each read unblocks one
	// goroutine that reached Upload; the goroutine then blocks on proceed.
	for i := 0; i < limit; i++ {
		select {
		case <-be.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for upload %d/%d to start", i+1, limit)
		}
	}

	// No more than 'limit' should have started — the (limit+1)th is
	// blocked on the semaphore.
	select {
	case <-be.started:
		t.Fatal("more uploads started than the concurrency limit allows")
	case <-time.After(200 * time.Millisecond):
	}

	// Verify peak concurrent equals the limit.
	assert.Equal(t, limit, um.PeakConcurrentUploads(),
		"peak concurrent uploads should match the configured limit")

	// Unblock all in-flight uploads so they finish.
	close(be.proceed)

	// Let remaining timer goroutines drain through the semaphore.
	time.Sleep(100 * time.Millisecond)
}

func TestUploadManager_ConcurrencyLimitDefault(t *testing.T) {
	t.Parallel()

	const numPaths = 10

	be := &blockingExecutor{
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	// Use NewUploadManager directly (no SetConcurrencyLimit) — defaults to 4.
	um := NewUploadManager(be, time.Hour, &mockObserver{})

	for i := 0; i < numPaths; i++ {
		path := fmt.Sprintf("/g_%d.txt", i)
		um.MarkDirty(path, "p", path, int64(i+1), "", stagingReader("x"), nil)
	}

	um.FireAllTimers()

	const defaultLimit = 4
	for i := 0; i < defaultLimit; i++ {
		select {
		case <-be.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for upload %d/%d to start", i+1, defaultLimit)
		}
	}

	select {
	case <-be.started:
		t.Fatal("more uploads started than default concurrency limit (4)")
	case <-time.After(200 * time.Millisecond):
	}

	peak := um.PeakConcurrentUploads()
	assert.GreaterOrEqual(t, peak, 1, "peak should be at least 1")
	assert.LessOrEqual(t, peak, defaultLimit,
		"peak concurrent uploads should not exceed default limit")

	close(be.proceed)
	time.Sleep(100 * time.Millisecond)
}

func TestUploadManager_ConcurrencyLimitZeroDefaults(t *testing.T) {
	t.Parallel()

	um := NewUploadManager(&mockExecutor{}, time.Hour, &mockObserver{})
	um.SetConcurrencyLimit(0) // should become 4

	um.MarkDirty("/f.txt", "p", "f.txt", 1, "", stagingReader("x"), nil)
	um.FireAllTimers()
	time.Sleep(50 * time.Millisecond)

	// The semaphore was created with cap=4, so the upload proceeds normally.
	// We just verify no panic and peak is reasonable.
	assert.GreaterOrEqual(t, um.PeakConcurrentUploads(), 1)
}

func TestUploadManager_ConcurrencyLimitRaceSafe(t *testing.T) {
	t.Parallel()

	const limit = 2
	const numPaths = 20

	be := &blockingExecutor{
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	um, _ := newUploadManager(t, be)
	um.SetConcurrencyLimit(limit)

	// Concurrently mark and fire from multiple goroutines.
	var wg sync.WaitGroup
	for i := 0; i < numPaths; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			path := fmt.Sprintf("/h_%d.txt", i)
			um.MarkDirty(path, "p", path, int64(i+1), "", stagingReader("x"), nil)
		}()
	}

	// Fire timers while marks are still being made.
	um.FireAllTimers()

	wg.Wait()

	// Fire remaining timers that may have been added after the first fire.
	um.FireAllTimers()

	// Read started count — should not exceed limit.
	for i := 0; i < limit; i++ {
		select {
		case <-be.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for upload %d/%d to start", i+1, limit)
		}
	}

	select {
	case <-be.started:
		t.Fatal("more uploads started than limit allows in race test")
	case <-time.After(200 * time.Millisecond):
	}

	peak := um.PeakConcurrentUploads()
	assert.LessOrEqual(t, peak, limit,
		"peak concurrent uploads must not exceed limit in race test")

	close(be.proceed)
	time.Sleep(100 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// cancellation & shutdown tests
// ---------------------------------------------------------------------------

// cancelDetectExecutor blocks in Upload until either the context is
// cancelled or proceed is closed, counting how many took the cancellation path.
type cancelDetectExecutor struct {
	started     chan struct{}
	proceed     chan struct{}
	cancelCount atomic.Int32
}

func (e *cancelDetectExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	e.started <- struct{}{}
	select {
	case <-ctx.Done():
		e.cancelCount.Add(1)
		return drivers.Entry{}, ctx.Err()
	case <-e.proceed:
		return drivers.Entry{ID: "fid_ok", Name: name, Size: size}, nil
	}
}

func TestUploadManager_CancelSync_CancelsActiveUpload(t *testing.T) {
	t.Parallel()

	exec := &cancelDetectExecutor{
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	um, obs := newUploadManager(t, exec)

	um.MarkDirty("/f.txt", "p", "f.txt", 10, "", stagingReader("x"), nil)
	um.FireAllTimers()

	// Wait for Upload to be entered (blocked on select)
	<-exec.started

	// Cancel while upload is in-flight
	um.CancelSync("/f.txt")

	// Verify context was cancelled by CancelSync
	time.Sleep(50 * time.Millisecond)
	assert.True(t, exec.cancelCount.Load() > 0,
		"upload context should be cancelled by CancelSync")

	// Unblock any remaining select-waiting goroutine
	close(exec.proceed)
	time.Sleep(50 * time.Millisecond)

	// No observer notification since the upload was terminated
	assert.Empty(t, obs.results(), "observer should not be called for cancelled upload")
}

func TestUploadManager_Shutdown_DrainsAndCancels(t *testing.T) {
	t.Parallel()

	exec := &cancelDetectExecutor{
		started: make(chan struct{}, 10), // buffered so drain goroutines don't deadlock
		proceed: make(chan struct{}),
	}
	um, _ := newUploadManager(t, exec)
	const limit = 3
	um.SetConcurrencyLimit(limit)

	// Start more uploads than the concurrency limit.
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("/f_%d.txt", i)
		um.MarkDirty(path, "p", path, int64(i+1), "", stagingReader("x"), nil)
	}

	um.FireAllTimers()

	// Wait for exactly 'limit' goroutines to enter Upload.
	for i := 0; i < limit; i++ {
		<-exec.started
	}

	// Confirm the limit is reached.
	assert.Equal(t, int32(limit), atomic.LoadInt32(&um.concurrentCount),
		"should have exactly limit concurrent uploads")

	// Shutdown — cancels all roots, waits for semaphore to drain.
	um.Shutdown()

	// After Shutdown returns, no upload goroutines are running.
	assert.Equal(t, int32(0), atomic.LoadInt32(&um.concurrentCount),
		"concurrent count should be 0 after Shutdown")

	// At least the in-flight uploads were cancelled by root-context cancellation.
	assert.Greater(t, exec.cancelCount.Load(), int32(0),
		"in-flight uploads should be cancelled by Shutdown")
}

func TestUploadManager_NoUploadAfterShutdown(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, obs := newUploadManager(t, exec)

	// Verify upload works before shutdown.
	um.MarkDirty("/before.txt", "p", "before.txt", 1, "", stagingReader("x"), nil)
	fireAll(um)
	require.Len(t, exec.calls(), 1, "upload should work before shutdown")
	require.Len(t, obs.results(), 1, "observer should be called before shutdown")

	// Shutdown.
	um.Shutdown()

	// MarkDirty after shutdown records state but does NOT schedule a new upload.
	um.MarkDirty("/after.txt", "p", "after.txt", 2, "", stagingReader("y"), nil)
	assert.True(t, um.IsPending("/after.txt"), "state should be recorded after shutdown")

	// FireAllTimers does nothing (all timers were stopped and cleared).
	fireAll(um)

	// No new upload goroutine was started.
	assert.Len(t, exec.calls(), 1, "no new upload should start after shutdown")
	assert.Len(t, obs.results(), 1, "observer should not be called again after shutdown")
}

func TestUploadManager_RetryCancelDoesNotResurrect(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{
		uploadErr: assert.AnError,
	}
	um, obs := newUploadManager(t, exec)
	um.SetMaxRetries(3)

	um.MarkDirty("/f.txt", "p", "f.txt", 1, "", stagingReader("x"), nil)

	// Attempt 1 → fails → retry timer created (backoff=2s, won't fire naturally).
	fireAll(um)

	require.Len(t, exec.calls(), 1, "first upload attempt should have been made")

	// CancelSync stops the retry timer and removes all state.
	um.CancelSync("/f.txt")

	// FireAllTimers — the retry timer was stopped and removed, so this is a no-op.
	fireAll(um)

	// No additional upload should have happened.
	assert.Len(t, exec.calls(), 1, "no upload after CancelSync killed the retry timer")
	assert.Empty(t, obs.results(), "observer should not be called (upload never succeeded)")
}

func TestUploadManager_FireAllTimersAfterShutdown(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	um, _ := newUploadManager(t, exec)

	um.MarkDirty("/f.txt", "p", "f.txt", 1, "", stagingReader("x"), nil)

	// Shutdown stops all pending timers and clears the timer map.
	um.Shutdown()

	// FireAllTimers on an empty timer map must not panic.
	require.NotPanics(t, func() {
		um.FireAllTimers()
	}, "FireAllTimers after Shutdown should not panic")

	// No upload should have been triggered.
	assert.Empty(t, exec.calls(), "no upload should occur after shutdown")
}

// ---------------------------------------------------------------------------
// benchmarks
// ---------------------------------------------------------------------------

// countingExecutor records Upload calls and uses a WaitGroup so benchmarks
// can wait for all uploads to complete.
type countingExecutor struct {
	mu sync.Mutex
	n  int
	wg sync.WaitGroup
}

func (e *countingExecutor) Upload(_ context.Context, parentID, name string, size int64, _ io.Reader) (drivers.Entry, error) {
	defer e.wg.Done()
	e.mu.Lock()
	e.n++
	e.mu.Unlock()
	return drivers.Entry{ID: "fid_1", Name: name, Size: size}, nil
}

func BenchmarkVFSUploadStorm(b *testing.B) {
	b.ReportAllocs()

	const numPaths = 50

	for i := 0; i < b.N; i++ {
		exec := &countingExecutor{}
		exec.wg.Add(numPaths)
		um := NewUploadManager(exec, time.Hour, &mockObserver{})
		um.SetConcurrencyLimit(4)

		for j := 0; j < numPaths; j++ {
			path := fmt.Sprintf("/f_%d.txt", j)
			um.MarkDirty(path, "p", path, int64(j+1), "", stagingReader("x"), nil)
		}

		um.FireAllTimers()
		exec.wg.Wait()
		um.Shutdown()
	}
}

func BenchmarkVFSUploadRepeatedDirty(b *testing.B) {
	b.ReportAllocs()

	const numMarks = 100

	for i := 0; i < b.N; i++ {
		exec := &countingExecutor{}
		exec.wg.Add(1)
		um := NewUploadManager(exec, time.Hour, &mockObserver{})

		for j := 0; j < numMarks; j++ {
			um.MarkDirty("/file.txt", "p", fmt.Sprintf("v%d.txt", j), int64(j), "", stagingReader("x"), nil)
		}

		um.FireAllTimers()
		exec.wg.Wait()
		um.Shutdown()

		// Multiple marks for the same path must coalesce to a single upload.
		if exec.n != 1 {
			b.Fatalf("expected 1 upload after %d marks, got %d", numMarks, exec.n)
		}
	}
}

// ---------------------------------------------------------------------------
// stress test: bounded concurrency with many dirty paths
// ---------------------------------------------------------------------------

func TestUploadManager_StressManyDirtyPathsBoundedConcurrency(t *testing.T) {
	const numPaths = 50
	const limit = 3

	be := &blockingExecutor{
		started: make(chan struct{}, numPaths),
		proceed: make(chan struct{}),
	}
	um, _ := newUploadManager(t, be)
	um.SetConcurrencyLimit(limit)

	for i := 0; i < numPaths; i++ {
		path := fmt.Sprintf("/f_%d.txt", i)
		um.MarkDirty(path, "p", path, int64(i+1), "", stagingReader("x"), nil)
	}

	// Fire all timers — goroutines race through the semaphore.
	um.FireAllTimers()

	// Exactly 'limit' uploads should start immediately.
	for i := 0; i < limit; i++ {
		select {
		case <-be.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for upload %d/%d to start", i+1, limit)
		}
	}

	// No more than 'limit' should have started.
	select {
	case <-be.started:
		t.Fatal("more uploads started than the concurrency limit allows")
	case <-time.After(200 * time.Millisecond):
	}

	peak := um.PeakConcurrentUploads()
	assert.LessOrEqual(t, peak, limit,
		"peak concurrent uploads must not exceed the configured limit")

	// Unblock in-flight uploads so the rest can drain through the semaphore.
	close(be.proceed)

	// Wait for all paths to complete — each must leave the pending set.
	for i := 0; i < numPaths; i++ {
		path := fmt.Sprintf("/f_%d.txt", i)
		require.Eventually(t, func() bool {
			return !um.IsPending(path)
		}, 10*time.Second, 50*time.Millisecond,
			"path %s should eventually complete", path)
	}

	um.Shutdown()
}
