package qrypt

import (
	"context"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type Options struct {
	Cipher        cipher.Cipher
	Dirs          DirResolver
	Creds         CredentialStore
	DriverFactory DriverFactory
	RateLimitBPS  int64
	NumWorkers    int
	MountLister   MountLister
	CacheHooks    CacheInvalidatorHooks
}

type FileAPI struct {
	cp    cipher.Cipher
	dirs  DirResolver
	creds CredentialStore

	sessions  SessionManager
	uploadQ   Orchestrator
	eventBus  EventBus
	progress  ProgressHub
	cacheInv  *CacheInvalidator
	rateLimit RateLimiter
}

func NewFileAPI(opts Options) (*FileAPI, error) {
	if opts.Cipher == nil {
		return nil, NewErrorf(ErrInternal, "Cipher is required")
	}
	if opts.Dirs == nil {
		return nil, NewErrorf(ErrInternal, "DirResolver is required")
	}
	if opts.Creds == nil {
		return nil, NewErrorf(ErrInternal, "CredentialStore is required")
	}
	if opts.DriverFactory == nil {
		return nil, NewErrorf(ErrInternal, "DriverFactory is required")
	}
	if opts.NumWorkers <= 0 {
		opts.NumWorkers = 3
	}

	eb := NewEventBus()
	ph := NewProgressHub(eb)
	rl := NewRateLimiter(opts.RateLimitBPS)
	oq := NewOrchestrator(opts.NumWorkers, rl, ph)
	sm := NewSessionManager(opts.DriverFactory)

	api := &FileAPI{
		cp:        opts.Cipher,
		dirs:      opts.Dirs,
		creds:     opts.Creds,
		sessions:  sm,
		eventBus:  eb,
		progress:  ph,
		rateLimit: rl,
		uploadQ:   oq,
	}

	if opts.CacheHooks != nil {
		api.cacheInv = NewCacheInvalidator(opts.CacheHooks, eb)
	}

	return api, nil
}

func (a *FileAPI) Events() EventBus         { return a.eventBus }
func (a *FileAPI) Progress() ProgressHub    { return a.progress }
func (a *FileAPI) Sessions() SessionManager { return a.sessions }

func (a *FileAPI) acquireDriver(ctx context.Context, mount string) (drivers.Driver, error) {
	if a.sessions == nil {
		return nil, NewErrorf(ErrInternal, "no session manager")
	}
	key := SessionKey{Type: mount, CredKey: mount}
	if mount == "" {
		key = SessionKey{Type: "default", CredKey: "default"}
	}
	cfg := SessionConfig{Type: key.Type}
	s, err := a.sessions.Acquire(ctx, key, cfg)
	if err != nil {
		return nil, err
	}
	return s.Drv, nil
}

func (a *FileAPI) releaseDriver(ctx context.Context, mount string) {
	if a.sessions == nil {
		return
	}
	key := SessionKey{Type: mount, CredKey: mount}
	if mount == "" {
		key = SessionKey{Type: "default", CredKey: "default"}
	}
	a.sessions.Release(ctx, key)
}

func (a *FileAPI) dirOf(path string) string {
	idx := strings.LastIndex(strings.TrimRight(path, "/"), "/")
	if idx < 0 {
		return "/"
	}
	return path[:idx+1]
}

func (a *FileAPI) baseOf(path string) string {
	path = strings.TrimRight(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}

func (a *FileAPI) resolvePath(ctx context.Context, drv drivers.Driver, path string) (string, error) {
	r, ok := drv.(drivers.PathResolver)
	if !ok {
		return "", NewErrorf(ErrInternal, "driver does not support path resolution")
	}
	return r.ResolvePath(ctx, path)
}

func (a *FileAPI) List(ctx context.Context, mount, path string) ([]FileEntry, error) {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return nil, err
	}
	defer a.releaseDriver(ctx, mount)

	fid, err := a.resolvePath(ctx, drv, path)
	if err != nil {
		return nil, err
	}

	entries, err := drv.List(ctx, fid)
	if err != nil {
		return nil, WrapError(ErrNetwork, "list", err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		decName, decErr := a.cp.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}
		plainSize, sizeErr := a.cp.DecryptedSize(e.Size)
		if sizeErr != nil {
			plainSize = e.Size
		}
		result = append(result, FileEntry{
			ID:        e.ID,
			Name:      e.Name,
			DecName:   decName,
			IsDir:     e.IsDir,
			Size:      e.Size,
			PlainSize: plainSize,
			ModTime:   e.ModTime,
		})
	}
	return result, nil
}

func (a *FileAPI) Stat(ctx context.Context, mount, path string) (*FileEntry, error) {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return nil, err
	}
	defer a.releaseDriver(ctx, mount)

	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	if path == "/" {
		rootFid, err := a.resolvePath(ctx, drv, "/")
		if err != nil {
			return nil, err
		}
		return &FileEntry{ID: rootFid, Name: "/", DecName: "/", IsDir: true}, nil
	}

	parentPath := a.dirOf(path)
	baseName := a.baseOf(path)

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return nil, err
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		return nil, WrapError(ErrNetwork, "list parent", err)
	}

	encName := a.cp.EncryptSegment(baseName)
	for _, e := range entries {
		if e.Name == encName || e.Name == baseName {
			decName, decErr := a.cp.DecryptSegment(e.Name)
			if decErr != nil {
				decName = e.Name
			}
			plainSize, sizeErr := a.cp.DecryptedSize(e.Size)
			if sizeErr != nil {
				plainSize = e.Size
			}
			return &FileEntry{
				ID:        e.ID,
				Name:      e.Name,
				DecName:   decName,
				IsDir:     e.IsDir,
				Size:      e.Size,
				PlainSize: plainSize,
				ModTime:   e.ModTime,
			}, nil
		}
	}

	return nil, NewErrorf(ErrNotFound, "not found: %s", path)
}

