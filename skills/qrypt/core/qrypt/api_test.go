package qrypt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type mockCipher struct{}

func (m *mockCipher) EncryptSegment(plain string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

func (m *mockCipher) DecryptSegment(cipher string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cipher)
	if err != nil {
		return cipher, nil
	}
	return string(b), nil
}

func (m *mockCipher) EncryptBlock(plaintext []byte, blockIndex uint64, nonce [24]byte) ([]byte, error) {
	out := make([]byte, len(plaintext)+16)
	copy(out[:16], nonce[:16])
	for i, b := range plaintext {
		out[i+16] = b ^ byte(blockIndex) ^ nonce[i%24]
	}
	return out, nil
}

func (m *mockCipher) DecryptBlock(ciphertext []byte, blockIndex uint64, nonce [24]byte) ([]byte, error) {
	if len(ciphertext) < 16 {
		return nil, io.ErrUnexpectedEOF
	}
	plaintext := make([]byte, len(ciphertext)-16)
	for i := range plaintext {
		plaintext[i] = ciphertext[i+16] ^ byte(blockIndex) ^ nonce[i%24]
	}
	return plaintext, nil
}

func (m *mockCipher) EncryptedSize(plainSize int64) int64 {
	if plainSize <= 0 {
		return int64(drivers.FileHeaderSize)
	}
	blocks := plainSize / drivers.BlockDataSize
	residue := plainSize % drivers.BlockDataSize
	encSize := int64(drivers.FileHeaderSize) + blocks*(drivers.BlockHeaderSize+drivers.BlockDataSize)
	if residue != 0 {
		encSize += drivers.BlockHeaderSize + residue
	}
	return encSize
}

func (m *mockCipher) DecryptedSize(cipherSize int64) (int64, error) {
	if cipherSize <= int64(drivers.FileHeaderSize) {
		return 0, nil
	}
	size := cipherSize - int64(drivers.FileHeaderSize)
	blocks := size / drivers.BlockSize
	residue := size % drivers.BlockSize
	decSize := blocks * drivers.BlockDataSize
	if residue > 0 {
		residue -= drivers.BlockHeaderSize
		if residue <= 0 {
			return 0, io.ErrUnexpectedEOF
		}
		decSize += residue
	}
	return decSize, nil
}

func (m *mockCipher) GenerateRandomNonce() ([24]byte, error) {
	var nonce [24]byte
	_, err := rand.Read(nonce[:])
	return nonce, err
}

type mockEntry struct {
	id       string
	name     string
	isDir    bool
	size     int64
	modTime  time.Time
	parentID string
	children map[string]*mockEntry
	data     []byte
}

type mockDriver struct {
	mu      sync.RWMutex
	root    *mockEntry
	entries map[string]*mockEntry
	seq     int64
}

func newMockDriver() *mockDriver {
	root := &mockEntry{
		id:       "0",
		name:     "",
		isDir:    true,
		children: make(map[string]*mockEntry),
	}
	return &mockDriver{
		root:    root,
		entries: map[string]*mockEntry{"0": root},
		seq:     1,
	}
}

func (m *mockDriver) nextID() string {
	m.seq++
	return "m" + strings.Repeat("0", 15-int(m.seq))
}

func (m *mockDriver) Init(ctx context.Context) error { return nil }
func (m *mockDriver) Drop(ctx context.Context) error { return nil }

func (m *mockDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolvePathLocked(path)
}

func (m *mockDriver) resolvePathLocked(path string) (string, error) {
	path = strings.TrimRight(path, "/")
	parts := strings.Split(path, "/")
	current := m.root
	if path == "" || path == "/" {
		return "0", nil
	}
	cipher := &mockCipher{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		child, ok := current.children[part]
		if !ok {
			encName := cipher.EncryptSegment(part)
			child, ok = current.children[encName]
		}
		if !ok {
			return "", NewErrorf(ErrNotFound, "path not found")
		}
		current = child
	}
	return current.id, nil
}

func (m *mockDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	parent, ok := m.entries[parentID]
	if !ok {
		return nil, NewErrorf(ErrNotFound, "entry not found")
	}
	var result []drivers.Entry
	for _, child := range parent.children {
		result = append(result, drivers.Entry{
			ID: child.id, Name: child.name,
			IsDir: child.isDir, Size: child.size, ModTime: child.modTime,
		})
	}
	return result, nil
}

