package qrypt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	pageFlushDelay     = 250 * time.Millisecond
	pageMaxSize        = 1 << 20 // 1MB — flush early if page grows beyond this
	pageInitialBufSize = 4096
)

// roundUpPow2 rounds v up to the next power of 2.
// Returns 0 when v is 0 (caller should avoid passing 0).
func roundUpPow2(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}

// Page buffers writes for a single staging file. Consecutive writes to the
// same file are coalesced in memory and flushed asynchronously.
type Page struct {
	mu        sync.Mutex
	buf       []byte // backing buffer; grows via append / overwrite
	dirty     bool
	timer     *time.Timer
	fid       string
	maxOffset int64                              // highest off+len(data) seen; actual data size in buf
	flush     func(fid string, buf []byte) error // calls staging's write-to-disk
	onDone    func(fid string)                   // cleanup after flush/close
}

// Store manages staging files on disk with an optional writeback page cache.
type Store struct {
	dir   string
	pages sync.Map // fid → *Page
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string {
	return s.dir
}

const diskSpaceThresholdWarn = 1 << 30
const diskSpaceThresholdCrit = 100 << 20

func (s *Store) checkDiskSpace() error {
	return nil
}

type ErrDiskSpaceCritical struct {
	Available int64
}

func (e *ErrDiskSpaceCritical) Error() string {
	return "critical low disk space: available " + formatBytes(e.Available)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	exp := 0
	for n >= unit {
		n /= unit
		exp++
	}
	return fmt.Sprintf("%d %cB", n, "KMGTPE"[exp])
}

func (s *Store) Path(fid string) string {
	return filepath.Join(s.dir, fid+".staging")
}

func FidFromPath(path string) string {
	base := filepath.Base(path)
	if filepath.Ext(base) == ".staging" {
		return base[:len(base)-len(".staging")]
	}
	return base
}

func (s *Store) Create(fid string) (string, error) {
	if err := s.checkDiskSpace(); err != nil {
		return "", err
	}
	path := s.Path(fid)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) Ensure(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// getPage returns the Page for the given fid, creating one if needed.
func (s *Store) getPage(fid string) *Page {
	if v, ok := s.pages.Load(fid); ok {
		return v.(*Page)
	}
	p := &Page{
		fid:   fid,
		buf:   make([]byte, 0, pageInitialBufSize),
		flush: func(fid string, buf []byte) error { return s.writePage(fid, buf) },
		onDone: func(fid string) {
			s.pages.Delete(fid)
		},
	}
	s.pages.Store(fid, p)
	return p
}

// writePage writes buf to the staging file on disk and truncates to its size.
func (s *Store) writePage(fid string, buf []byte) error {
	path := s.Path(fid)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := f.WriteAt(buf, 0)
	if err == nil && n != len(buf) {
		return io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	// Truncate to actual data size — buf may be shorter than the on-disk file
	// if previous data was removed or overwritten with a smaller amount.
	return f.Truncate(int64(len(buf)))
}

// WriteAt buffers data in a Page if one exists or can be created for the fid
// extracted from path; otherwise falls through to a direct disk write.
func (s *Store) WriteAt(path string, data []byte, off int64) (int, error) {
	if err := s.checkDiskSpace(); err != nil {
		return 0, err
	}

	fid := FidFromPath(path)

	if v, ok := s.pages.Load(fid); ok {
		return v.(*Page).WriteAt(data, off)
	}

	if len(data) < pageMaxSize/4 {
		// Only create a page for files that are still empty on disk.
		// If the file already has data (from prior direct-to-disk writes),
		// the page buffer would start empty and overwrite that data with
		// zeros when flushed.
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			goto directWrite
		}
		return s.getPage(fid).WriteAt(data, off)
	}

directWrite:
	// Large write, or small write to a file with existing disk data — direct to disk.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.WriteAt(data, off)
}

// Page.WriteAt buffers data in memory. Multiple writes to the same fid are
// coalesced until a flush trigger (timer, Sync, Close, or page full).
func (p *Page) WriteAt(data []byte, off int64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	need := off + int64(len(data))
	if need > int64(len(p.buf)) {
		newSize := need
		// Exponential growth below pageMaxSize to avoid O(n²) copying
		// from repeated exact-fit reallocation on sequential writes.
		if newSize < pageMaxSize {
			newSize = int64(roundUpPow2(uint64(need)))
		}
		newBuf := make([]byte, newSize)
		copy(newBuf, p.buf)
		p.buf = newBuf
	}
	copy(p.buf[off:], data)
	p.dirty = true
	if end := off + int64(len(data)); end > p.maxOffset {
		p.maxOffset = end
	}

	// Flush early if page exceeds threshold.
	if len(p.buf) > pageMaxSize {
		p.mu.Unlock()
		p.flushNow()
		p.mu.Lock()
	} else {
		p.resetTimer()
	}

	return len(data), nil
}

// flushBuf writes buffered data to disk unconditionally. After flushing,
// the buffer is kept in memory (dirty cleared) so subsequent writes can
// extend it without losing data. Caller must NOT hold p.mu.
func (p *Page) flushBuf() error {
	p.mu.Lock()
	if p.maxOffset == 0 {
		p.mu.Unlock()
		return nil
	}
	buf := p.flushDataLocked()
	p.dirty = false
	p.stopTimer()
	p.mu.Unlock()

	if buf == nil {
		return nil
	}
	return p.flush(p.fid, buf)
}

// flushNow writes buffered data to disk only if dirty.  After flushing,
// the buffer is kept in memory (dirty cleared) so subsequent writes can
// extend it without losing data at earlier offsets.
// Caller must NOT hold p.mu.
func (p *Page) flushNow() error {
	p.mu.Lock()
	if !p.dirty {
		p.mu.Unlock()
		return nil
	}
	buf := p.flushDataLocked()
	p.dirty = false
	p.stopTimer()
	p.mu.Unlock()

	if buf == nil {
		return nil
	}
	return p.flush(p.fid, buf)
}

// resetTimer (re-)arms the flush timer.  Caller must hold p.mu.
func (p *Page) resetTimer() {
	if p.timer != nil {
		p.timer.Stop()
	}
	page := p
	p.timer = time.AfterFunc(pageFlushDelay, func() {
		_ = page.flushNow()
	})
}

// stopTimer stops the flush timer.  Caller must hold p.mu.
func (p *Page) stopTimer() {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

// flushDataLocked returns a copy of the buffer containing only the actual data
// (up to maxOffset). Caller must hold p.mu.
func (p *Page) flushDataLocked() []byte {
	dataLen := p.maxOffset
	if dataLen > int64(len(p.buf)) {
		dataLen = int64(len(p.buf))
	}
	if dataLen == 0 {
		return nil
	}
	buf := make([]byte, dataLen)
	copy(buf, p.buf[:dataLen])
	return buf
}

func (s *Store) ReadAt(path string, buf []byte, off int64) (int, error) {
	fid := FidFromPath(path)
	if v, ok := s.pages.Load(fid); ok {
		p := v.(*Page)
		p.mu.Lock()
		if off < p.maxOffset {
			// Read directly from page buffer — it holds the latest data
			// regardless of dirty state. Avoids disk reads that may hit
			// a stale empty file (replaced by concurrent Snapshot).
			// Cap at actual data (maxOffset), not rounded-up buffer length.
			readLen := p.maxOffset - off
			if readLen > int64(len(buf)) {
				readLen = int64(len(buf))
			}
			n := copy(buf, p.buf[off:off+readLen])
			p.mu.Unlock()

			// Caller asked for more data than the buffer has — flush to
			// disk so subsequent reads/stats see a consistent on-disk file.
			if int64(len(buf)) > readLen {
				_ = p.flushNow()
			}

			return n, nil
		}
		// Partial or beyond page — flush to disk first for consistency.
		p.mu.Unlock()
		_ = p.flushNow()
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := f.ReadAt(buf, off)
	if n == 0 && errors.Is(err, io.EOF) {
		// The file exists but is empty — it may have been snapshotted
		// by a concurrent syncFile (os.Rename replaces the original
		// with a new empty file).  Try the snapshot copy instead.
		snapPath := path + ".snap"
		if sf, sErr := os.Open(snapPath); sErr == nil {
			defer sf.Close()
			return sf.ReadAt(buf, off)
		}
	}
	return n, err
}

// flushBuf flushes any pending page buffer for the given path to disk.
// Used by read/size/truncate paths that need disk consistency.
func (s *Store) flushBuf(path string) error {
	fid := FidFromPath(path)
	if v, ok := s.pages.Load(fid); ok {
		return v.(*Page).flushNow()
	}
	return nil
}

// flushBufForce writes the page buffer to disk regardless of dirty state.
func (s *Store) flushBufForce(path string) error {
	fid := FidFromPath(path)
	if v, ok := s.pages.Load(fid); ok {
		return v.(*Page).flushBuf()
	}
	return nil
}

func (s *Store) OpenReader(path string) (io.ReadCloser, error) {
	if err := s.flushBuf(path); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *Store) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Store) FileSize(path string) (int64, error) {
	if err := s.flushBuf(path); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Truncate resizes the staging file on disk and invalidates the page buffer
// for that fid so subsequent writes build a fresh page.
func (s *Store) Truncate(path string, size int64) error {
	// Flush any buffered data to disk first so the on-disk file is current.
	if err := s.flushBuf(path); err != nil {
		return err
	}
	// Discard the page — truncate changes the file size and the page buffer
	// would reference stale offset ranges.
	fid := FidFromPath(path)
	s.pages.Delete(fid)
	if err := s.Ensure(path); err != nil {
		return err
	}
	return os.Truncate(path, size)
}

func (s *Store) Snapshot(path string) (string, error) {
	if err := s.flushBuf(path); err != nil {
		return "", err
	}

	// Write page buffer to disk even if not dirty — the buffer holds the
	// latest data and the disk file may be stale (e.g., from a previous
	// Snapshot that created an empty file).
	if err := s.flushBufForce(path); err != nil {
		return "", err
	}

	snapPath := path + ".snap"
	if err := os.Rename(path, snapPath); err != nil {
		return "", err
	}
	if err := s.Ensure(path); err != nil {
		os.Rename(snapPath, path)
		return "", err
	}
	return snapPath, nil
}

func (s *Store) ReleaseSnapshot(snapPath string) error {
	return os.Remove(snapPath)
}

func (s *Store) RestoreSnapshot(path, snapPath string) error {
	if snapPath == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(snapPath, path); err != nil {
		return err
	}
	return nil
}

func (s *Store) Remove(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) Sync(path string) error {
	fid := FidFromPath(path)
	if v, ok := s.pages.Load(fid); ok {
		if err := v.(*Page).flushNow(); err != nil {
			return err
		}
	}
	// Also fsync the on-disk file so the page flush is durable.
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Close flushes and destroys the page for the given path.  It is safe to call
// multiple times (no-op after the first flush).
func (s *Store) Close(path string) error {
	fid := FidFromPath(path)
	if v, ok := s.pages.Load(fid); ok {
		p := v.(*Page)
		p.mu.Lock()
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		dirty := p.dirty
		buf := p.flushDataLocked()
		p.buf = nil
		p.dirty = false
		p.mu.Unlock()

		s.pages.Delete(fid)

		if dirty && len(buf) > 0 {
			if err := s.writePage(fid, buf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ListStagingFiles() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".staging" {
			files = append(files, filepath.Join(s.dir, name))
		}
	}
	return files, nil
}

func (s *Store) CleanupOrphanedStagingFiles(activeFids map[string]bool) ([]string, error) {
	files, err := s.ListStagingFiles()
	if err != nil {
		return nil, err
	}
	var cleaned []string
	for _, path := range files {
		fid := FidFromPath(path)
		if !activeFids[fid] {
			if err := s.Remove(path); err != nil {
				continue
			}
			cleaned = append(cleaned, path)
		}
	}
	return cleaned, nil
}