func (a *FileAPI) Mkdir(ctx context.Context, mount, path string) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	parentPath := a.dirOf(path)
	dirName := a.baseOf(path)

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return err
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		return WrapError(ErrNetwork, "list parent", err)
	}

	encName := a.cp.EncryptSegment(dirName)
	for _, e := range entries {
		if e.Name == encName {
			if e.IsDir {
				return nil
			}
			return NewErrorf(ErrAlreadyExists, "path exists and is not a directory: %s", path)
		}
	}

	w, ok := drv.(drivers.Writer)
	if !ok {
		return NewErrorf(ErrInternal, "driver does not support write operations")
	}
	_, err = w.Mkdir(ctx, parentFid, encName)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return WrapError(ErrAlreadyExists, "mkdir", err)
		}
		return WrapError(ErrInternal, "mkdir", err)
	}
	return nil
}

type MoveOptions struct {
	NoClobber bool
}

func (a *FileAPI) Move(ctx context.Context, mount, oldPath, newPath string, opts ...MoveOptions) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	noClobber := false
	if len(opts) > 0 {
		noClobber = opts[0].NoClobber
	}

	srcFid, err := a.resolvePath(ctx, drv, oldPath)
	if err != nil {
		return err
	}

	dstParentPath := a.dirOf(newPath)
	dstName := a.cp.EncryptSegment(a.baseOf(newPath))

	dstParentFid, err := a.resolvePath(ctx, drv, dstParentPath)
	if err != nil {
		return WrapError(ErrNotFound, "resolve dest parent", err)
	}

	w, ok := drv.(drivers.Writer)
	if !ok {
		return NewErrorf(ErrInternal, "driver does not support move")
	}

	if noClobber {
		entries, _ := drv.List(ctx, dstParentFid)
		for _, e := range entries {
			if e.Name == dstName && e.ID != srcFid {
				return NewErrorf(ErrAlreadyExists, "target exists (no-clobber)")
			}
		}
	}

	srcParentPath := a.dirOf(oldPath)
	srcParentFid, _ := a.resolvePath(ctx, drv, srcParentPath)

	if srcParentFid != dstParentFid {
		if err := w.Move(ctx, drivers.Entry{ID: srcFid}, dstParentFid); err != nil {
			return WrapError(ErrInternal, "move", err)
		}
	}

	srcName := a.baseOf(oldPath)
	srcEncName := a.cp.EncryptSegment(srcName)
	if dstName != srcEncName {
		if err := w.Rename(ctx, drivers.Entry{ID: srcFid}, dstName); err != nil {
			return WrapError(ErrInternal, "rename", err)
		}
	}

	return nil
}

