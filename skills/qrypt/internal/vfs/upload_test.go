package vfs

import (
	"context"
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

func (m *mockObserver) OnUploadComplete(path string, newFid string, err error) {
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

	um.MarkDirty("/file.txt", "parent_1", "file.txt", 42, "", stagingReader("hello"))

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

	um.MarkDirty("/file.txt", "parent_1", "file.txt", 100, "", stagingReader("data"))
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

	um.MarkDirty("/file.txt", "parent_1", "old.txt", 10, "", stagingReader("old"))
	um.MarkDirty("/file.txt", "parent_1", "new.txt", 20, "", stagingReader("new"))

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

	um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("x"))

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

	um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("hello"))

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
	var uploadedName string
	var uploadedSize int64

	exec := &mockExecutor{
		uploadErr: assert.AnError,
	}
	// Wrap Upload to capture the first call's params and fail, then succeed.
	wrapped := &retryCaptureExecutor{
		mockExecutor: exec,
		count:        &callCount,
		name:         &uploadedName,
		size:         &uploadedSize,
	}
	um, obs := newUploadManager(t, wrapped)
	um.SetMaxRetries(3)

	um.MarkDirty("/file.txt", "p", "f.txt", 99, "", stagingReader("data"))

	fireAll(um) // attempt 1
	fireAll(um) // attempt 2

	assert.Equal(t, int32(2), callCount.Load())
	assert.Equal(t, "f.txt", uploadedName)
	assert.Equal(t, int64(99), uploadedSize)
	assert.False(t, um.IsPending("/file.txt"))
	assert.NoError(t, obs.results()[0].err)
}

type retryCaptureExecutor struct {
	*mockExecutor
	count *atomic.Int32
	name  *string
	size  *int64
}

func (r *retryCaptureExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	n := r.count.Add(1)
	*r.name = name
	*r.size = size
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
			um.MarkDirty("/file.txt", "p", "f.txt", 1, "", stagingReader("x"))
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

	um.MarkDirty("/f.txt", "0", "f.txt", 10, "", stagingReader("v1"))
	time.Sleep(100 * time.Millisecond)

	// Second write while first upload may be running.
	um.MarkDirty("/f.txt", "0", "f.txt", 20, "", stagingReader("v2-final"))
	time.Sleep(200 * time.Millisecond)

	c := exec.calls()
	assert.GreaterOrEqual(t, len(c), 1, "should have at least one upload")
	if len(c) > 0 {
		last := c[len(c)-1]
		assert.Equal(t, int64(20), last.size, "last upload should use final size 20")
	}
}
