package fusefs

import (
	"bytes"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/internal/mockdrive"
)

type e2eSuite struct {
	t  *testing.T
	fs *QryptFS
}

func newE2E(t *testing.T) *e2eSuite {
	return newE2EWithOpts(t, FSOptions{MaxRetries: 3, ConcurrentUploads: 3})
}

func newE2EWithOpts(t *testing.T, opts FSOptions) *e2eSuite {
	t.Helper()
	logger, _ := logging.New("off", "", nil)
	logging.L = logger

	cacheDir := t.TempDir()
	cm, err := qrypt.NewCacheManager(cacheDir, 100*1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	memCache, _ := lru.New[string, []byte](100)
	cph, _ := cipher.NewRcloneCipher("e2etest", "")

	drv := mockdrive.NewDriver()
	fs := NewFS(drv, cph, cm, "0", opts)
	fs.memCache = memCache

	fs.storeNode("/", &Node{
		fid: "0", name: "", currentPath: "/", isFolder: true, source: "remote",
	})
	return &e2eSuite{t: t, fs: fs}
}

func (s *e2eSuite) hasTimer(path string) bool {
	s.fs.syncDelayMu.Lock()
	defer s.fs.syncDelayMu.Unlock()
	_, ok := s.fs.syncTimers[path]
	return ok
}

func (s *e2eSuite) timerCount() int {
	s.fs.syncDelayMu.Lock()
	defer s.fs.syncDelayMu.Unlock()
	return len(s.fs.syncTimers)
}

func (s *e2eSuite) waitForTimer(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !s.hasTimer(path) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timer for %s did not fire within %v", path, timeout)
}

func (s *e2eSuite) waitForUpload(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n, errc := s.fs.lookup(path)
		if errc != 0 {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		n.mu.RLock()
		fid := n.fid
		n.mu.RUnlock()
		if !strings.HasPrefix(fid, "local_") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("upload not completed within %v for %s", timeout, path)
}

func (s *e2eSuite) lookup(path string) *Node {
	n, errc := s.fs.lookup(path)
	if errc != 0 {
		s.t.Fatalf("lookup(%s): errc=%d", path, errc)
	}
	return n
}

func (s *e2eSuite) mustMkdir(path string) {
	if errc := s.fs.Mkdir(path, 0o755); errc != 0 {
		s.t.Fatalf("Mkdir(%s) returned %d", path, errc)
	}
}

func (s *e2eSuite) mustCreate(path string) uint64 {
	errc, fh := s.fs.Create(path, 0, 0o644)
	if errc != 0 {
		s.t.Fatalf("Create(%s): errc=%d", path, errc)
	}
	return fh
}

func (s *e2eSuite) mustWrite(path string, data []byte, ofst int64) int {
	n := s.fs.Write(path, data, ofst, 0)
	if n != len(data) {
		s.t.Fatalf("Write(%s): wrote %d, want %d", path, n, len(data))
	}
	return n
}

func (s *e2eSuite) mustRead(path string, size int, ofst int64) []byte {
	buf := make([]byte, size)
	n := s.fs.Read(path, buf, ofst, 0)
	return buf[:n]
}

func (s *e2eSuite) mustReaddir(path string) []string {
	var names []string
	fill := func(name string, _ *fuse.Stat_t, _ int64) bool {
		names = append(names, name)
		return true
	}
	if errc := s.fs.Readdir(path, fill, 0, 0); errc != 0 {
		s.t.Fatalf("Readdir(%s): errc=%d", path, errc)
	}
	return names
}

func (s *e2eSuite) mustGetattr(path string) fuse.Stat_t {
	var st fuse.Stat_t
	if errc := s.fs.Getattr(path, &st, 0); errc != 0 {
		s.t.Fatalf("Getattr(%s): errc=%d", path, errc)
	}
	return st
}

func (s *e2eSuite) mustUnlink(path string) {
	if errc := s.fs.Unlink(path); errc != 0 {
		s.t.Fatalf("Unlink(%s): errc=%d", path, errc)
	}
}

func (s *e2eSuite) writeFile(path string, data []byte) {
	s.mustCreate(path)
	s.mustWrite(path, data, 0)
	s.fs.Release(path, 0)
}

// ============================================================
// E2E Tests
// ============================================================

func TestE2E_FileCreateWriteRead(t *testing.T) {
	s := newE2E(t)

	fh := s.mustCreate("/hello.txt")
	s.mustWrite("/hello.txt", []byte("hello world"), 0)
	s.fs.Release("/hello.txt", fh)

	got := s.mustRead("/hello.txt", 100, 0)
	if string(got) != "hello world" {
		t.Errorf("got %q, want %q", string(got), "hello world")
	}
}

func TestE2E_FileMultipleWrites(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/append.txt")
	s.mustWrite("/append.txt", []byte("AAA"), 0)
	s.mustWrite("/append.txt", []byte("BBB"), 3)
	s.mustWrite("/append.txt", []byte("CCC"), 6)
	s.fs.Release("/append.txt", 0)

	got := s.mustRead("/append.txt", 20, 0)
	if string(got) != "AAABBBCCC" {
		t.Errorf("got %q, want %q", string(got), "AAABBBCCC")
	}
}

func TestE2E_FileOverwriteMidRegion(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/overwrite.txt")
	s.mustWrite("/overwrite.txt", []byte("xxxxxxxxxxxx"), 0)
	s.mustWrite("/overwrite.txt", []byte("OOO"), 2)
	s.fs.Release("/overwrite.txt", 0)

	got := s.mustRead("/overwrite.txt", 12, 0)
	if string(got) != "xxOOOxxxxxxx" {
		t.Errorf("got %q, want %q", string(got), "xxOOOxxxxxxx")
	}
}

func TestE2E_ReadAtOffset(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/offset.txt")
	s.mustWrite("/offset.txt", []byte("0123456789ABCDEF"), 0)
	s.fs.Release("/offset.txt", 0)

	got := s.mustRead("/offset.txt", 4, 5)
	if string(got) != "5678" {
		t.Errorf("got %q, want %q", string(got), "5678")
	}
}

func TestE2E_ReadBeyondFileSize(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/short.txt")
	s.mustWrite("/short.txt", []byte("short"), 0)
	s.fs.Release("/short.txt", 0)

	got := s.mustRead("/short.txt", 100, 0)
	if string(got) != "short" {
		t.Errorf("got %q, want %q", string(got), "short")
	}
}

func TestE2E_ReadPastEOF(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/eof.txt")
	s.mustWrite("/eof.txt", []byte("data"), 0)
	s.fs.Release("/eof.txt", 0)

	got := s.mustRead("/eof.txt", 10, 100)
	if len(got) != 0 {
		t.Errorf("expected empty read past EOF, got %d bytes", len(got))
	}
}

func TestE2E_EmptyFile(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/empty.txt")
	s.fs.Release("/empty.txt", 0)

	st := s.mustGetattr("/empty.txt")
	if st.Size != 0 {
		t.Errorf("expected size 0 for empty file, got %d", st.Size)
	}

	got := s.mustRead("/empty.txt", 10, 0)
	if len(got) != 0 {
		t.Errorf("expected empty read, got %d bytes", len(got))
	}
}

func TestE2E_DirectoryCreateAndList(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/mydir")

	st := s.mustGetattr("/mydir")
	if st.Mode&fuse.S_IFDIR == 0 {
		t.Error("expected directory mode")
	}

	entries := s.mustReaddir("/mydir")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (., ..), got %v", entries)
	}
}

func TestE2E_DirectoryWithFiles(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/parent")
	s.writeFile("/parent/a.txt", []byte("aaa"))
	s.writeFile("/parent/b.txt", []byte("bbb"))

	entries := s.mustReaddir("/parent")
	names := make(map[string]bool)
	for _, e := range entries {
		names[e] = true
	}
	for _, want := range []string{".", "..", "a.txt", "b.txt"} {
		if !names[want] {
			t.Errorf("missing entry %q in %v", want, entries)
		}
	}
}

func TestE2E_FileDelete(t *testing.T) {
	s := newE2E(t)
	s.writeFile("/delete_me.txt", []byte("bye"))

	s.mustUnlink("/delete_me.txt")

	if _, ok := s.fs.nodes.Load("/delete_me.txt"); ok {
		t.Error("node should be deleted after Unlink")
	}
}

func TestE2E_Rename(t *testing.T) {
	s := newE2E(t)
	s.writeFile("/old.txt", []byte("rename me"))

	errc := s.fs.Rename("/old.txt", "/new.txt")
	if errc != 0 {
		t.Fatalf("Rename: errc=%d", errc)
	}

	if _, ok := s.fs.nodes.Load("/old.txt"); ok {
		t.Error("old path should not exist after rename")
	}
	got := s.mustRead("/new.txt", 20, 0)
	if string(got) != "rename me" {
		t.Errorf("got %q, want %q", string(got), "rename me")
	}
}

func TestE2E_RenameDir(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/olddir")
	s.writeFile("/olddir/f.txt", []byte("nested"))

	errc := s.fs.Rename("/olddir", "/newdir")
	if errc != 0 {
		t.Fatalf("Rename dir: errc=%d", errc)
	}

	if _, ok := s.fs.nodes.Load("/olddir"); ok {
		t.Error("old dir path should not exist")
	}
	got := s.mustRead("/newdir/f.txt", 20, 0)
	if string(got) != "nested" {
		t.Errorf("nested file content mismatch: %q", string(got))
	}
}

func TestE2E_RenameCrossDir(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/dir1")
	s.mustMkdir("/dir2")
	s.writeFile("/dir1/file.txt", []byte("cross-dir move"))

	errc := s.fs.Rename("/dir1/file.txt", "/dir2/moved.txt")
	if errc != 0 {
		t.Fatalf("Rename cross-dir: errc=%d", errc)
	}

	if _, ok := s.fs.nodes.Load("/dir1/file.txt"); ok {
		t.Error("old path should not exist after cross-dir move")
	}
	got := s.mustRead("/dir2/moved.txt", 20, 0)
	if string(got) != "cross-dir move" {
		t.Errorf("got %q, want %q", string(got), "cross-dir move")
	}
}

func TestE2E_ChineseFilename(t *testing.T) {
	s := newE2E(t)

	// Create file with Chinese name
	s.writeFile("/中文文件.txt", []byte("chinese filename content"))
	got := s.mustRead("/中文文件.txt", 30, 0)
	if string(got) != "chinese filename content" {
		t.Errorf("got %q, want %q", string(got), "chinese filename content")
	}

	errc := s.fs.Rename("/中文文件.txt", "/重命名文件.txt")
	if errc != 0 {
		t.Fatalf("Rename Chinese: errc=%d", errc)
	}
	if _, ok := s.fs.nodes.Load("/中文文件.txt"); ok {
		t.Error("old Chinese name should not exist after rename")
	}
	got2 := s.mustRead("/重命名文件.txt", 30, 0)
	if string(got2) != "chinese filename content" {
		t.Errorf("got %q, want %q", string(got2), "chinese filename content")
	}
}

func TestE2E_UnlinkFile(t *testing.T) {
	s := newE2E(t)
	s.writeFile("/delete-me.txt", []byte("will be deleted"))

	errc := s.fs.Unlink("/delete-me.txt")
	if errc != 0 {
		t.Fatalf("Unlink: errc=%d", errc)
	}
	if _, ok := s.fs.nodes.Load("/delete-me.txt"); ok {
		t.Error("node should not exist after unlink")
	}
	// Readdir on root should not include the deleted file
	found := false
	s.fs.Readdir("/", func(name string, st *fuse.Stat_t, ofst int64) bool {
		if name == "delete-me.txt" {
			found = true
		}
		return true
	}, 0, 0)
	if found {
		t.Error("deleted file should not appear in readdir")
	}
}

func TestE2E_Truncate(t *testing.T) {
	s := newE2E(t)
	s.writeFile("/trunc.txt", []byte("hello world truncate"))

	errc := s.fs.Truncate("/trunc.txt", 5, 0)
	if errc != 0 {
		t.Fatalf("Truncate: errc=%d", errc)
	}

	st := s.mustGetattr("/trunc.txt")
	if st.Size != 5 {
		t.Errorf("expected size 5 after truncate, got %d", st.Size)
	}
	got := s.mustRead("/trunc.txt", 10, 0)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", string(got), "hello")
	}
}