func (a *FileAPI) Remove(ctx context.Context, mount, path string, recursive bool) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	parentPath := a.dirOf(path)
	baseName := a.baseOf(path)

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return err
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		return WrapError(ErrNetwork, "list parent", err)
	}

	encName := a.cp.EncryptSegment(baseName)
	var target drivers.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName || e.Name == baseName {
			target = e
			found = true
			break
		}
	}
	if !found {
		return NewErrorf(ErrNotFound, "not found: %s", path)
	}

	if target.IsDir {
		if recursive {
			if err := a.removeAll(ctx, drv, target); err != nil {
				return err
			}
		} else {
			children, listErr := drv.List(ctx, target.ID)
			if listErr != nil {
				return WrapError(ErrNetwork, "list target", listErr)
			}
			if len(children) > 0 {
				return NewErrorf(ErrNotEmpty, "directory not empty: %s", path)
			}
		}
	}

	w, ok := drv.(drivers.Writer)
	if !ok {
		return NewErrorf(ErrInternal, "driver does not support delete")
	}
	if err := w.Remove(ctx, target); err != nil {
		return WrapError(ErrInternal, "remove", err)
	}
	return nil
}

func (a *FileAPI) removeAll(ctx context.Context, drv drivers.Driver, dir drivers.Entry) error {
	children, err := drv.List(ctx, dir.ID)
	if err != nil {
		return WrapError(ErrNetwork, "list children", err)
	}
	for _, child := range children {
		if child.IsDir {
			if err := a.removeAll(ctx, drv, child); err != nil {
				return err
			}
		}
		w, ok := drv.(drivers.Writer)
		if !ok {
			return NewErrorf(ErrInternal, "driver does not support delete")
		}
		if err := w.Remove(ctx, child); err != nil {
			return WrapError(ErrInternal, "remove child", err)
		}
	}
	return nil
}

func (a *FileAPI) Read(ctx context.Context, mount, path string) (io.ReadCloser, error) {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return nil, err
	}
	defer a.releaseDriver(ctx, mount)

	parentPath := a.dirOf(path)
	baseName := a.baseOf(path)

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return nil, err
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		return nil, WrapError(ErrNetwork, "list parent", err)
	}

	encName := a.cp.EncryptSegment(baseName)
	var target drivers.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName || e.Name == baseName {
			target = e
			found = true
			break
		}
	}
	if !found {
		return nil, NewErrorf(ErrNotFound, "not found: %s", path)
	}

	rc, err := a.readFile(ctx, drv, target)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (a *FileAPI) readFile(ctx context.Context, drv drivers.Driver, target drivers.Entry) (io.ReadCloser, error) {
	headerSize := int64(cipher.FileHeaderSize)
	rcHeader, err := drv.Read(ctx, target, 0, headerSize)
	if err != nil {
		return nil, WrapError(ErrNetwork, "read header", err)
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(rcHeader, header); err != nil {
		rcHeader.Close()
		return nil, WrapError(ErrNetwork, "read header data", err)
	}
	rcHeader.Close()

	var fileNonce [cipher.FileNonceSize]byte
	copy(fileNonce[:], header[cipher.FileMagicSize:])

	encBodySize := target.Size - headerSize
	if encBodySize <= 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}

	rcBody, err := drv.Read(ctx, target, headerSize, encBodySize)
	if err != nil {
		return nil, WrapError(ErrNetwork, "read body", err)
	}

	decReader := NewDecryptingReader(rcBody, a.cp, fileNonce)

	return &decryptReadCloser{decReader, rcBody}, nil
}

type decryptReadCloser struct {
	*DecryptingReader
	body io.Closer
}

func (d *decryptReadCloser) Close() error {
	if d.body != nil {
		return d.body.Close()
	}
	return nil
}

func (a *FileAPI) Push(ctx context.Context, mount, localPath, remotePath string, opts ...PushOptions) error {
	fi, err := os.Stat(localPath)
	if err != nil {
		return WrapError(ErrNotFound, "stat local", err)
	}
	if fi.IsDir() {
		return a.pushDirectory(ctx, mount, localPath, remotePath)
	}
	return a.pushFile(ctx, mount, localPath, remotePath, opts...)
}

type PushOptions struct {
	OnProgress func(p *ProgressEntry)
}

