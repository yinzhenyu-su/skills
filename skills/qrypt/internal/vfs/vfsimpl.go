package vfs

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/pkg/cipherdriver"
	vfsapi "github.com/yinzhenyu/skills/qrypt/pkg/vfs"
)

var _ vfsapi.VFS = (*VFSImpl)(nil)

// FSOptions configures filesystem-level behaviour.
type FSOptions struct {
	MaxRetries        int
	ConcurrentUploads int
	MemCacheSizeMB    int
	WriteBackTimeout  time.Duration
	NoAppleDouble     bool
}

type nodeState struct {
	mu sync.Mutex

	localPath  string
	isDirty    bool
	syncQueued bool
	source     string
	baseMtime  int64
	baseSize   int64
	lastUpload time.Time
	lastCheck  time.Time
}

// VFSImpl is the concrete implementation of vfsapi.VFS. It owns and wires
// together all sub-managers: tree, staging, cache, cipher driver, uploads,
// and the operation coalescer.
type VFSImpl struct {
	tree      *TreeManager
	staging   *qrypt.Store
	cache     *CacheLayer
	cipherDrv *cipherdriver.CipherDriver
	uploads   *UploadManager
	coalescer *OperationCoalescer
	executor  *OpExecutor
	diskCache *qrypt.CacheManager

	cp       *cipher.RcloneCipher
	rootFid  string
	merging  sync.Map
	states   sync.Map
	deleting sync.Map
}

// NewVFSImpl creates a fully-wired VFSImpl with the given rootFid.
// rootFid is the backend file identifier of the mount root (usually "0" for /).
// When root_path is configured, this should be the resolved FID of that path.
func NewVFSImpl(drv drivers.Driver, cp cipher.Cipher, cacheDir string, rootFid string, opts FSOptions) *VFSImpl {
	cipherDrv := cipherdriver.New(drv, cp)
	executor := NewOpExecutor(cipherDrv)

	var cpPtr *cipher.RcloneCipher
	if rc, ok := cp.(*cipher.RcloneCipher); ok {
		cpPtr = rc
	}

	stagingDir := filepath.Join(cacheDir, "staging")
	staging, err := qrypt.NewStore(stagingDir)
	if err != nil {
		logging.L.Errorf("vfs: failed to create staging store: %v", err)
	}

	vfsImpl := &VFSImpl{
		cipherDrv: cipherDrv,
		executor:  executor,
		staging:   staging,
		cp:        cpPtr,
		rootFid:   rootFid,
	}

	vfsImpl.tree = NewTreeManager(vfsImpl.rootFid)

	diskCache, err := qrypt.NewCacheManager(cacheDir, 10*1024*1024*1024)
	if err != nil {
		logging.L.Errorf("vfs: failed to create disk cache: %v", err)
	}
	vfsImpl.diskCache = diskCache

	cacheSizeMB := opts.MemCacheSizeMB
	if cacheSizeMB <= 0 {
		cacheSizeMB = 64
	}
	vfsImpl.cache = NewCacheLayer(cacheSizeMB, diskCache,
		func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
			node := vfsImpl.tree.LookupByFid(fid)
			if node == nil {
				return nil, fmt.Errorf("vfs: node not found for fid %s", fid)
			}
			rfid := node.RemoteFID()
			if rfid == "" {
				rfid = node.FID()
			}
			entry := drivers.Entry{
				ID:   rfid,
				Size: node.EncSize(),
			}
			return cipherDrv.ReadBlocks(ctx, entry, startBlock, count)
		})

	writeBackTimeout := opts.WriteBackTimeout
	if writeBackTimeout <= 0 {
		writeBackTimeout = 5 * time.Second
	}
	vfsImpl.uploads = NewUploadManager(executor, writeBackTimeout, vfsImpl)

	caps := Capabilities{BatchDelete: false, Rename: true, Move: true}
	if _, ok := drv.(drivers.BatchRemover); ok {
		caps.BatchDelete = true
	}
	vfsImpl.coalescer = NewOperationCoalescer(caps, vfsImpl.handleCoalescedIntents)

	rootNode := NewVFSNode(stagingID(), "0", "", "/", true)
	rootNode.SetUploaded(vfsImpl.rootFid)
	vfsImpl.tree.Store("/", rootNode)
	state := &nodeState{source: "remote"}
	vfsImpl.states.Store("/", state)

	return vfsImpl
}