func TestE2E_LargeFileMultiChunk(t *testing.T) {
	s := newE2E(t)

	const dataSize = 256 * 1024
	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte(i % 251)
	}

	s.mustCreate("/large.bin")
	s.mustWrite("/large.bin", data, 0)
	s.fs.Release("/large.bin", 0)

	got := s.mustRead("/large.bin", dataSize, 0)
	if !bytes.Equal(got, data) {
		t.Errorf("large file mismatch: read %d bytes, want %d", len(got), dataSize)
	}

	check := func(offset, length int) {
		got := s.mustRead("/large.bin", length, int64(offset))
		want := data[offset : offset+length]
		if !bytes.Equal(got, want) {
			t.Errorf("offset %d: mismatch", offset)
		}
	}
	check(0, 10)
	check(65530, 20)
	check(131000, 100)
	check(200000, 50)
}

func TestE2E_SequentialReadsDetectPattern(t *testing.T) {
	s := newE2E(t)

	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i)
	}
	s.mustCreate("/seq.bin")
	s.mustWrite("/seq.bin", data, 0)
	s.fs.Release("/seq.bin", 0)

	s.mustRead("/seq.bin", 65536, 0)
	s.mustRead("/seq.bin", 65536, 65536)
	s.mustRead("/seq.bin", 65536, 131072)

	got := s.mustRead("/seq.bin", 100, 65536)
	if !bytes.Equal(got, data[65536:65636]) {
		t.Error("sequential read data mismatch")
	}
}