func (a *FileAPI) pushFile(ctx context.Context, mount, localPath, remotePath string, opts ...PushOptions) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	remoteDir := a.dirOf(remotePath)
	fileName := a.baseOf(remotePath)

	f, err := os.Open(localPath)
	if err != nil {
		return WrapError(ErrNotFound, "open local file", err)
	}
	defer f.Close()

	fi, _ := f.Stat()

	parentFid, err := a.resolvePath(ctx, drv, remoteDir)
	if err != nil {
		return err
	}

	up, ok := drv.(drivers.Uploader)
	if !ok {
		return NewErrorf(ErrInternal, "driver does not support upload")
	}

	taskID := fmt.Sprintf("push_%d", time.Now().UnixNano())
	a.progress.Publish(&ProgressEntry{
		TaskID:    taskID,
		Mount:     mount,
		Direction: "push",
		File:      localPath,
		State:     "started",
	})

	var wg sync.WaitGroup
	var mu sync.Mutex
	var uploadErr error
	wg.Add(1)
	queued := a.uploadQ.Submit(func(ctx context.Context) error {
		defer wg.Done()
		_, putErr := EncryptAndPut(ctx, up, a.cp, EncryptPutRequest{
			Reader:    f,
			PlainSize: fi.Size(),
			PlainName: fileName,
			ParentID:  parentFid,
		})
		mu.Lock()
		if putErr != nil {
			uploadErr = putErr
			a.progress.Publish(&ProgressEntry{
				TaskID: taskID, Mount: mount, Direction: "push",
				File: localPath, Bytes: fi.Size(), Total: fi.Size(),
				State: "failed", Error: putErr.Error(),
			})
		} else {
			a.progress.Publish(&ProgressEntry{
				TaskID: taskID, Mount: mount, Direction: "push",
				File: localPath, Bytes: fi.Size(), Total: fi.Size(),
				State: "completed",
			})
		}
		for _, o := range opts {
			if o.OnProgress != nil {
				pe := &ProgressEntry{
					TaskID: taskID, Mount: mount, Direction: "push",
					File: localPath, Bytes: fi.Size(), Total: fi.Size(),
				}
				if putErr != nil {
					pe.State = "failed"
					pe.Error = putErr.Error()
				} else {
					pe.State = "completed"
				}
				o.OnProgress(pe)
			}
		}
		mu.Unlock()
		return putErr
	})
	if !queued {
		wg.Done()
		return NewErrorf(ErrInternal, "upload queue full")
	}
	wg.Wait()
	if uploadErr != nil {
		return WrapError(ErrNetwork, "push", uploadErr)
	}
	return nil
}

func (a *FileAPI) pushDirectory(ctx context.Context, mount, localDir, remoteDir string) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	up, upOK := drv.(drivers.Uploader)
	if !upOK {
		return NewErrorf(ErrInternal, "driver does not support upload")
	}
	w, wOK := drv.(drivers.Writer)

	rootFid, err := a.resolveOrCreateDir(ctx, drv, remoteDir, w, wOK)
	if err != nil {
		return err
	}

	dirFids := map[string]string{".": rootFid}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var closeFiles []*os.File
	defer func() {
		for _, f := range closeFiles {
			f.Close()
		}
	}()

	walkErr := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(localDir, path)
		if relPath == "." {
			return nil
		}
		parentRel := filepath.Dir(relPath)
		currentParent := dirFids[parentRel]

		if info.IsDir() {
			encName := a.cp.EncryptSegment(info.Name())
			entries, _ := drv.List(ctx, currentParent)
			var fid string
			for _, e := range entries {
				if e.IsDir && e.Name == encName {
					fid = e.ID
					break
				}
			}
			if fid == "" && wOK {
				ne, mkErr := w.Mkdir(ctx, currentParent, encName)
				if mkErr == nil {
					fid = ne.ID
				}
			}
			dirFids[relPath] = fid
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		closeFiles = append(closeFiles, f)

		fi, _ := f.Stat()
		nonce, nonceErr := a.cp.GenerateRandomNonce()
		if nonceErr != nil {
			return nonceErr
		}
		encName := a.cp.EncryptSegment(info.Name())
		encSize := a.cp.EncryptedSize(fi.Size())
		encReader := NewEncryptingReader(f, a.cp, nonce, fi.Size())

		wg.Add(1)
		submitted := a.uploadQ.Submit(func(ctx context.Context) error {
			defer wg.Done()
			_, putErr := up.Put(ctx, currentParent, encName, encSize, encReader)
			mu.Lock()
			if putErr != nil && firstErr == nil {
				firstErr = putErr
			}
			mu.Unlock()
			return putErr
		})
		if !submitted {
			wg.Done()
		}

		return nil
	})

	wg.Wait()
	if walkErr != nil {
		return WrapError(ErrInternal, "walk local dir", walkErr)
	}
	if firstErr != nil {
		return WrapError(ErrNetwork, "push dir", firstErr)
	}
	return nil
}