// resolveParentFid returns the current parent FID for a given path/node.
// It looks up the parent directory in the tree and returns its remote FID.
func (vfs *VFSImpl) resolveParentFid(path string, n *VFSNode) string {
	parentPath := filepath.Dir(path)
	if p := vfs.tree.Lookup(parentPath); p != nil {
		if rfid := p.RemoteFID(); rfid != "" {
			return rfid
		}
	}
	return n.ParentFID()
}

func (vfs *VFSImpl) getState(path string) *nodeState {
	path = filepath.Clean(path)
	v, _ := vfs.states.Load(path)
	if v == nil {
		return nil
	}
	return v.(*nodeState)
}

func (vfs *VFSImpl) getOrCreateState(path string) *nodeState {
	path = filepath.Clean(path)
	v, _ := vfs.states.LoadOrStore(path, &nodeState{source: "remote"})
	return v.(*nodeState)
}

func (vfs *VFSImpl) Stat(ctx context.Context, path string) (vfsapi.NodeInfo, error) {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return vfsapi.NodeInfo{}, fmt.Errorf("not found: %s", path)
	}
	id := n.RemoteFID()
	if id == "" {
		id = n.FID()
	}
	return vfsapi.NodeInfo{
		ID:      id,
		Name:    n.Name(),
		IsDir:   n.IsFolder(),
		Size:    n.Size(),
		ModTime: n.ModTime(),
	}, nil
}

func (vfs *VFSImpl) ListDir(ctx context.Context, path string) ([]vfsapi.DirEntry, error) {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return nil, fmt.Errorf("not found: %s", path)
	}
	if !n.IsFolder() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	parentFid := n.RemoteFID()
	if parentFid == "" {
		parentFid = n.FID()
	}
	state := vfs.getOrCreateState(path)

	state.mu.Lock()
	lastCheck := state.lastCheck
	state.mu.Unlock()

	forceRefresh := false
	n.mu.RLock()
	childCount := len(n.children)
	n.mu.RUnlock()
	if childCount == 0 && time.Since(lastCheck) > 5*time.Second {
		forceRefresh = true
	}

	if time.Since(lastCheck) > 15*time.Second || forceRefresh {
		if forceRefresh {
			entries, err := vfs.cipherDrv.List(ctx, parentFid)
			if err == nil {
				vfs.mergeRemoteChanges(path, parentFid, entries)
			}
			state.mu.Lock()
			state.lastCheck = time.Now()
			state.mu.Unlock()
		} else {
			state.mu.Lock()
			state.lastCheck = time.Now()
			state.mu.Unlock()
			go vfs.refreshDirAsync(path, parentFid)
		}
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]vfsapi.DirEntry, 0, len(n.children))
	for name, child := range n.children {
		result = append(result, vfsapi.DirEntry{
			Name:  name,
			IsDir: child.IsFolder(),
			Size:  child.Size(),
		})
	}
	return result, nil
}

func (vfs *VFSImpl) Read(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return nil, fmt.Errorf("not found: %s", path)
	}
	if n.IsFolder() {
		return nil, fmt.Errorf("is a directory: %s", path)
	}

	nodeSize := n.Size()
	if offset >= nodeSize {
		return io.NopCloser(strings.NewReader("")), nil
	}

	state := vfs.getState(path)
	if state != nil && vfs.staging != nil {
		state.mu.Lock()
		lp := state.localPath
		state.mu.Unlock()
		if lp != "" && vfs.staging.Exists(lp) {
			return vfs.staging.OpenReader(lp)
		}
	}

	return &vfsReader{
		vfs:    vfs,
		node:   n,
		offset: offset,
	}, nil
}