func (m *mockDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[entry.ID]
	if !ok || e.isDir {
		return nil, NewErrorf(ErrNotFound, "entry not found")
	}
	if offset >= int64(len(e.data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	end := offset + size
	if end > int64(len(e.data)) {
		end = int64(len(e.data))
	}
	return io.NopCloser(bytes.NewReader(e.data[offset:end])), nil
}

func (m *mockDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parent, ok := m.entries[parentID]
	if !ok {
		return drivers.Entry{}, NewErrorf(ErrNotFound, "parent not found")
	}
	if _, exists := parent.children[name]; exists {
		return drivers.Entry{}, NewErrorf(ErrAlreadyExists, "already exists")
	}
	id := m.nextID()
	e := &mockEntry{
		id: id, name: name, isDir: true,
		modTime: time.Now(), parentID: parentID,
		children: make(map[string]*mockEntry),
	}
	parent.children[name] = e
	m.entries[id] = e
	return drivers.Entry{ID: id, Name: name, IsDir: true}, nil
}

func (m *mockDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[entry.ID]
	if !ok {
		return NewErrorf(ErrNotFound, "entry not found")
	}
	srcParent, ok := m.entries[e.parentID]
	if !ok {
		return NewErrorf(ErrNotFound, "src parent not found")
	}
	dstParent, ok := m.entries[dstParentID]
	if !ok {
		return NewErrorf(ErrNotFound, "dst parent not found")
	}
	delete(srcParent.children, e.name)
	e.parentID = dstParentID
	dstParent.children[e.name] = e
	return nil
}

func (m *mockDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[entry.ID]
	if !ok {
		return NewErrorf(ErrNotFound, "entry not found")
	}
	parent, ok := m.entries[e.parentID]
	if !ok {
		return NewErrorf(ErrNotFound, "parent not found")
	}
	delete(parent.children, e.name)
	e.name = newName
	parent.children[newName] = e
	return nil
}

func (m *mockDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[entry.ID]
	if !ok {
		return NewErrorf(ErrNotFound, "entry not found")
	}
	parent, ok := m.entries[e.parentID]
	if !ok {
		return NewErrorf(ErrNotFound, "parent not found")
	}
	delete(parent.children, e.name)
	delete(m.entries, entry.ID)
	return nil
}

func (m *mockDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parent, ok := m.entries[parentID]
	if !ok {
		return drivers.Entry{}, NewErrorf(ErrNotFound, "parent not found")
	}
	data, _ := io.ReadAll(body)
	id := m.nextID()
	e := &mockEntry{
		id: id, name: name, isDir: false,
		size: size, data: data, modTime: time.Now(), parentID: parentID,
	}
	parent.children[name] = e
	m.entries[id] = e
	return drivers.Entry{ID: id, Name: name, Size: size}, nil
}

type mockDriverFactory struct {
	drv *mockDriver
}

func (f *mockDriverFactory) CreateDriver(ctx context.Context, cfg SessionConfig) (drivers.Driver, error) {
	return f.drv, nil
}

type mockDirResolver struct{}

func (mockDirResolver) CacheDir() string  { return os.TempDir() }
func (mockDirResolver) DataDir() string   { return "." }
func (mockDirResolver) ConfigDir() string { return "." }

type mockCredentialStore struct{}

func (mockCredentialStore) Get(key string) (string, error) {
	return "", NewErrorf(ErrNotFound, "credential not found: %s", key)
}
func (mockCredentialStore) Set(key, value string) error { return nil }
func (mockCredentialStore) Delete(key string) error     { return nil }

func newFileAPIWithMock(t *testing.T) *FileAPI {
	t.Helper()
	drv := newMockDriver()
	factory := &mockDriverFactory{drv: drv}
	api, err := NewFileAPI(Options{
		Cipher:        &mockCipher{},
		Dirs:          mockDirResolver{},
		Creds:         mockCredentialStore{},
		DriverFactory: factory,
	})
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func writeTestFile(t *testing.T, api *FileAPI) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	err := api.Push(context.Background(), "", path, "/hello.txt")
	if err != nil {
		t.Fatalf("setup Push: %v", err)
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func TestFileAPI_List(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	entries, err := api.List(context.Background(), "", "/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.DecName)
	}
	if !contains(names, "hello.txt") {
		t.Fatalf("expected hello.txt in entries, got %v", names)
	}
}

func TestFileAPI_Stat_Root(t *testing.T) {
	api := newFileAPIWithMock(t)
	e, err := api.Stat(context.Background(), "", "/")
	if err != nil {
		t.Fatalf("Stat /: %v", err)
	}
	if !e.IsDir {
		t.Fatal("expected root to be a directory")
	}
}

func TestFileAPI_Stat_File(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	e, err := api.Stat(context.Background(), "", "/hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if e.IsDir {
		t.Fatal("expected a file")
	}
	if e.DecName != "hello.txt" {
		t.Fatalf("expected hello.txt, got %s", e.DecName)
	}
}

func TestFileAPI_Stat_NotFound(t *testing.T) {
	api := newFileAPIWithMock(t)
	_, err := api.Stat(context.Background(), "", "/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFileAPI_Mkdir(t *testing.T) {
	api := newFileAPIWithMock(t)
	err := api.Mkdir(context.Background(), "", "/newdir")
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	entries, _ := api.List(context.Background(), "", "/")
	var names []string
	for _, e := range entries {
		names = append(names, e.DecName)
	}
	if !contains(names, "newdir") {
		t.Fatalf("expected newdir in entries, got %v", names)
	}
}

func TestFileAPI_Mkdir_AlreadyExists(t *testing.T) {
	api := newFileAPIWithMock(t)
	err := api.Mkdir(context.Background(), "", "/")
	if err != nil {
		t.Fatalf("Mkdir existing dir should be no-op: %v", err)
	}
}

func TestFileAPI_Move_Rename(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	err := api.Move(context.Background(), "", "/hello.txt", "/renamed.txt")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	_, err = api.Stat(context.Background(), "", "/renamed.txt")
	if err != nil {
		t.Fatalf("Stat renamed: %v", err)
	}
	_, err = api.Stat(context.Background(), "", "/hello.txt")
	if err == nil {
		t.Fatal("expected old path to be gone")
	}
}

func TestFileAPI_Remove(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	err := api.Remove(context.Background(), "", "/hello.txt", false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = api.Stat(context.Background(), "", "/hello.txt")
	if err == nil {
		t.Fatal("expected removed file to be gone")
	}
}

func TestFileAPI_Remove_NonEmptyDir(t *testing.T) {
	api := newFileAPIWithMock(t)
	if err := api.Mkdir(context.Background(), "", "/sub"); err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	childPath := filepath.Join(tmpDir, "child.txt")
	if err := os.WriteFile(childPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := api.Push(context.Background(), "", childPath, "/sub/child.txt"); err != nil {
		t.Fatal(err)
	}
	err := api.Remove(context.Background(), "", "/sub", false)
	if err == nil {
		t.Fatal("expected error for non-empty dir without recursive")
	}
}

func TestFileAPI_Remove_Recursive(t *testing.T) {
	api := newFileAPIWithMock(t)
	subDir := t.TempDir()
	if err := api.Mkdir(context.Background(), "", "/sub"); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(subDir, "child.txt")
	if err := os.WriteFile(childPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := api.Push(context.Background(), "", childPath, "/sub/child.txt"); err != nil {
		t.Fatal(err)
	}
	err := api.Remove(context.Background(), "", "/sub", true)
	if err != nil {
		t.Fatalf("Remove recursive: %v", err)
	}
	_, err = api.Stat(context.Background(), "", "/sub")
	if err == nil {
		t.Fatal("expected removed dir to be gone")
	}
}

func TestFileAPI_PushAndRead(t *testing.T) {
	api := newFileAPIWithMock(t)
	content := "hello world test content"
	dir := t.TempDir()
	filePath := filepath.Join(dir, "newfile.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	err := api.Push(context.Background(), "", filePath, "/newfile.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	rc, err := api.Read(context.Background(), "", "/newfile.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != content {
		t.Fatalf("round-trip content mismatch: got %q, want %q", string(data), content)
	}
}

func TestFileAPI_Read_NotFound(t *testing.T) {
	api := newFileAPIWithMock(t)
	_, err := api.Read(context.Background(), "", "/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFileAPI_PushFromFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("push content"), 0644); err != nil {
		t.Fatal(err)
	}

	api := newFileAPIWithMock(t)
	err := api.Push(context.Background(), "", filePath, "/pushed.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	entries, _ := api.List(context.Background(), "", "/")
	var names []string
	for _, e := range entries {
		names = append(names, e.DecName)
	}
	if !contains(names, "pushed.txt") {
		t.Fatalf("expected pushed.txt in entries, got %v", names)
	}
}

func TestFileAPI_Find(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	otherDir := t.TempDir()
	otherPath := filepath.Join(otherDir, "other.txt")
	if err := os.WriteFile(otherPath, []byte("zzz"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := api.Push(context.Background(), "", otherPath, "/other.txt"); err != nil {
		t.Fatal(err)
	}

	results, err := api.Find(context.Background(), "", "/", "hello", -1, 0, false)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one match")
	}
	matchFound := false
	for _, r := range results {
		if strings.Contains(r.DecName, "hello") {
			matchFound = true
			break
		}
	}
	if !matchFound {
		t.Fatal("expected an entry matching 'hello'")
	}
}

func TestFileAPI_Find_NoMatch(t *testing.T) {
	api := newFileAPIWithMock(t)
	writeTestFile(t, api)
	results, err := api.Find(context.Background(), "", "/", "zzz_nonexistent", -1, 0, false)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestFileAPI_Shutdown(t *testing.T) {
	api := newFileAPIWithMock(t)
	api.Shutdown()
}

func TestNewFileAPI_MissingRequired(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"no cipher", Options{Dirs: mockDirResolver{}, Creds: mockCredentialStore{}, DriverFactory: &mockDriverFactory{}}},
		{"no dirs", Options{Cipher: &mockCipher{}, Creds: mockCredentialStore{}, DriverFactory: &mockDriverFactory{}}},
		{"no creds", Options{Cipher: &mockCipher{}, Dirs: mockDirResolver{}, DriverFactory: &mockDriverFactory{}}},
		{"no factory", Options{Cipher: &mockCipher{}, Dirs: mockDirResolver{}, Creds: mockCredentialStore{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileAPI(tt.opts)
			if err == nil {
				t.Fatal("expected error for missing required field")
			}
		})
	}
}

func TestSessionManager_AcquireRelease(t *testing.T) {
	sm := NewSessionManager(&mockDriverFactory{drv: newMockDriver()})
	key := SessionKey{Type: "mock", CredKey: "test"}
	cfg := SessionConfig{Type: "mock"}

	s, err := sm.Acquire(context.Background(), key, cfg)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if s.RefCount != 1 {
		t.Fatalf("expected refcount 1, got %d", s.RefCount)
	}
	s2, err := sm.Acquire(context.Background(), key, cfg)
	if err != nil {
		t.Fatalf("Acquire second: %v", err)
	}
	if s2.RefCount != 2 {
		t.Fatalf("expected refcount 2, got %d", s2.RefCount)
	}
	sm.Release(context.Background(), key)
	sm.Release(context.Background(), key)
}

func TestOrchestrator_SubmitAndShutdown(t *testing.T) {
	eb := NewEventBus()
	ph := NewProgressHub(eb)
	rl := NewRateLimiter(0)
	o := NewOrchestrator(2, rl, ph)

	var mu sync.Mutex
	var results []int
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		i := i
		wg.Add(1)
		o.Submit(func(ctx context.Context) error {
			mu.Lock()
			results = append(results, i)
			mu.Unlock()
			wg.Done()
			return nil
		})
	}
	wg.Wait()
	o.Shutdown()
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

func TestEventBus(t *testing.T) {
	eb := NewEventBus()

	ch1 := eb.Subscribe("sub1")
	ch2 := eb.Subscribe("sub2")

	eb.Publish(&Event{Type: EventSyncProgress})

	select {
	case evt := <-ch1:
		if evt.Type != EventSyncProgress {
			t.Fatalf("expected EventSyncProgress, got %v", evt.Type)
		}
	default:
		t.Fatal("expected event on ch1")
	}

	select {
	case evt := <-ch2:
		if evt.Type != EventSyncProgress {
			t.Fatalf("expected EventSyncProgress, got %v", evt.Type)
		}
	default:
		t.Fatal("expected event on ch2")
	}

	eb.Unsubscribe("sub1")
	_, open := <-ch1
	if open {
		t.Fatal("expected ch1 to be closed after Unsubscribe")
	}
}

func TestProgressHub(t *testing.T) {
	eb := NewEventBus()
	ph := NewProgressHub(eb)

	ph.Publish(&ProgressEntry{TaskID: "t1", Direction: "push", File: "f1", State: "started"})
	ph.Publish(&ProgressEntry{TaskID: "t1", Direction: "push", File: "f1", State: "completed"})

	active := ph.Active()
	if len(active) != 0 {
		t.Fatalf("expected 0 active after completed, got %d", len(active))
	}

	ph.Publish(&ProgressEntry{TaskID: "t2", Direction: "pull", File: "f2", State: "uploading"})
	active = ph.Active()
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	rl := NewRateLimiter(0)
	err := rl.Wait(context.Background(), 1000)
	if err != nil {
		t.Fatalf("Wait on disabled limiter: %v", err)
	}
}

func TestRateLimiter_WaitFor(t *testing.T) {
	rl := NewRateLimiter(100_000)
	err := rl.WaitFor(context.Background(), 1000, time.Second)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
}

func TestErrorKind(t *testing.T) {
	err := NewErrorf(ErrNotFound, "file not found")
	if err.Kind != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err.Kind)
	}
}

func TestError_Wrap(t *testing.T) {
	cause := NewErrorf(ErrNotFound, "root cause")
	wrapped := WrapError(ErrNetwork, "network error", cause)
	if wrapped.Cause != cause {
		t.Fatal("expected Cause to be preserved")
	}
	if wrapped.Kind != ErrNetwork {
		t.Fatalf("expected ErrNetwork, got %v", wrapped.Kind)
	}
}

func TestError_Is(t *testing.T) {
	e1 := NewErrorf(ErrNotFound, "not found")
	e2 := NewErrorf(ErrNotFound, "also not found")
	if !e1.Is(e2) {
		t.Fatal("same Kind should match via Is")
	}
	e3 := NewErrorf(ErrInternal, "internal")
	if e1.Is(e3) {
		t.Fatal("different Kind should not match")
	}
}

func TestCacheInvalidator(t *testing.T) {
	var mu sync.Mutex
	invalidated := make([]string, 0)
	hooks := &mockCacheHooks{
		fn: func(mount, dir string) error {
			mu.Lock()
			invalidated = append(invalidated, mount+":"+dir)
			mu.Unlock()
			return nil
		},
	}

	eb := NewEventBus()
	_ = NewCacheInvalidator(hooks, eb)

	eb.Publish(&Event{
		Type:     EventSyncCompleted,
		Progress: &ProgressEntry{Mount: "m1", File: "/dir/file.txt"},
	})
	eb.Publish(&Event{
		Type:     EventSyncProgress,
		Progress: &ProgressEntry{Mount: "m2", File: "/other/file.txt"},
	})

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	count := len(invalidated)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 invalidation, got %d", count)
	}
	mu.Lock()
	val := invalidated[0]
	mu.Unlock()
	if val != "m1:/dir" {
		t.Fatalf("expected m1:/dir, got %s", val)
	}
}

type mockCacheHooks struct {
	fn func(mount, dir string) error
}

func (h *mockCacheHooks) InvalidateDirCache(mount, dir string) error {
	return h.fn(mount, dir)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cp := &mockCipher{}
	plaintext := []byte("hello world this is a test message that spans multiple blocks")
	nonce, _ := cp.GenerateRandomNonce()

	er := NewEncryptingReader(bytes.NewReader(plaintext), cp, nonce, int64(len(plaintext)))
	encData, err := io.ReadAll(er)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if len(encData) < drivers.FileHeaderSize {
		t.Fatal("encrypted data too short")
	}

	dr := NewDecryptingReader(bytes.NewReader(encData[drivers.FileHeaderSize:]), cp, nonce)
	decrypted, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}