func (a *FileAPI) resolveOrCreateDir(ctx context.Context, drv drivers.Driver, path string, w drivers.Writer, wOK bool) (string, error) {
	parentPath := a.dirOf(path)
	dirName := a.baseOf(path)
	if dirName == "" {
		return a.resolvePath(ctx, drv, path)
	}

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return "", err
	}

	encName := a.cp.EncryptSegment(dirName)
	entries, _ := drv.List(ctx, parentFid)
	for _, e := range entries {
		if e.IsDir && e.Name == encName {
			return e.ID, nil
		}
	}

	if !wOK {
		return parentFid, nil
	}
	ne, err := w.Mkdir(ctx, parentFid, encName)
	if err != nil {
		return "", WrapError(ErrInternal, "mkdir remote", err)
	}
	return ne.ID, nil
}

func (a *FileAPI) Pull(ctx context.Context, mount, remotePath, localPath string, opts ...PullOptions) error {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return err
	}
	defer a.releaseDriver(ctx, mount)

	parentPath := a.dirOf(remotePath)
	baseName := a.baseOf(remotePath)

	parentFid, err := a.resolvePath(ctx, drv, parentPath)
	if err != nil {
		return err
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		return WrapError(ErrNetwork, "list", err)
	}

	encName := a.cp.EncryptSegment(baseName)
	var target drivers.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName || e.Name == baseName {
			target = e
			found = true
			break
		}
	}
	if !found {
		return NewErrorf(ErrNotFound, "not found: %s", remotePath)
	}

	if localPath == "" {
		localPath = baseName
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return WrapError(ErrInternal, "create local dir", err)
	}

	taskID := fmt.Sprintf("pull_%d", time.Now().UnixNano())
	a.progress.Publish(&ProgressEntry{
		TaskID:    taskID,
		Mount:     mount,
		Direction: "pull",
		File:      remotePath,
		State:     "started",
	})

	outFile, err := os.Create(localPath)
	if err != nil {
		return WrapError(ErrInternal, "create local file", err)
	}
	success := false
	defer func() {
		outFile.Close()
		if !success {
			os.Remove(localPath)
		}
	}()

	headerSize := int64(cipher.FileHeaderSize)
	rcHeader, err := drv.Read(ctx, target, 0, headerSize)
	if err != nil {
		return WrapError(ErrNetwork, "read header", err)
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(rcHeader, header); err != nil {
		rcHeader.Close()
		return WrapError(ErrNetwork, "read header data", err)
	}
	rcHeader.Close()

	var fileNonce [cipher.FileNonceSize]byte
	copy(fileNonce[:], header[cipher.FileMagicSize:])

	encBodySize := target.Size - headerSize
	if encBodySize > 0 {
		rcBody, err := drv.Read(ctx, target, headerSize, encBodySize)
		if err != nil {
			return WrapError(ErrNetwork, "read body", err)
		}
		defer rcBody.Close()

		decReader := NewDecryptingReader(rcBody, a.cp, fileNonce)

		if _, err := io.Copy(outFile, decReader); err != nil {
			return WrapError(ErrNetwork, "copy decrypted stream", err)
		}
	}

	success = true

	a.progress.Publish(&ProgressEntry{
		TaskID:    taskID,
		Mount:     mount,
		Direction: "pull",
		File:      localPath,
		Bytes:     target.Size,
		Total:     target.Size,
		State:     "completed",
	})

	return nil
}

type PullOptions struct {
	OnProgress func(p *ProgressEntry)
}