func TestE2E_AccessAndTimes(t *testing.T) {
	s := newE2E(t)

	errc := s.fs.Access("/", 0)
	if errc != 0 {
		t.Errorf("Access root: errc=%d", errc)
	}

	errc = s.fs.Access("/nonexistent", 0)
	if errc == 0 {
		t.Error("expected error for nonexistent Access")
	}
}

func TestE2E_Chmod(t *testing.T) {
	s := newE2E(t)

	errc := s.fs.Chmod("/", 0o755)
	if errc != 0 {
		t.Errorf("Chmod: errc=%d", errc)
	}

	errc = s.fs.Chmod("/nonexistent", 0o644)
	if errc == 0 {
		t.Error("expected error for nonexistent Chmod")
	}
}

func TestE2E_Utimens(t *testing.T) {
	s := newE2E(t)
	now := fuse.NewTimespec(time.Now())
	errc := s.fs.Utimens("/", []fuse.Timespec{now, now})
	if errc != 0 {
		t.Errorf("Utimens: errc=%d", errc)
	}
}

func TestE2E_WritebackFlushConsistency(t *testing.T) {
	s := newE2E(t)

	fh := s.mustCreate("/writeback.txt")
	s.mustWrite("/writeback.txt", []byte("writeback data"), 0)
	st := s.mustGetattr("/writeback.txt")
	if st.Size == 0 {
		t.Error("expected non-zero size before Release")
	}
	s.fs.Release("/writeback.txt", fh)

	// After Release, Read should return the same data via staging
	got := s.mustRead("/writeback.txt", 20, 0)
	if string(got) != "writeback data" {
		t.Errorf("got %q, want %q", string(got), "writeback data")
	}
}