type vfsReader struct {
	vfs    *VFSImpl
	node   *VFSNode
	offset int64
	pos    int64
}

func (r *vfsReader) Read(p []byte) (int, error) {
	nodeSize := r.node.Size()
	absPos := r.pos + r.offset
	if absPos >= nodeSize {
		return 0, io.EOF
	}

	startChunk := absPos / cipher.BlockDataSize
	blockIdx := uint64(startChunk)

	chunkData, err := r.vfs.cache.ReadBlock(context.Background(), r.node.FID(), blockIdx)
	if err != nil {
		return 0, err
	}
	r.vfs.cache.NotifyRead(r.node.FID(), blockIdx)

	chunkStart := absPos % cipher.BlockDataSize
	if chunkStart >= int64(len(chunkData)) {
		return 0, io.EOF
	}

	available := int64(len(chunkData)) - chunkStart
	remaining := nodeSize - absPos
	if available > remaining {
		available = remaining
	}

	n := copy(p, chunkData[chunkStart:chunkStart+available])
	if n == 0 {
		return 0, io.EOF
	}
	r.pos += int64(n)
	return n, nil
}

func (r *vfsReader) Close() error { return nil }

func (vfs *VFSImpl) Create(ctx context.Context, path string) error {
	parentPath := filepath.Dir(path)
	parentNode := vfs.tree.Lookup(parentPath)
	if parentNode == nil {
		return fmt.Errorf("parent not found: %s", parentPath)
	}
	name := filepath.Base(path)

	if !strings.Contains(name, ".") && !strings.HasPrefix(name, ".") {
		return vfs.Mkdir(ctx, path)
	}

	sid := stagingID()
	parentFid := parentNode.RemoteFID()
	if parentFid == "" {
		parentFid = parentNode.FID()
	}

	n := NewVFSNode(sid, parentFid, name, path, false)
	state := vfs.getOrCreateState(path)

	var localPath string
	if vfs.staging != nil {
		var err error
		localPath, err = vfs.staging.Create(sid)
		if err != nil {
			return fmt.Errorf("create staging: %w", err)
		}
	}

	state.mu.Lock()
	state.isDirty = true
	state.source = "local"
	if localPath != "" {
		state.localPath = localPath
	}
	state.mu.Unlock()

	vfs.tree.Store(path, n)

	return nil
}

func (vfs *VFSImpl) Write(ctx context.Context, path string, offset int64, reader io.Reader) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	if n.IsFolder() {
		return fmt.Errorf("is a directory: %s", path)
	}

	state := vfs.getOrCreateState(path)
	if vfs.staging == nil {
		return fmt.Errorf("staging unavailable")
	}

	state.mu.Lock()
	localPath := state.localPath
	state.mu.Unlock()

	if localPath == "" {
		var err error
		localPath, err = vfs.staging.Create(stagingID())
		if err != nil {
			return fmt.Errorf("create staging for write: %w", err)
		}
		state.mu.Lock()
		state.localPath = localPath
		state.mu.Unlock()
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	written, err := vfs.staging.WriteAt(localPath, data, offset)
	if err != nil {
		return err
	}

	state.mu.Lock()
	state.isDirty = true
	state.mu.Unlock()
	n.SetModTime(time.Now())
	newSize := offset + int64(written)
	if newSize < n.Size() && vfs.staging != nil {
		vfs.staging.Truncate(localPath, newSize)
	}
	n.SetSize(newSize)

	var oldFid string
	if state != nil {
		if n.IsUploaded() {
			oldFid = n.RemoteFID()
		}
	}
	parentFid := vfs.resolveParentFid(path, n)
	vfs.uploads.MarkDirty(path, parentFid, n.Name(), n.Size(), oldFid, func() (io.ReadCloser, error) {
		state.mu.Lock()
		lp := state.localPath
		state.mu.Unlock()
		return vfs.staging.OpenReader(lp)
	})

	return nil
}

