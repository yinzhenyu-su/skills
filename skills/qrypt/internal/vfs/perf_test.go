package vfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

const mockUploadBlocksPerPart = 128

type silentLogger struct{}

func (silentLogger) Printf(string, ...interface{}) {}

type mockUploadTransport struct {
	bandwidthBytesPerSec int64
	partLatency          time.Duration
	controlLatency       time.Duration

	mu            sync.Mutex
	partNumbers   []int
	partSizes     []int
	uploadedBytes int64
}

func (t *mockUploadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()

	switch {
	case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/file/upload/pre"):
		time.Sleep(t.controlLatency)
		return jsonResponse(map[string]any{
			"status":  200,
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"task_id":    "task-1",
				"upload_id":  "upload-1",
				"obj_key":    "bench-object",
				"upload_url": "https://mock-oss.local",
				"fid":        "remote-fid-1",
				"finish":     false,
				"bucket":     "bench-bucket",
				"callback": map[string]any{
					"callbackUrl":  "https://callback.local",
					"callbackBody": "ok",
				},
				"auth_info": "auth-info",
			},
			"metadata": map[string]any{
				"part_size": 8 * 1024 * 1024,
			},
		}), nil
	case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/file/upload/auth"):
		time.Sleep(t.controlLatency)
		return jsonResponse(map[string]any{
			"status":  200,
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"auth_key": "mock-auth-key",
			},
		}), nil
	case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/file/update/hash"):
		time.Sleep(t.controlLatency)
		return jsonResponse(map[string]any{
			"status":  200,
			"code":    0,
			"message": "ok",
			"data": map[string]any{
				"finish": false,
				"fid":    "remote-fid-1",
			},
		}), nil
	case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/file/upload/finish"):
		time.Sleep(t.controlLatency)
		return jsonResponse(map[string]any{
			"status":  200,
			"code":    0,
			"message": "ok",
		}), nil
	case req.Method == http.MethodPut && req.URL.Query().Get("partNumber") != "":
		partNumber, err := strconv.Atoi(req.URL.Query().Get("partNumber"))
		if err != nil {
			return nil, err
		}
		t.simulateUpload(len(body))
		t.mu.Lock()
		t.partNumbers = append(t.partNumbers, partNumber)
		t.partSizes = append(t.partSizes, len(body))
		t.uploadedBytes += int64(len(body))
		t.mu.Unlock()
		resp := emptyResponse(http.StatusOK)
		resp.Header.Set("Etag", fmt.Sprintf("etag-%d", partNumber))
		return resp, nil
	case req.Method == http.MethodPost && req.URL.Query().Get("uploadId") != "":
		time.Sleep(t.controlLatency)
		return emptyResponse(http.StatusOK), nil
	default:
		return textResponse(http.StatusNotFound, "unexpected request"), nil
	}
}

func (t *mockUploadTransport) simulateUpload(size int) {
	delay := t.partLatency
	if t.bandwidthBytesPerSec > 0 {
		delay += time.Duration(int64(size)) * time.Second / time.Duration(t.bandwidthBytesPerSec)
	}
	time.Sleep(delay)
}

func (t *mockUploadTransport) stats() (partNumbers []int, partSizes []int, uploadedBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	partNumbers = append([]int(nil), t.partNumbers...)
	partSizes = append([]int(nil), t.partSizes...)
	return partNumbers, partSizes, t.uploadedBytes
}

type uploadPerfResult struct {
	Duration       time.Duration
	ThroughputMiBS float64
	PartNumbers    []int
	PartSizes      []int
	UploadedBytes  int64
}

type syncObservation struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Snapshot   syncPerformanceSnapshot
	Err        error
}

type syncPerfRecorder struct {
	mu        sync.Mutex
	startedAt time.Time
	finished  chan syncObservation
}

func newSyncPerfRecorder() *syncPerfRecorder {
	return &syncPerfRecorder{
		finished: make(chan syncObservation, 1),
	}
}

func (r *syncPerfRecorder) OnSyncStart(path string, snapshotSize int64) {
	r.mu.Lock()
	r.startedAt = time.Now()
	r.mu.Unlock()
}

func (r *syncPerfRecorder) OnSyncFinish(snapshot syncPerformanceSnapshot, err error) {
	r.mu.Lock()
	startedAt := r.startedAt
	r.mu.Unlock()

	r.finished <- syncObservation{
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Snapshot:   snapshot,
		Err:        err,
	}
}