func TestE2E_ConcurrentReadsSameFile(t *testing.T) {
	s := newE2E(t)

	data := make([]byte, 128*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	s.mustCreate("/concurrent_read.bin")
	s.mustWrite("/concurrent_read.bin", data, 0)
	s.fs.Release("/concurrent_read.bin", 0)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			got := s.mustRead("/concurrent_read.bin", len(data), 0)
			if !bytes.Equal(got, data) {
				s.t.Errorf("concurrent reader %d: data mismatch", id)
			}
		}(i)
	}
	wg.Wait()
}

func TestE2E_ConcurrentReadsDifferentOffsets(t *testing.T) {
	s := newE2E(t)

	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i % 251)
	}
	s.mustCreate("/concurrent_offset.bin")
	s.mustWrite("/concurrent_offset.bin", data, 0)
	s.fs.Release("/concurrent_offset.bin", 0)

	var wg sync.WaitGroup
	offsets := []int{0, 65536, 131072, 200000, 250000}
	lengths := []int{100, 200, 50, 300, 150}
	for i := 0; i < len(offsets); i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			off := offsets[idx]
			ln := lengths[idx]
			if off+ln > len(data) {
				ln = len(data) - off
			}
			got := s.mustRead("/concurrent_offset.bin", ln, int64(off))
			want := data[off : off+ln]
			if !bytes.Equal(got, want) {
				s.t.Errorf("offset %d: data mismatch", off)
			}
		}(i)
	}
	wg.Wait()
}