func (a *FileAPI) Find(ctx context.Context, mount, path, pattern string, maxDepth, maxMatches int, caseSensitive bool, workers int) ([]FileEntry, error) {
	drv, err := a.acquireDriver(ctx, mount)
	if err != nil {
		return nil, err
	}
	defer a.releaseDriver(ctx, mount)
	path = cleanRemotePath(path)

	fid, err := a.resolvePath(ctx, drv, path)
	if err != nil {
		return nil, err
	}

	var result []FileEntry
	if maxDepth == 0 {
		maxDepth = -1
	}
	if workers == 0 {
		workers = 4
	} else if workers < 1 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}
	if workers > 1 {
		err = a.walkAndMatchConcurrent(ctx, drv, fid, path, maxDepth, maxMatches, pattern, caseSensitive, workers, &result)
	} else {
		err = a.walkAndMatch(ctx, drv, fid, path, 0, maxDepth, maxMatches, pattern, caseSensitive, &result)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (a *FileAPI) walkAndMatch(ctx context.Context, drv drivers.Driver, fid, displayPath string, depth, maxDepth, maxMatches int, pattern string, caseSensitive bool, result *[]FileEntry) error {
	if maxDepth >= 0 && depth > maxDepth {
		return nil
	}
	if maxMatches > 0 && len(*result) >= maxMatches {
		return nil
	}

	entries, err := drv.List(ctx, fid)
	if err != nil {
		return WrapError(ErrNetwork, "list", err)
	}

	for _, e := range entries {
		decName, decErr := a.cp.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}
		childPath := joinRemotePath(displayPath, decName)

		if matchName(decName, pattern, caseSensitive) {
			if maxMatches > 0 && len(*result) >= maxMatches {
				break
			}
			plainSize, sizeErr := a.cp.DecryptedSize(e.Size)
			if sizeErr != nil {
				plainSize = e.Size
			}
			*result = append(*result, FileEntry{
				ID:        e.ID,
				Path:      childPath,
				Name:      e.Name,
				DecName:   decName,
				IsDir:     e.IsDir,
				Size:      e.Size,
				PlainSize: plainSize,
			})
		}

		if e.IsDir {
			if err := a.walkAndMatch(ctx, drv, e.ID, childPath, depth+1, maxDepth, maxMatches, pattern, caseSensitive, result); err != nil {
				continue
			}
		}
	}
	return nil
}

func (a *FileAPI) walkAndMatchConcurrent(ctx context.Context, drv drivers.Driver, fid, displayPath string, maxDepth, maxMatches int, pattern string, caseSensitive bool, workers int, result *[]FileEntry) error {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	stopped := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return firstErr != nil || maxMatches > 0 && len(*result) >= maxMatches
	}
	addMatch := func(e drivers.Entry, decName, childPath string) {
		plainSize, sizeErr := a.cp.DecryptedSize(e.Size)
		if sizeErr != nil {
			plainSize = e.Size
		}
		mu.Lock()
		defer mu.Unlock()
		if maxMatches > 0 && len(*result) >= maxMatches {
			return
		}
		*result = append(*result, FileEntry{ID: e.ID, Path: childPath, Name: e.Name, DecName: decName, IsDir: e.IsDir, Size: e.Size, PlainSize: plainSize, ModTime: e.ModTime})
	}

	var walk func(string, string, int)
	walk = func(currentID, currentPath string, depth int) {
		defer wg.Done()
		if maxDepth >= 0 && depth > maxDepth || stopped() {
			return
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			setErr(ctx.Err())
			return
		}
		entries, err := drv.List(ctx, currentID)
		<-sem
		if err != nil {
			setErr(WrapError(ErrNetwork, "list", err))
			return
		}
		for _, e := range entries {
			if stopped() {
				return
			}
			decName, decErr := a.cp.DecryptSegment(e.Name)
			if decErr != nil {
				decName = e.Name
			}
			childPath := joinRemotePath(currentPath, decName)
			if matchName(decName, pattern, caseSensitive) {
				addMatch(e, decName, childPath)
			}
			if e.IsDir {
				wg.Add(1)
				go walk(e.ID, childPath, depth+1)
			}
		}
	}
	wg.Add(1)
	go walk(fid, displayPath, 0)
	wg.Wait()
	return firstErr
}

func matchName(name, pattern string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(name, pattern)
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(pattern))
}

func cleanRemotePath(p string) string {
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return pathpkg.Clean(p)
}

func joinRemotePath(parent, name string) string {
	return pathpkg.Join(cleanRemotePath(parent), name)
}

func (a *FileAPI) Shutdown() {
	if a.uploadQ != nil {
		a.uploadQ.Shutdown()
	}
}

func readerSize(r io.Reader) (int64, error) {
	switch v := r.(type) {
	case *os.File:
		fi, err := v.Stat()
		if err != nil {
			return -1, err
		}
		return fi.Size(), nil
	case *strings.Reader:
		return int64(v.Len()), nil
	default:
		return -1, nil
	}
}