func (r *syncPerfRecorder) Wait(tb testing.TB, timeout time.Duration) syncObservation {
	tb.Helper()
	select {
	case obs := <-r.finished:
		return obs
	case <-time.After(timeout):
		tb.Fatalf("timeout waiting for sync observation after %s", timeout)
		return syncObservation{}
	}
}

type fullPathUploadPerfResult struct {
	WriteDuration   time.Duration
	QueueDelay      time.Duration
	SyncDuration    time.Duration
	EndToEnd        time.Duration
	ThroughputMiBS  float64
	PartNumbers     []int
	PartSizes       []int
	UploadedBytes   int64
	SyncObservation syncObservation
}

func runMockUploadPerf(tb testing.TB, fileSize int64, bandwidthBytesPerSec int64, partLatency time.Duration) uploadPerfResult {
	tb.Helper()

	oldLog := driver.Log
	driver.Log = silentLogger{}
	defer func() { driver.Log = oldLog }()

	cipher, err := crypt.NewRcloneCipher("benchmark-password", "")
	if err != nil {
		tb.Fatalf("cipher init failed: %v", err)
	}

	transport := &mockUploadTransport{
		bandwidthBytesPerSec: bandwidthBytesPerSec,
		partLatency:          partLatency,
		controlLatency:       time.Millisecond,
	}
	d := driver.NewQuarkDriver("mock-cookie")
	d.SetClient(&http.Client{Transport: transport})

	cacheDir := tb.TempDir()
	cm, err := cache.NewCacheManager(cacheDir, filepath.Join(cacheDir, "qrypt_perf.db"), 1<<30)
	if err != nil {
		tb.Fatalf("cache init failed: %v", err)
	}
	defer cm.Close()

	fs := NewQryptFS(d, cm, "root", "", cipher)

	n := &node{
		fid:       "local_perf_file",
		parentFid: "root",
		name:      "bench.bin",
		size:      fileSize,
		isDirty:   true,
		mtime:     time.Unix(1, 0),
	}
	localPath, err := fs.staging.Create(n.fid)
	if err != nil {
		tb.Fatalf("staging create failed: %v", err)
	}
	n.localPath = localPath

	totalBlocks := (fileSize + crypt.BlockDataSize - 1) / crypt.BlockDataSize
	for i := int64(0); i < totalBlocks; i++ {
		chunkSize := int(crypt.BlockDataSize)
		remaining := fileSize - i*crypt.BlockDataSize
		if remaining < int64(chunkSize) {
			chunkSize = int(remaining)
		}
		chunk := bytes.Repeat([]byte{byte(i % 251)}, chunkSize)
		if _, err := fs.staging.WriteAt(localPath, chunk, i*crypt.BlockDataSize); err != nil {
			tb.Fatalf("staging write failed: %v", err)
		}
	}

	start := time.Now()
	if err := fs.syncFile("/bench.bin", n); err != nil {
		tb.Fatalf("syncFile failed: %v", err)
	}
	duration := time.Since(start)

	if n.isDirty {
		tb.Fatalf("expected node to be clean after sync")
	}

	partNumbers, partSizes, uploadedBytes := transport.stats()
	return uploadPerfResult{
		Duration:       duration,
		ThroughputMiBS: bytesPerSecondToMiBS(float64(fileSize) / duration.Seconds()),
		PartNumbers:    partNumbers,
		PartSizes:      partSizes,
		UploadedBytes:  uploadedBytes,
	}
}