func TestE2E_ConcurrentCreateDifferentFiles(t *testing.T) {
	s := newE2E(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := "/concurrent_file_" + string(rune('A'+id)) + ".txt"
			s.mustCreate(path)
			data := []byte{byte('A' + id)}
			s.mustWrite(path, data, 0)
			s.fs.Release(path, 0)

			got := s.mustRead(path, 10, 0)
			if len(got) == 0 || got[0] != byte('A'+id) {
				s.t.Errorf("file %s: content mismatch", path)
			}
		}(i)
	}
	wg.Wait()
}

func TestE2E_ChunkBoundaryRead(t *testing.T) {
	s := newE2E(t)

	blockSize := cipher.BlockDataSize
	data := make([]byte, blockSize*3)
	for i := range data {
		data[i] = byte(i % 256)
	}
	s.mustCreate("/chunk_boundary.bin")
	s.mustWrite("/chunk_boundary.bin", data, 0)
	s.fs.Release("/chunk_boundary.bin", 0)

	got := s.mustRead("/chunk_boundary.bin", blockSize, 0)
	if !bytes.Equal(got, data[:blockSize]) {
		t.Error("first chunk mismatch")
	}

	crossData := data[blockSize-100 : blockSize+100]
	got = s.mustRead("/chunk_boundary.bin", 200, int64(blockSize-100))
	if !bytes.Equal(got, crossData) {
		t.Error("cross-chunk read mismatch")
	}

	midOffset := blockSize + blockSize/2
	got = s.mustRead("/chunk_boundary.bin", blockSize, int64(midOffset))
	want := data[midOffset : midOffset+blockSize]
	if !bytes.Equal(got, want) {
		t.Error("mid-chunk offset read mismatch")
	}
}

func TestE2E_RandomAccessPattern(t *testing.T) {
	s := newE2E(t)

	blockSize := 4096
	totalSize := blockSize * 50
	data := make([]byte, totalSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	s.mustCreate("/random_access.bin")
	s.mustWrite("/random_access.bin", data, 0)
	s.fs.Release("/random_access.bin", 0)

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 20; i++ {
		offset := rng.Intn(totalSize - blockSize)
		got := s.mustRead("/random_access.bin", blockSize, int64(offset))
		want := data[offset : offset+blockSize]
		if !bytes.Equal(got, want) {
			t.Errorf("random access offset %d: mismatch", offset)
		}
	}
}

func TestE2E_Statfs(t *testing.T) {
	s := newE2E(t)
	var st fuse.Statfs_t
	errc := s.fs.Statfs("/", &st)
	if errc != 0 {
		t.Fatalf("Statfs: errc=%d", errc)
	}
	if st.Bsize != 4096 {
		t.Errorf("expected Bsize=4096, got %d", st.Bsize)
	}
	if st.Frsize != 4096 {
		t.Errorf("expected Frsize=4096, got %d", st.Frsize)
	}
}

func TestE2E_WriteThenStat(t *testing.T) {
	s := newE2E(t)

	s.mustCreate("/stat_test.txt")
	s.mustWrite("/stat_test.txt", []byte("hello"), 0)
	s.fs.Release("/stat_test.txt", 0)

	st := s.mustGetattr("/stat_test.txt")
	if st.Size != 5 {
		t.Errorf("expected size 5, got %d", st.Size)
	}
	if st.Nlink != 1 {
		t.Errorf("expected Nlink=1, got %d", st.Nlink)
	}
	if st.Mode&fuse.S_IFREG == 0 {
		t.Error("expected regular file mode")
	}
}

func TestE2E_Mknod(t *testing.T) {
	s := newE2E(t)
	errc := s.fs.Mknod("/mknod_test.txt", 0o644, 0)
	if errc != 0 {
		t.Fatalf("Mknod: errc=%d", errc)
	}
	got := s.mustRead("/mknod_test.txt", 10, 0)
	if len(got) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(got))
	}
}

func TestE2E_OpenNonExistent(t *testing.T) {
	s := newE2E(t)
	errc, _ := s.fs.Open("/no_such_file.txt", 0)
	if errc == 0 {
		t.Error("expected error for non-existent file")
	}
}

func TestE2E_UnlinkNonExistent(t *testing.T) {
	s := newE2E(t)
	s.fs.Unlink("/no_such_file.txt")
}