func (vfs *VFSImpl) Mkdir(ctx context.Context, path string) error {
	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode := vfs.tree.Lookup(parentPath)
	if parentNode == nil {
		return fmt.Errorf("parent not found: %s", parentPath)
	}

	parentRemoteFid := parentNode.RemoteFID()
	if parentRemoteFid == "" {
		parentRemoteFid = parentNode.FID()
	}
	var entry drivers.Entry
	var err error
	for retry := 0; retry < 4; retry++ {
		entry, err = vfs.executor.Mkdir(ctx, parentRemoteFid, name)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "同名冲突") {
			// Directory exists on remote (from previous upload or concurrent
			// operation). Fall through to the fallback list below.
			time.Sleep(time.Duration(retry) * 500 * time.Millisecond)
			break
		}
		logging.L.Warnf("mkdir: remote error for %s (parentFid=%s): %v\n", path, parentRemoteFid, err)
		return fmt.Errorf("mkdir remote: %w", err)
	}
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "同名冲突") {
			return err
		}

		if entries, listErr := vfs.cipherDrv.List(ctx, parentRemoteFid); listErr == nil {
			for _, e := range entries {
				if e.Name == name {
					entry = e
					err = nil
					break
				}
			}
		}
		if err != nil {
			logging.L.Warnf("mkdir: fallback list failed for %s parentFid=%s: %v\n", path, parentRemoteFid, err)
			return err
		}
	}

	if entry.ID == "" {
		return fmt.Errorf("mkdir: remote returned empty fid for %s", path)
	}

	// If the node already exists in the tree (e.g. "already exists" fallback
	// or concurrent Mkdir), update its remote FID without losing children.
	if existing := vfs.tree.Lookup(path); existing != nil {
		existing.SetUploaded(entry.ID)
		vfs.tree.Store(path, existing)
		return nil
	}

	parentFid := parentNode.RemoteFID()
	if parentFid == "" {
		parentFid = parentNode.FID()
	}
	n := NewVFSNode(stagingID(), parentFid, name, path, true)
	n.SetUploaded(entry.ID)
	vfs.tree.Store(path, n)

	state := vfs.getOrCreateState(path)
	state.mu.Lock()
	state.source = "remote"
	state.mu.Unlock()

	return nil
}

func (vfs *VFSImpl) Remove(ctx context.Context, path string) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	return vfs.removeNode(path, n)
}

func (vfs *VFSImpl) removeNode(path string, n *VFSNode) error {
	if n.IsFolder() {
		return vfs.rmdir(path, n)
	}
	return vfs.unlink(path, n)
}

func (vfs *VFSImpl) unlink(path string, n *VFSNode) error {
	fid := n.FID()
	parentFid := n.ParentFID()

	vfs.coalescer.Submit(FileOp{
		Kind:      OpDelete,
		Path:      path,
		Fid:       fid,
		ParentFid: parentFid,
		IsDir:     false,
		Time:      time.Now(),
		Node:      n,
	})

	vfs.uploads.CancelSync(path)
	vfs.deleting.Store(path, true)
	vfs.states.Delete(path)
	vfs.tree.Delete(path, n)
	return nil
}

func (vfs *VFSImpl) rmdir(path string, n *VFSNode) error {
	parentFid := n.ParentFID()
	fid := n.FID()

	var childFids []string
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	n.mu.RLock()
	for _, child := range n.children {
		if child.IsUploaded() {
			childFids = append(childFids, child.RemoteFID())
		}
	}
	n.mu.RUnlock()

	vfs.coalescer.Submit(FileOp{
		Kind:      OpDelete,
		Path:      path,
		Fid:       fid,
		ParentFid: parentFid,
		IsDir:     true,
		Fids:      childFids,
		Time:      time.Now(),
		Node:      n,
	})

	vfs.uploads.CancelSync(path)
	vfs.deleting.Store(path, true)
	vfs.states.Delete(path)
	vfs.tree.Delete(path, n)
	return nil
}