func runMockFullPathUploadPerf(tb testing.TB, fileSize int64, writeChunkSize int, bandwidthBytesPerSec int64, partLatency time.Duration) fullPathUploadPerfResult {
	tb.Helper()

	oldLog := driver.Log
	driver.Log = silentLogger{}
	defer func() { driver.Log = oldLog }()

	cipher, err := crypt.NewRcloneCipher("benchmark-password", "")
	if err != nil {
		tb.Fatalf("cipher init failed: %v", err)
	}

	cacheDir := tb.TempDir()
	cm, err := cache.NewCacheManager(cacheDir, filepath.Join(cacheDir, "qrypt_perf.db"), 1<<30)
	if err != nil {
		tb.Fatalf("cache init failed: %v", err)
	}
	defer cm.Close()

	transport := &mockUploadTransport{
		bandwidthBytesPerSec: bandwidthBytesPerSec,
		partLatency:          partLatency,
		controlLatency:       time.Millisecond,
	}
	d := driver.NewQuarkDriver("mock-cookie")
	d.SetClient(&http.Client{Transport: transport})

	fs := NewQryptFS(d, cm, "root", "", cipher)
	recorder := newSyncPerfRecorder()
	fs.syncObserver = recorder

	path := "/bench.bin"
	errc, fh := fs.Create(path, 0, 0644)
	if errc != 0 {
		tb.Fatalf("create failed with errc=%d", errc)
	}

	chunk := bytes.Repeat([]byte("q"), writeChunkSize)
	writeStart := time.Now()
	var offset int64
	for offset < fileSize {
		payload := chunk
		remaining := fileSize - offset
		if remaining < int64(len(payload)) {
			payload = payload[:remaining]
		}
		written := fs.Write(path, payload, offset, fh)
		if written != len(payload) {
			tb.Fatalf("write returned %d, want %d", written, len(payload))
		}
		offset += int64(written)
	}
	writeDuration := time.Since(writeStart)

	flushStart := time.Now()
	if errc := fs.Release(path, fh); errc != 0 {
		tb.Fatalf("release failed with errc=%d", errc)
	}

	obs := recorder.Wait(tb, 15*time.Second)
	if obs.Err != nil {
		tb.Fatalf("background sync failed: %v", obs.Err)
	}
	if v, ok := fs.nodes.Load(path); ok {
		n := v.(*node)
		n.mu.RLock()
		isDirty := n.isDirty
		n.mu.RUnlock()
		if isDirty {
			tb.Fatalf("expected node to be clean after full path upload")
		}
	}

	partNumbers, partSizes, uploadedBytes := transport.stats()
	queueDelay := time.Duration(0)
	if !obs.StartedAt.IsZero() {
		queueDelay = obs.StartedAt.Sub(flushStart)
	}
	endToEnd := obs.FinishedAt.Sub(writeStart)
	return fullPathUploadPerfResult{
		WriteDuration:   writeDuration,
		QueueDelay:      queueDelay,
		SyncDuration:    obs.Snapshot.TotalDuration,
		EndToEnd:        endToEnd,
		ThroughputMiBS:  bytesPerSecondToMiBS(float64(fileSize) / endToEnd.Seconds()),
		PartNumbers:     partNumbers,
		PartSizes:       partSizes,
		UploadedBytes:   uploadedBytes,
		SyncObservation: obs,
	}
}

func TestSyncFileUploadPerfMock_5MiB(t *testing.T) {
	result := runMockUploadPerf(t, 5*1024*1024, 128*1024*1024, 2*time.Millisecond)

	if len(result.PartNumbers) != 1 {
		t.Fatalf("expected 1 uploaded part, got %d", len(result.PartNumbers))
	}
	if result.PartNumbers[0] != 1 {
		t.Fatalf("expected part number 1, got %v", result.PartNumbers)
	}
	if result.ThroughputMiBS <= 0 {
		t.Fatalf("expected positive throughput, got %.2f MiB/s", result.ThroughputMiBS)
	}

	t.Logf("5MiB upload: duration=%s throughput=%.2f MiB/s uploaded=%d bytes", result.Duration, result.ThroughputMiBS, result.UploadedBytes)
}

func TestSyncFileUploadPerfMock_9MiBMultipart(t *testing.T) {
	fileSize := int64(9 * 1024 * 1024)
	result := runMockUploadPerf(t, fileSize, 128*1024*1024, 2*time.Millisecond)

	expectedParts := expectedMockParts(fileSize)
	if len(result.PartNumbers) != expectedParts {
		t.Fatalf("expected %d uploaded parts, got %d", expectedParts, len(result.PartNumbers))
	}
	for i, partNumber := range result.PartNumbers {
		if partNumber != i+1 {
			t.Fatalf("expected sequential part numbers, got %v", result.PartNumbers)
		}
	}
	if len(result.PartSizes) != expectedParts {
		t.Fatalf("expected %d part sizes, got %d", expectedParts, len(result.PartSizes))
	}
	if result.PartSizes[0] <= result.PartSizes[len(result.PartSizes)-1] {
		t.Fatalf("expected first part to be larger than last part, got %v", result.PartSizes)
	}

	t.Logf("9MiB multipart upload: duration=%s throughput=%.2f MiB/s parts=%v sizes=%v", result.Duration, result.ThroughputMiBS, result.PartNumbers, result.PartSizes)
}