func TestE2E_FileInSubdirectory(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/sub")
	s.writeFile("/sub/nested.txt", []byte("nested content"))

	got := s.mustRead("/sub/nested.txt", 20, 0)
	if string(got) != "nested content" {
		t.Errorf("got %q, want %q", string(got), "nested content")
	}

	entries := s.mustReaddir("/sub")
	if len(entries) != 3 {
		t.Errorf("expected 3 entries in sub, got %v", entries)
	}
}

func TestE2E_DeepNestedDirectory(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/a")
	s.mustMkdir("/a/b")
	s.mustMkdir("/a/b/c")
	s.writeFile("/a/b/c/deep.txt", []byte("deep"))

	got := s.mustRead("/a/b/c/deep.txt", 10, 0)
	if string(got) != "deep" {
		t.Errorf("got %q, want %q", string(got), "deep")
	}
}

func TestE2E_XattrOperations(t *testing.T) {
	s := newE2E(t)

	// List xattr on root (should succeed even if empty)
	errc := s.fs.Listxattr("/", func(name string) bool { return true })
	if errc != 0 {
		t.Errorf("Listxattr: errc=%d", errc)
	}

	// Getxattr should return ENOATTR
	errc, _ = s.fs.Getxattr("/", "user.test")
	if errc == 0 {
		t.Error("expected ENOATTR for unset xattr")
	}

	// Setxattr should succeed
	errc = s.fs.Setxattr("/", "user.test", []byte("value"), 0)
	if errc != 0 {
		t.Errorf("Setxattr: errc=%d", errc)
	}

	// Removexattr
	errc = s.fs.Removexattr("/", "user.test")
	if errc != 0 && errc != -fuse.ENOATTR {
		t.Errorf("Removexattr: unexpected errc=%d", errc)
	}
}

func TestE2E_Flush(t *testing.T) {
	s := newE2E(t)
	s.mustCreate("/flush_test.txt")
	s.mustWrite("/flush_test.txt", []byte("flush me"), 0)
	errc := s.fs.Flush("/flush_test.txt", 0)
	if errc != 0 {
		t.Errorf("Flush: errc=%d", errc)
	}
	s.fs.Release("/flush_test.txt", 0)
}

func TestE2E_Fsync(t *testing.T) {
	s := newE2E(t)
	s.mustCreate("/fsync_test.txt")
	s.mustWrite("/fsync_test.txt", []byte("sync me"), 0)
	errc := s.fs.Fsync("/fsync_test.txt", false, 0)
	if errc != 0 {
		t.Errorf("Fsync: errc=%d", errc)
	}
	s.fs.Release("/fsync_test.txt", 0)
}

// ============================================================
// Writeback-specific e2e tests
// ============================================================

// TestE2E_WritebackFsyncThenRead verifies data survives Fsync + page flush.
func TestE2E_WritebackFsyncThenRead(t *testing.T) {
	s := newE2E(t)
	fh := s.mustCreate("/wbfsync.txt")
	s.mustWrite("/wbfsync.txt", []byte("fsync data"), 0)
	s.fs.Fsync("/wbfsync.txt", false, fh)
	got := s.mustRead("/wbfsync.txt", 20, 0)
	if string(got) != "fsync data" {
		t.Errorf("got %q, want %q", string(got), "fsync data")
	}
	s.fs.Release("/wbfsync.txt", fh)
}

// TestE2E_WritebackMultipleWritesThenRead verifies page coalescing through
// several small sequential writes.
func TestE2E_WritebackMultipleWritesThenRead(t *testing.T) {
	s := newE2E(t)
	fh := s.mustCreate("/wbmulti.txt")
	for i := 0; i < 20; i++ {
		s.mustWrite("/wbmulti.txt", []byte{byte('A' + i)}, int64(i))
	}
	s.fs.Fsync("/wbmulti.txt", false, fh)
	got := s.mustRead("/wbmulti.txt", 20, 0)
	expected := "ABCDEFGHIJKLMNOPQRST"
	if string(got) != expected {
		t.Errorf("got %q, want %q", string(got), expected)
	}
	s.fs.Release("/wbmulti.txt", fh)
}