func (vfs *VFSImpl) Rename(ctx context.Context, oldPath, newPath string) error {
	oldNode := vfs.tree.Lookup(oldPath)
	if oldNode == nil {
		return fmt.Errorf("not found: %s", oldPath)
	}

	vfs.coalescer.Submit(FileOp{
		Kind:      OpRename,
		Path:      oldPath,
		NewPath:   newPath,
		Fid:       oldNode.FID(),
		ParentFid: oldNode.ParentFID(),
		IsDir:     oldNode.IsFolder(),
		Time:      time.Now(),
		Node:      oldNode,
	})

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	if oldParent != newParent {
		newParentNode := vfs.tree.Lookup(newParent)
		if newParentNode != nil {
			oldNode.parentFid = newParentNode.FID()
		}
	}
	oldNode.name = filepath.Base(newPath)

	vfs.tree.ReplacePath(oldPath, newPath, oldNode)

	if state := vfs.getState(oldPath); state != nil {
		vfs.states.Delete(oldPath)
		vfs.states.Store(filepath.Clean(newPath), state)
	}
	vfs.uploads.CancelSync(oldPath)
	return nil
}

func (vfs *VFSImpl) Truncate(ctx context.Context, path string, size int64) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	if n.IsFolder() {
		return fmt.Errorf("is a directory: %s", path)
	}
	if size < 0 {
		return fmt.Errorf("invalid size: %d", size)
	}

	oldSize := n.Size()
	n.SetSize(size)

	state := vfs.getOrCreateState(path)

	state.mu.Lock()
	state.isDirty = true
	state.mu.Unlock()
	n.SetModTime(time.Now())

	if vfs.staging == nil {
		return fmt.Errorf("staging unavailable")
	}

	state.mu.Lock()
	localPath := state.localPath
	state.mu.Unlock()

	if localPath == "" {
		var err error
		localPath, err = vfs.staging.Create(stagingID())
		if err != nil {
			return fmt.Errorf("create staging for truncate: %w", err)
		}
		if oldSize > 0 && n.IsUploaded() {
			entry := drivers.Entry{ID: n.RemoteFID(), Size: oldSize}
			rc, err := vfs.cipherDrv.Read(ctx, entry, 0, oldSize)
			if err == nil {
				defer rc.Close()
				data, _ := io.ReadAll(rc)
				vfs.staging.WriteAt(localPath, data, 0)
			}
		}
		state.mu.Lock()
		state.localPath = localPath
		state.mu.Unlock()
	}

	if err := vfs.staging.Truncate(localPath, size); err != nil {
		return err
	}

	vfs.uploads.MarkDirty(path, vfs.resolveParentFid(path, n), n.Name(), size, "", func() (io.ReadCloser, error) {
		state.mu.Lock()
		lp := state.localPath
		state.mu.Unlock()
		return vfs.staging.OpenReader(lp)
	})

	return nil
}

func (vfs *VFSImpl) Utimens(ctx context.Context, path string, mtime time.Time) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	n.SetModTime(mtime)
	return nil
}

func (vfs *VFSImpl) Flush(ctx context.Context, path string) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	state := vfs.getState(path)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	isDirty := state.isDirty
	localPath := state.localPath
	state.mu.Unlock()
	if !isDirty {
		return nil
	}
	if localPath != "" && vfs.staging != nil {
		vfs.staging.Sync(localPath)
	}
	vfs.coalescer.Submit(FileOp{
		Kind:      OpUpload,
		Path:      path,
		Fid:       n.FID(),
		ParentFid: n.ParentFID(),
		Staging:   localPath,
		Time:      time.Now(),
		Node:      n,
	})
	return nil
}

func (vfs *VFSImpl) Fsync(ctx context.Context, path string) error {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return fmt.Errorf("not found: %s", path)
	}
	state := vfs.getState(path)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	localPath := state.localPath
	state.mu.Unlock()
	if localPath != "" && vfs.staging != nil {
		return vfs.staging.Sync(localPath)
	}
	return nil
}