func TestUploadFullPathPerfMock_5MiB(t *testing.T) {
	result := runMockFullPathUploadPerf(t, 5*1024*1024, 64*1024, 128*1024*1024, 2*time.Millisecond)

	if result.ThroughputMiBS <= 0 {
		t.Fatalf("expected positive end-to-end throughput, got %.2f MiB/s", result.ThroughputMiBS)
	}
	if len(result.PartNumbers) != 1 {
		t.Fatalf("expected 1 uploaded part, got %d", len(result.PartNumbers))
	}
	if result.SyncObservation.Snapshot.PartCount != 1 {
		t.Fatalf("expected 1 part in sync snapshot, got %d", result.SyncObservation.Snapshot.PartCount)
	}

	t.Logf("full path 5MiB: write=%s queue=%s sync=%s end_to_end=%s throughput=%.2f MiB/s parts=%v",
		result.WriteDuration, result.QueueDelay, result.SyncDuration, result.EndToEnd, result.ThroughputMiBS, result.PartNumbers)
}

func TestUploadFullPathPerfMock_64MiB(t *testing.T) {
	fileSize := int64(64 * 1024 * 1024)
	result := runMockFullPathUploadPerf(t, fileSize, 64*1024, 128*1024*1024, 2*time.Millisecond)

	expectedParts := expectedMockParts(fileSize)
	if len(result.PartNumbers) != expectedParts {
		t.Fatalf("expected %d uploaded parts, got %d", expectedParts, len(result.PartNumbers))
	}
	if result.SyncObservation.Snapshot.PartCount != expectedParts {
		t.Fatalf("expected %d parts in sync snapshot, got %d", expectedParts, result.SyncObservation.Snapshot.PartCount)
	}

	t.Logf("full path 64MiB: write=%s queue=%s sync=%s end_to_end=%s throughput=%.2f MiB/s upload=%s update_hash=%s commit=%s finish=%s",
		result.WriteDuration,
		result.QueueDelay,
		result.SyncDuration,
		result.EndToEnd,
		result.ThroughputMiBS,
		result.SyncObservation.Snapshot.UploadPartDuration,
		result.SyncObservation.Snapshot.UpdateHashDuration,
		result.SyncObservation.Snapshot.CommitDuration,
		result.SyncObservation.Snapshot.FinishDuration,
	)
}

func BenchmarkSyncFileUploadMock_5MiB(b *testing.B) {
	runUploadBenchmark(b, 5*1024*1024)
}

func BenchmarkSyncFileUploadMock_64MiB(b *testing.B) {
	runUploadBenchmark(b, 64*1024*1024)
}

func BenchmarkUploadFullPathMock_5MiB(b *testing.B) {
	runFullPathUploadBenchmark(b, 5*1024*1024)
}

func BenchmarkUploadFullPathMock_64MiB(b *testing.B) {
	runFullPathUploadBenchmark(b, 64*1024*1024)
}

func runUploadBenchmark(b *testing.B, fileSize int64) {
	b.ReportAllocs()
	b.SetBytes(fileSize)

	var last uploadPerfResult
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		last = runMockUploadPerf(b, fileSize, 128*1024*1024, 2*time.Millisecond)
	}
	b.StopTimer()

	b.ReportMetric(last.ThroughputMiBS, "mib/s")
	b.ReportMetric(float64(len(last.PartNumbers)), "parts/op")
}

func runFullPathUploadBenchmark(b *testing.B, fileSize int64) {
	b.ReportAllocs()
	b.SetBytes(fileSize)

	var last fullPathUploadPerfResult
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		last = runMockFullPathUploadPerf(b, fileSize, 64*1024, 128*1024*1024, 2*time.Millisecond)
	}
	b.StopTimer()

	b.ReportMetric(last.ThroughputMiBS, "mib/s")
	b.ReportMetric(float64(len(last.PartNumbers)), "parts/op")
}

func expectedMockParts(fileSize int64) int {
	cipher, err := crypt.NewRcloneCipher("benchmark-password", "")
	if err != nil {
		panic(err)
	}
	encryptedSize := cipher.EncryptedSize(fileSize)
	if encryptedSize == 0 {
		return 1
	}
	return int(math.Ceil(float64(encryptedSize) / float64(8*1024*1024)))
}

func bytesPerSecondToMiBS(bytesPerSec float64) float64 {
	return bytesPerSec / 1024 / 1024
}

func jsonResponse(v any) *http.Response {
	body, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func emptyResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
}