// TestE2E_WritebackWriteAfterFsync verifies that writing after Fsync does
// not corrupt previously flushed data (page buffer kept, new writes extend).
func TestE2E_WritebackWriteAfterFsync(t *testing.T) {
	s := newE2E(t)
	fh := s.mustCreate("/wbappend.txt")
	s.mustWrite("/wbappend.txt", []byte("AAA"), 0)
	s.fs.Fsync("/wbappend.txt", false, fh)
	s.mustWrite("/wbappend.txt", []byte("BBB"), 3)
	s.fs.Fsync("/wbappend.txt", false, fh)
	got := s.mustRead("/wbappend.txt", 6, 0)
	if string(got) != "AAABBB" {
		t.Errorf("got %q, want %q", string(got), "AAABBB")
	}
	s.fs.Release("/wbappend.txt", fh)
}

// TestE2E_WritebackOverwriteAfterFsync verifies overwriting at an existing
// offset after Fsync picks up the new data.
func TestE2E_WritebackOverwriteAfterFsync(t *testing.T) {
	s := newE2E(t)
	fh := s.mustCreate("/wboverwrite.txt")
	s.mustWrite("/wboverwrite.txt", []byte("xxxxxxxxxxxx"), 0)
	s.fs.Fsync("/wboverwrite.txt", false, fh)
	s.mustWrite("/wboverwrite.txt", []byte("OOO"), 2)
	s.fs.Fsync("/wboverwrite.txt", false, fh)
	got := s.mustRead("/wboverwrite.txt", 12, 0)
	if string(got) != "xxOOOxxxxxxx" {
		t.Errorf("got %q, want %q", string(got), "xxOOOxxxxxxx")
	}
	s.fs.Release("/wboverwrite.txt", fh)
}

// TestE2E_WritebackReleaseTriggersFlush verifies that Release enqueues sync
// and the page data is readable afterward (via page buffer).
func TestE2E_WritebackReleaseRead(t *testing.T) {
	s := newE2E(t)
	fh := s.mustCreate("/wbrelease.txt")
	s.mustWrite("/wbrelease.txt", []byte("release data"), 0)
	s.fs.Release("/wbrelease.txt", fh)
	got := s.mustRead("/wbrelease.txt", 20, 0)
	if string(got) != "release data" {
		t.Errorf("got %q, want %q", string(got), "release data")
	}
}

func TestE2E_ReadDuringUploadStillWorks(t *testing.T) {
	s := newE2E(t)
	s.writeFile("/protect_read.txt", []byte("readable data"))

	s.fs.syncDelayMu.Lock()
	for path, tmr := range s.fs.syncTimers {
		tmr.Stop()
		delete(s.fs.syncTimers, path)
	}
	s.fs.syncDelayMu.Unlock()

	got := s.mustRead("/protect_read.txt", 100, 0)
	if string(got) != "readable data" {
		t.Errorf("got %q, want %q", string(got), "readable data")
	}
}

func TestE2E_SyncTimerMigrateOnRename(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.writeFile("/old.txt", []byte("rename me"))

	if !s.hasTimer("/old.txt") {
		t.Fatal("timer should exist after write")
	}

	errc := s.fs.Rename("/old.txt", "/new.txt")
	if errc != 0 {
		t.Fatalf("Rename: errc=%d", errc)
	}

	if s.hasTimer("/old.txt") {
		t.Error("old path timer should be removed after rename")
	}
	if !s.hasTimer("/new.txt") {
		t.Error("new path timer should be created after rename")
	}

	s.waitForTimer("/new.txt", 2*time.Second)
	s.waitForUpload("/new.txt", 2*time.Second)

	got := s.mustRead("/new.txt", 20, 0)
	if string(got) != "rename me" {
		t.Errorf("content after rename: got %q, want %q", string(got), "rename me")
	}
}

func TestE2E_SyncTimerCancelOnUnlink(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.writeFile("/cancel.txt", []byte("to be deleted"))

	if !s.hasTimer("/cancel.txt") {
		t.Fatal("timer should exist after write")
	}

	s.mustUnlink("/cancel.txt")

	if s.hasTimer("/cancel.txt") {
		t.Error("timer should be cancelled after unlink")
	}
}

func TestE2E_SyncTimerRenameThenDelete(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.writeFile("/stage.txt", []byte("data"))

	s.fs.Rename("/stage.txt", "/stage2.txt")

	if !s.hasTimer("/stage2.txt") {
		t.Fatal("timer should move to /stage2.txt after rename")
	}

	s.mustUnlink("/stage2.txt")

	if s.hasTimer("/stage2.txt") {
		t.Error("timer should be cancelled after unlink")
	}
}