func (vfs *VFSImpl) handleCoalescedIntents(intents []RemoteIntent) {
	for _, intent := range intents {
		if intent.SkipRemote {
			vfs.cleanupLocal(intent)
			continue
		}
		switch intent.Kind {
		case "UPLOAD":
			if intent.Node != nil {
				state := vfs.getOrCreateState(intent.Node.CurrentPath())
				state.mu.Lock()
				state.syncQueued = true
				state.mu.Unlock()
				parentFid := intent.ParentFid
				if p := vfs.tree.Lookup(filepath.Dir(intent.Path)); p != nil {
					if rfid := p.RemoteFID(); rfid != "" {
						parentFid = rfid
					}
				}
				vfs.uploads.MarkDirty(
					intent.Path,
					parentFid,
					filepath.Base(intent.Path),
					intent.Node.Size(),
					"",
					func() (io.ReadCloser, error) {
						state := vfs.getState(intent.Path)
						if state == nil || vfs.staging == nil {
							return nil, fmt.Errorf("staging not found for %s", intent.Path)
						}
						state.mu.Lock()
						lp := state.localPath
						state.mu.Unlock()
						if lp == "" {
							return nil, fmt.Errorf("staging not found for %s", intent.Path)
						}
						return vfs.staging.OpenReader(lp)
					})
			}
		case "DELETE":
			vfs.executeDelete(intent)
		case "RENAME":
			vfs.executeCoalescedRename(intent)
		case "MKDIR":
			vfs.executeCoalescedMkdir(intent)
		}
	}
}

func (vfs *VFSImpl) executeDelete(intent RemoteIntent) {
	fids := intent.Fids
	rfid := intent.Fid
	if intent.Node != nil && intent.Node.IsUploaded() {
		rfid = intent.Node.RemoteFID()
	}
	if rfid != "" {
		found := false
		for _, f := range fids {
			if f == rfid {
				found = true
				break
			}
		}
		if !found {
			fids = append(fids, rfid)
		}
	}

	var remoteFids []drivers.Entry
	for _, fid := range fids {
		if fid != "" {
			remoteFids = append(remoteFids, drivers.Entry{ID: fid})
		}
	}

	if len(remoteFids) == 0 {
		vfs.cleanupLocal(intent)
		return
	}

	go func() {
		for attempt := 0; attempt < 3; attempt++ {
			err := vfs.executor.BatchRemove(context.Background(), remoteFids)
			if err == nil {
				logging.L.Infof("delete: removed %d fids for %s\n", len(remoteFids), intent.Path)
				vfs.cleanupLocal(intent)
				return
			}
			logging.L.Warnf("delete: attempt %d/3 failed for %s: %v\n", attempt+1, intent.Path, err)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
		logging.L.Errorf("delete: FAILED for %s after 3 attempts\n", intent.Path)
	}()
}

func (vfs *VFSImpl) cleanupLocal(intent RemoteIntent) {
	if intent.Node != nil {
		state := vfs.getState(intent.Node.CurrentPath())
		if state != nil && vfs.staging != nil {
			state.mu.Lock()
			lp := state.localPath
			state.mu.Unlock()
			if lp != "" {
				vfs.staging.Remove(lp)
			}
		}
	}
	if intent.Staging != "" && vfs.staging != nil {
		vfs.staging.Remove(intent.Staging)
	}
}

func (vfs *VFSImpl) executeCoalescedRename(intent RemoteIntent) {
	fid := intent.Fid
	newPath := intent.NewPath
	if intent.Node != nil && intent.Node.IsUploaded() {
		fid = intent.Node.RemoteFID()
	}
	if fid == "" || newPath == "" {
		return
	}

	oldName := filepath.Base(intent.Path)
	newName := filepath.Base(newPath)
	if vfs.cp == nil {
		return
	}
	encName := vfs.cp.EncryptSegment(newName)
	oldEncName := vfs.cp.EncryptSegment(oldName)

	if encName != oldEncName {
		entry := drivers.Entry{ID: fid}
		for attempt := 0; attempt < 5; attempt++ {
			err := vfs.executor.Rename(context.Background(), entry, newName)
			if err != nil {
				if strings.Contains(err.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				logging.L.Errorf("coalesced rename: %s -> %s failed: %v\n", intent.Path, newPath, err)
				return
			}
			break
		}
	}
}

func (vfs *VFSImpl) executeCoalescedMkdir(intent RemoteIntent) {
	path := intent.Path
	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode := vfs.tree.Lookup(parentPath)
	if parentNode == nil {
		logging.L.Warnf("coalesced mkdir: parent lookup failed for %s\n", path)
		return
	}

	parentRemoteFid := parentNode.RemoteFID()
	if parentRemoteFid == "" {
		parentRemoteFid = parentNode.FID()
	}
	entry, err := vfs.executor.Mkdir(context.Background(), parentRemoteFid, name)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return
		}
		logging.L.Warnf("coalesced mkdir: failed for %s: %v\n", path, err)
		return
	}

	if intent.Node != nil && entry.ID != "" {
		intent.Node.SetUploaded(entry.ID)
		vfs.tree.Store(path, intent.Node)
		state := vfs.getOrCreateState(path)
		state.mu.Lock()
		state.source = "remote"
		state.mu.Unlock()
	}
}