func TestE2E_SyncTimerRenameDirChildren(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.mustMkdir("/mydir")
	s.writeFile("/mydir/f.txt", []byte("nested data"))

	if !s.hasTimer("/mydir/f.txt") {
		t.Fatal("timer should exist for child after write")
	}

	errc := s.fs.Rename("/mydir", "/newdir")
	if errc != 0 {
		t.Fatalf("Rename dir: errc=%d", errc)
	}

	if s.hasTimer("/mydir/f.txt") {
		t.Error("old child path timer should be removed after dir rename")
	}
	if !s.hasTimer("/newdir/f.txt") {
		t.Error("new child path timer should exist after dir rename")
	}

	s.waitForTimer("/newdir/f.txt", 2*time.Second)
	s.waitForUpload("/newdir/f.txt", 2*time.Second)

	got := s.mustRead("/newdir/f.txt", 20, 0)
	if string(got) != "nested data" {
		t.Errorf("content after dir rename: got %q, want %q", string(got), "nested data")
	}
}

func TestE2E_SyncTimerRenameThenWriteDedup(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.writeFile("/v1.txt", []byte("version1"))
	s.fs.Rename("/v1.txt", "/v2.txt")

	if cnt := s.timerCount(); cnt != 1 {
		t.Fatalf("expected 1 timer after rename, got %d", cnt)
	}

	s.mustWrite("/v2.txt", []byte("version2"), 0)
	s.fs.Release("/v2.txt", 0)

	if cnt := s.timerCount(); cnt != 1 {
		t.Fatalf("expected 1 timer after write, got %d", cnt)
	}
	if !s.hasTimer("/v2.txt") {
		t.Error("timer should be at /v2.txt")
	}

	s.waitForTimer("/v2.txt", 2*time.Second)
	s.waitForUpload("/v2.txt", 2*time.Second)

	got := s.mustRead("/v2.txt", 20, 0)
	if string(got) != "version2" {
		t.Errorf("got %q, want %q", string(got), "version2")
	}
}

func TestE2E_SyncTimerMultipleRenames(t *testing.T) {
	s := newE2EWithOpts(t, FSOptions{
		MaxRetries: 3, ConcurrentUploads: 3,
		WriteBackTimeout: 100 * time.Millisecond,
	})

	s.writeFile("/a.txt", []byte("final"))

	s.fs.Rename("/a.txt", "/b.txt")
	s.fs.Rename("/b.txt", "/c.txt")
	s.fs.Rename("/c.txt", "/d.txt")

	if cnt := s.timerCount(); cnt != 1 {
		t.Fatalf("expected 1 timer after triple rename, got %d", cnt)
	}
	if !s.hasTimer("/d.txt") {
		t.Error("timer should be at final path /d.txt")
	}

	s.waitForTimer("/d.txt", 2*time.Second)
	s.waitForUpload("/d.txt", 2*time.Second)

	got := s.mustRead("/d.txt", 20, 0)
	if string(got) != "final" {
		t.Errorf("got %q, want %q", string(got), "final")
	}
}

func TestE2E_RmdirWithUploadingChildReturnsNotEmpty(t *testing.T) {
	s := newE2E(t)
	s.mustMkdir("/protect_dir")
	s.writeFile("/protect_dir/f.txt", []byte("child"))
	dirNode := s.lookup("/protect_dir")

	atomic.AddInt32(&dirNode.uploadingChildren, 1)
	defer atomic.AddInt32(&dirNode.uploadingChildren, -1)

	errc := s.fs.Rmdir("/protect_dir")
	if errc != -fuse.ENOTEMPTY {
		t.Errorf("expected ENOTEMPTY when child is uploading, got %d", errc)
	}
}

func TestShutdown_SetsFlagAndRejectsOps(t *testing.T) {
	s := newE2E(t)

	if s.fs.IsShuttingDown() {
		t.Fatal("should not be shutting down initially")
	}

	s.fs.Shutdown()

	if !s.fs.IsShuttingDown() {
		t.Fatal("should be shutting down after Shutdown()")
	}

	// Operations should be rejected after shutdown.
	if errc := s.fs.Unlink("/nonexistent"); errc != -fuse.EIO {
		t.Errorf("expected EIO for Unlink after shutdown, got %d", errc)
	}
	if errc := s.fs.Rmdir("/"); errc != -fuse.EIO {
		t.Errorf("expected EIO for Rmdir after shutdown, got %d", errc)
	}
}