func (vfs *VFSImpl) OnUploadComplete(path string, newFid string, encSize int64, err error) {
	n := vfs.tree.Lookup(path)
	if n == nil {
		return
	}
	state := vfs.getState(path)
	if state == nil {
		return
	}

	if err != nil {
		logging.L.Warnf("upload: failed for %s: %v\n", path, err)
		state.mu.Lock()
		state.syncQueued = false
		state.mu.Unlock()
		return
	}

	logging.L.Infof("upload: succeeded for %s\n", path)

	state.mu.Lock()
	state.isDirty = false
	state.syncQueued = false
	state.lastUpload = time.Now()
	lp := state.localPath
	state.localPath = ""
	state.mu.Unlock()

	if lp != "" && vfs.staging != nil {
		vfs.staging.Remove(lp)
	}

	if newFid != "" {
		n.SetUploaded(newFid)
		if encSize > 0 {
			n.SetEncSize(encSize)
		}
		vfs.tree.Store(path, n)
	}

	if vfs.diskCache != nil {
		vfs.diskCache.RemovePendingNode(path)
	}
}

func (vfs *VFSImpl) InvalidateDirCache(path string) {
	n := vfs.tree.Lookup(path)
	if n != nil {
		if n.IsUploaded() {
			vfs.tree.fidNodes.Delete(n.RemoteFID())
		}
		vfs.tree.nodes.Delete(filepath.Clean(path))
	}
}

func (vfs *VFSImpl) refreshDirAsync(path, parentFid string) {
	entries, err := vfs.cipherDrv.List(context.Background(), parentFid)
	if err != nil {
		return
	}
	_ = entries
	vfs.mergeRemoteChanges(path, parentFid, entries)
	state := vfs.getOrCreateState(path)
	state.mu.Lock()
	state.lastCheck = time.Now()
	state.mu.Unlock()
}

func (vfs *VFSImpl) mergeRemoteChanges(parentPath, parentFid string, remoteFiles []drivers.Entry) {
	waitChan, loading := vfs.merging.LoadOrStore(parentFid, make(chan struct{}))
	if loading {
		<-waitChan.(chan struct{})
		return
	}
	defer func() {
		close(waitChan.(chan struct{}))
		vfs.merging.Delete(parentFid)
	}()

	remoteMap := make(map[string]drivers.Entry)
	for _, f := range remoteFiles {
		if f.ID == parentFid {
			continue
		}
		remoteMap[f.Name] = f
	}

	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	type nodeEntry struct {
		path string
		node *VFSNode
	}
	var localEntries []nodeEntry

	n := vfs.tree.Lookup(parentPath)
	if n != nil {
		n.mu.RLock()
		for name, child := range n.children {
			localEntries = append(localEntries, nodeEntry{path: prefix + name, node: child})
		}
		n.mu.RUnlock()
	}

	seenLocalNames := make(map[string]bool)

	for _, entry := range localEntries {
		nn := entry.node
		name := nn.Name()
		fid := nn.FID()
		seenLocalNames[name] = true

		state := vfs.getState(entry.path)
		if state != nil {
			state.mu.Lock()
			sq := state.syncQueued
			state.mu.Unlock()
			if sq {
				continue
			}
		}

		rf, exists := remoteMap[name]
		if !exists {
			continue
		}

		if state != nil {
			state.mu.Lock()
			if !state.isDirty && (state.source == "local" || state.source == "merged" ||
				(!state.lastUpload.IsZero() && time.Since(state.lastUpload) < 30*time.Second)) {
				if rf.ID == fid {
					state.source = "remote"
				}
			}
			state.mu.Unlock()
		}

		if nn.IsUploaded() {
			existingRemote := nn.RemoteFID()
			if existingRemote != "" && rf.ID != existingRemote {
				if state == nil || !state.isDirty {
					if state == nil || state.lastUpload.IsZero() || time.Since(state.lastUpload) >= 30*time.Second {
						vfs.cache.InvalidateFid(existingRemote)
						nn.SetUploaded(rf.ID)
						state = vfs.getOrCreateState(entry.path)
						state.mu.Lock()
						state.source = "remote"
						state.mu.Unlock()
					}
				}

				remoteMtime := rf.ModTime.UnixMilli()
				if state != nil && state.source == "remote" && state.baseMtime > 0 && remoteMtime > state.baseMtime+2000 {
					if !state.isDirty {
						decSize, errDec := vfs.cp.DecryptedSize(rf.Size)
						if errDec == nil {
							nn.SetUploaded(rf.ID)
							nn.SetSize(decSize)
							nn.SetEncSize(rf.Size)
							nn.SetModTime(rf.ModTime)
							state.mu.Lock()
							state.baseMtime = remoteMtime
							state.baseSize = decSize
							state.lastCheck = time.Now()
							state.source = "remote"
							state.mu.Unlock()
						}
					}
				}
			}
		}
	}

	for name, rf := range remoteMap {
		if seenLocalNames[name] {
			continue
		}

		childPath := prefix + name
		if _, deleting := vfs.deleting.Load(childPath); deleting {
			continue
		}
		decSize, errDec := vfs.cp.DecryptedSize(rf.Size)
		if errDec != nil {
			decSize = 0
		}
		modTime := rf.ModTime
		lastCheck := time.Now()
		if rf.IsDir {
			lastCheck = time.Time{}
		}

		child := NewVFSNode(stagingID(), parentFid, name, childPath, rf.IsDir)
		child.SetUploaded(rf.ID)
		child.SetSize(decSize)
		child.SetEncSize(rf.Size)
		child.SetModTime(modTime)

		vfs.tree.Store(childPath, child)

		state := vfs.getOrCreateState(childPath)
		state.mu.Lock()
		state.source = "remote"
		state.baseMtime = modTime.UnixMilli()
		state.baseSize = decSize
		state.lastCheck = lastCheck
		state.mu.Unlock()
	}

	// Clean up deleting entries that the remote has confirmed gone.
	// We must NOT delete the guard in cleanupLocal (executeDelete runs
	// async, and a subsequent remote refresh may still see the item due
	// to API caching, causing re-creation).
	vfs.deleting.Range(func(key, value interface{}) bool {
		dPath := key.(string)
		if !strings.HasPrefix(dPath, prefix) {
			return true
		}
		name := strings.TrimPrefix(dPath, prefix)
		if _, exists := remoteMap[name]; !exists {
			vfs.deleting.Delete(dPath)
		}
		return true
	})
}

func (vfs *VFSImpl) Shutdown() {
	vfs.coalescer.Shutdown()
}
