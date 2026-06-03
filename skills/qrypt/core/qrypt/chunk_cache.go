package qrypt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// OpsLogEntry is retained for compatibility with external callers
// (fs/workers.go logOpsBatch / replayOpsLog). Internally all journal
// entries are stored as JournalEntry in a single pending.jsonl file.
type OpsLogEntry struct {
	OpType    string `json:"op"`
	Path      string `json:"path"`
	Fid       string `json:"fid,omitempty"`
	Timestamp int64  `json:"ts"`
	Done      bool   `json:"done"`
}

// JournalOp values
const (
	JOpDelete    = "delete"
	JOpDeleteDone = "delete_done"
	JOpDirty    = "dirty"
	JOpUpdate   = "update"
	JOpClean    = "clean"
)

// JournalEntry is the unified log entry for pending.jsonl.
// It covers both remote operations (delete) and local dirty-file state
// (dirty, update, clean).
type JournalEntry struct {
	Op        string `json:"op"`
	Path      string `json:"path,omitempty"`
	Fid       string `json:"fid,omitempty"`
	Timestamp int64  `json:"ts,omitempty"`
	// Dirty-file fields (dirty / update / clean)
	ParentFid string `json:"parent_fid,omitempty"`
	Name      string `json:"name,omitempty"`
	Stg       string `json:"stg,omitempty"`
	Size      int64  `json:"size,omitempty"`
	IsFolder  bool   `json:"is_folder,omitempty"`
	Nonce     []byte `json:"nonce,omitempty"`
	BaseMtime int64  `json:"base_mtime,omitempty"`
	BaseSize  int64  `json:"base_size,omitempty"`
	UploadID  string `json:"upload_id,omitempty"`
	LastPart  int    `json:"last_part,omitempty"`
}

const CacheBatchBlocks = 16

type ChunkInfo struct {
	FilePath  string
	Offset    int64
	Size      int64
	IsDirty   bool
	AccessAt  time.Time
}

type PendingNode struct {
	Path            string
	Fid             string
	ParentFid       string
	Name            string
	LocalPath       string
	Size            int64
	IsFolder        bool
	Nonce           []byte
	BaseServerMtime int64
	BaseServerSize  int64
	UploadID        string
	LastPart        int
}

type StagingMeta struct {
	Fid       string
	LocalPath string
	Size      int64
	Status    string
	UpdatedAt time.Time
}

type fileChunkCache struct {
	mu     sync.RWMutex
	chunks map[int64]*ChunkInfo
}

type CacheManager struct {
	cacheDir string
	maxSize  int64
	staging  *Store
	evictCount int64

	mu            sync.RWMutex
	pendingNodes  map[string]*PendingNode
	stagingMetas  map[string]*StagingMeta
	chunkIndex    map[string]*fileChunkCache
}

func (m *CacheManager) CacheDir() string {
	return m.cacheDir
}

func (m *CacheManager) StagingDir() string {
	return filepath.Join(m.cacheDir, "staging")
}

func (m *CacheManager) ReadingDir() string {
	return filepath.Join(m.cacheDir, "reading")
}

func (m *CacheManager) Staging() *Store {
	return m.staging
}

func NewCacheManager(cacheDir string, maxSize int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	stagingDir := filepath.Join(cacheDir, "staging")
	store, err := NewStore(stagingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging store: %w", err)
	}

	readingDir := filepath.Join(cacheDir, "reading")
	if err := os.MkdirAll(readingDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create reading cache dir: %w", err)
	}

	m := &CacheManager{
		cacheDir:     cacheDir,
		maxSize:      maxSize,
		staging:      store,
		pendingNodes: make(map[string]*PendingNode),
		stagingMetas: make(map[string]*StagingMeta),
		chunkIndex:   make(map[string]*fileChunkCache),
	}

	// Remove old journal formats after migration to unified pending.jsonl.
	os.Remove(filepath.Join(cacheDir, "ops.jsonl"))
	os.Remove(filepath.Join(cacheDir, "pending.journal"))

	// Recover state from unified journal before cleaning up.
	deleteOps, recovered, err := m.loadJournal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadJournal: %v (proceeding with empty state)\n", err)
	} else {
		for path, pn := range recovered {
			m.pendingNodes[path] = pn
		}
		_ = deleteOps
	}

	m.cleanupOrphanedStagingFiles()

	return m, nil
}

func (m *CacheManager) cleanupOrphanedStagingFiles() {
	m.mu.RLock()
	activeFids := make(map[string]bool)
	for _, n := range m.pendingNodes {
		if n.Fid != "" {
			activeFids[n.Fid] = true
		}
	}
	m.mu.RUnlock()

	cleaned, err := m.staging.CleanupOrphanedStagingFiles(activeFids)
	if err != nil {
		fmt.Printf("cleanupOrphanedStagingFiles: failed: %v\n", err)
		return
	}
	if len(cleaned) > 0 {
		fmt.Printf("cleanupOrphanedStagingFiles: removed %d orphaned staging files\n", len(cleaned))
	}
}

// --- Staging Meta (MetaStore interface) ---

func (m *CacheManager) SaveStagingMeta(fid, localPath string, size int64) error {
	m.mu.Lock()
	m.stagingMetas[fid] = &StagingMeta{
		Fid:       fid,
		LocalPath: localPath,
		Size:      size,
		Status:    "active",
		UpdatedAt: time.Now(),
	}
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) UpdateStagingMeta(fid string, size int64) error {
	m.mu.Lock()
	if s, ok := m.stagingMetas[fid]; ok {
		s.Size = size
		s.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) RemoveStagingMeta(fid string) error {
	m.mu.Lock()
	delete(m.stagingMetas, fid)
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) GetStagingMeta(fid string) *StagingMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stagingMetas[fid]
}

// --- Chunk Cache ---

func (m *CacheManager) getOrCreateChunkCache(fid string) *fileChunkCache {
	m.mu.Lock()
	defer m.mu.Unlock()
	fc, ok := m.chunkIndex[fid]
	if !ok {
		fc = &fileChunkCache{chunks: make(map[int64]*ChunkInfo)}
		m.chunkIndex[fid] = fc
	}
	return fc
}

func (m *CacheManager) GetChunk(fid string, chunkIndex int64) ([]byte, error) {
	fc := m.getOrCreateChunkCache(fid)
	fc.mu.RLock()
	ci, ok := fc.chunks[chunkIndex]
	if !ok {
		fc.mu.RUnlock()
		return nil, nil
	}
	fc.mu.RUnlock()

	var data []byte
	if ci.Offset > 0 || ci.Size > 0 {
		f, err := os.Open(ci.FilePath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data = make([]byte, ci.Size)
		if _, err := f.ReadAt(data, ci.Offset); err != nil {
			return nil, err
		}
	} else {
		var err error
		data, err = os.ReadFile(ci.FilePath)
		if err != nil {
			return nil, err
		}
	}

	ci.AccessAt = time.Now()
	return data, nil
}

func (m *CacheManager) HasChunk(fid string, chunkIndex int64) (bool, error) {
	fc := m.getOrCreateChunkCache(fid)
	fc.mu.RLock()
	_, ok := fc.chunks[chunkIndex]
	fc.mu.RUnlock()
	return ok, nil
}

func (m *CacheManager) PutChunk(fid string, chunkIndex int64, data []byte, isDirty bool) error {
	suffix := ".dec.batch"
	dir := m.cacheDir
	if isDirty {
		suffix = ".dirty.batch"
	} else {
		dir = m.ReadingDir()
	}
	batchIdx := chunkIndex / CacheBatchBlocks
	offset := int64(chunkIndex%CacheBatchBlocks) * int64(len(data))
	fileName := fmt.Sprintf("%s_batch_%d%s", fid, batchIdx, suffix)
	path := filepath.Join(dir, fileName)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.WriteAt(data, offset); err != nil {
		f.Close()
		return err
	}
	f.Close()

	fc := m.getOrCreateChunkCache(fid)
	fc.mu.Lock()
	fc.chunks[chunkIndex] = &ChunkInfo{
		FilePath: path,
		Offset:   offset,
		Size:     int64(len(data)),
		IsDirty:  isDirty,
		AccessAt: time.Now(),
	}
	fc.mu.Unlock()

	m.evictCount++
	if m.evictCount%100 == 0 {
		go m.EvictIfNeeded(m.maxSize * 7 / 10)
	}

	return nil
}

func (m *CacheManager) RemoveChunksByFid(fid string) error {
	fc := m.getOrCreateChunkCache(fid)
	fc.mu.Lock()
	paths := make(map[string]bool)
	for _, ci := range fc.chunks {
		paths[ci.FilePath] = true
	}
	delete(m.chunkIndex, fid)
	fc.mu.Unlock()

	for p := range paths {
		os.Remove(p)
	}
	return nil
}

func (m *CacheManager) GetDirtyChunks(fid string) []int64 {
	fc := m.getOrCreateChunkCache(fid)
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	var indices []int64
	for idx, ci := range fc.chunks {
		if ci.IsDirty {
			indices = append(indices, idx)
		}
	}
	return indices
}

func (m *CacheManager) getTotalChunkSize() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total int64
	for _, fc := range m.chunkIndex {
		fc.mu.RLock()
		for _, ci := range fc.chunks {
			total += ci.Size
		}
		fc.mu.RUnlock()
	}
	return total
}

// --- Pending Nodes ---

func (m *CacheManager) SavePendingNode(path, fid, parentFid, name, localPath string, size int64, isFolder bool, nonce []byte, baseMtime, baseSize int64, uploadID string, lastPart int) error {
	m.mu.Lock()
	m.pendingNodes[path] = &PendingNode{
		Path:            path,
		Fid:             fid,
		ParentFid:       parentFid,
		Name:            name,
		LocalPath:       localPath,
		Size:            size,
		IsFolder:        isFolder,
		Nonce:           nonce,
		BaseServerMtime: baseMtime,
		BaseServerSize:  baseSize,
		UploadID:        uploadID,
		LastPart:        lastPart,
	}
	m.mu.Unlock()

	m.appendJournal(&JournalEntry{
		Op:        JOpDirty,
		Path:      path,
		Fid:       fid,
		ParentFid: parentFid,
		Name:      name,
		Stg:       localPath,
		Size:      size,
		IsFolder:  isFolder,
		Nonce:     nonce,
		BaseMtime: baseMtime,
		BaseSize:  baseSize,
		UploadID:  uploadID,
		LastPart:  lastPart,
	})
	return nil
}

func (m *CacheManager) RemovePendingNode(path string) error {
	m.mu.Lock()
	delete(m.pendingNodes, path)
	m.mu.Unlock()

	m.appendJournal(&JournalEntry{Op: JOpClean, Path: path})

	// Compact after every clean so stale entries do not accumulate.
	// Without this, clean entries pile up when the dirty side is throttled
	// by maybeSavePendingNodeLocked (250ms / 1MB) while RemovePendingNode
	// is called for every completed upload.
	m.compactJournal()
	return nil
}

func (m *CacheManager) UpdatePendingNodeUpload(path, uploadID string) error {
	m.mu.Lock()
	if n, ok := m.pendingNodes[path]; ok {
		n.UploadID = uploadID
	}
	m.mu.Unlock()

	m.appendJournal(&JournalEntry{Op: JOpUpdate, Path: path, UploadID: uploadID})
	return nil
}

func (m *CacheManager) UpdatePendingNodeLastPart(path string, lastPart int) error {
	m.mu.Lock()
	if n, ok := m.pendingNodes[path]; ok {
		n.LastPart = lastPart
	}
	m.mu.Unlock()

	m.appendJournal(&JournalEntry{Op: JOpUpdate, Path: path, LastPart: lastPart})
	return nil
}

func (m *CacheManager) RemovePendingNodesByPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	like := prefix
	if like[len(like)-1] != '/' {
		like += "/"
	}
	m.mu.Lock()
	for path := range m.pendingNodes {
		if path == prefix || (len(path) > len(like) && path[:len(like)] == like) {
			delete(m.pendingNodes, path)
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) RemovePendingNodesByFid(fid string) error {
	if fid == "" {
		return nil
	}
	m.mu.Lock()
	for path, n := range m.pendingNodes {
		if n.Fid == fid {
			delete(m.pendingNodes, path)
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) GetPendingNodes() []PendingNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nodes := make([]PendingNode, 0, len(m.pendingNodes))
	for _, n := range m.pendingNodes {
		nn := *n
		if nn.Nonce != nil {
			nn.Nonce = append([]byte(nil), n.Nonce...)
		}
		nodes = append(nodes, nn)
	}
	return nodes
}

// --- Cache Eviction ---

func (m *CacheManager) EvictIfNeeded(lowWatermark int64) error {
	total := m.getTotalChunkSize()
	if total <= m.maxSize {
		return nil
	}

	targetEvict := total - lowWatermark

	m.mu.Lock()
	var allChunks []struct {
		fid string
		idx int64
		ci  *ChunkInfo
	}
	for fid, fc := range m.chunkIndex {
		fc.mu.RLock()
		for idx, ci := range fc.chunks {
			if !ci.IsDirty {
				allChunks = append(allChunks, struct {
					fid string
					idx int64
					ci  *ChunkInfo
				}{fid, idx, ci})
			}
		}
		fc.mu.RUnlock()
	}
	m.mu.Unlock()

	sortByAccessTime(allChunks)

	var evicted int64
	deletedPaths := make(map[string]bool)
	for _, c := range allChunks {
		if evicted >= targetEvict {
			break
		}
		if !deletedPaths[c.ci.FilePath] {
			os.Remove(c.ci.FilePath)
			deletedPaths[c.ci.FilePath] = true
		}
		evicted += c.ci.Size

		fc := m.getOrCreateChunkCache(c.fid)
		fc.mu.Lock()
		delete(fc.chunks, c.idx)
		fc.mu.Unlock()
	}

	return nil
}

func sortByAccessTime(chunks []struct {
	fid string
	idx int64
	ci  *ChunkInfo
}) {
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ci.AccessAt.Before(chunks[j].ci.AccessAt)
	})
}

// --- Maintenance ---

const maintenanceInterval = 10 * time.Minute

// --- Unified Journal (pending.jsonl) ---

func (m *CacheManager) journalPath() string {
	return filepath.Join(m.cacheDir, "pending.jsonl")
}

func (m *CacheManager) appendJournal(entry *JournalEntry) error {
	entry.Timestamp = time.Now().UnixNano()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(m.journalPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// loadJournal reads pending.jsonl and returns:
//   - deleteOps: pending delete entries (those not yet completed via delete_done)
//   - pending:   dirty-file nodes (path → PendingNode, cross-checked against staging)
func (m *CacheManager) loadJournal() (deleteOps []OpsLogEntry, pending map[string]*PendingNode, err error) {
	f, err := os.Open(m.journalPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer f.Close()

	type dirtyState struct {
		entry  *JournalEntry
		update *JournalEntry
		cleaned bool
	}
	dirtyByPath := make(map[string]*dirtyState)
	order := make([]string, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Op {
		case JOpDelete:
			deleteOps = append(deleteOps, OpsLogEntry{
				OpType:    "DELETE",
				Path:      entry.Path,
				Fid:       entry.Fid,
				Timestamp: entry.Timestamp,
				Done:      false,
			})
		case JOpDeleteDone:
			for i := range deleteOps {
				if deleteOps[i].Path == entry.Path {
					deleteOps[i].Done = true
				}
			}
		case JOpDirty:
			if _, ok := dirtyByPath[entry.Path]; !ok {
				order = append(order, entry.Path)
			}
			dirtyByPath[entry.Path] = &dirtyState{
				entry:  &entry,
				update: nil,
				cleaned: false,
			}
		case JOpUpdate:
			if s, ok := dirtyByPath[entry.Path]; ok && !s.cleaned {
				if s.update == nil {
					s.update = &JournalEntry{}
				}
				if entry.UploadID != "" {
					s.update.UploadID = entry.UploadID
				}
				if entry.LastPart > 0 {
					s.update.LastPart = entry.LastPart
				}
				if entry.Size > 0 {
					s.update.Size = entry.Size
				}
			}
		case JOpClean:
			if _, ok := dirtyByPath[entry.Path]; ok {
				dirtyByPath[entry.Path] = &dirtyState{cleaned: true}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	pending = make(map[string]*PendingNode)
	var toClean []string
	for _, path := range order {
		s := dirtyByPath[path]
		if s == nil || s.cleaned || s.entry == nil {
			continue
		}
		if s.entry.Stg != "" {
			if _, statErr := os.Stat(s.entry.Stg); os.IsNotExist(statErr) {
				toClean = append(toClean, path)
				continue
			}
		}
		pn := &PendingNode{
			Path:      path,
			Fid:       s.entry.Fid,
			ParentFid: s.entry.ParentFid,
			Name:      s.entry.Name,
			LocalPath: s.entry.Stg,
			Size:      s.entry.Size,
			IsFolder:  s.entry.IsFolder,
			Nonce:     s.entry.Nonce,
			UploadID:  s.entry.UploadID,
			LastPart:  s.entry.LastPart,
		}
		if s.update != nil {
			if s.update.Size > 0 {
				pn.Size = s.update.Size
			}
			if s.update.UploadID != "" {
				pn.UploadID = s.update.UploadID
			}
			if s.update.LastPart > 0 {
				pn.LastPart = s.update.LastPart
			}
		}
		pending[path] = pn
	}

	for _, path := range toClean {
		m.appendJournal(&JournalEntry{Op: JOpClean, Path: path})
	}

	return deleteOps, pending, nil
}

// compactJournal rewrites the unified file removing completed entries:
//   - delete + delete_done pairs are removed (the delete is done)
//   - dirty + clean pairs are removed (the upload is done)
//   - remaining entries are written to a new file atomically
func (m *CacheManager) compactJournal() error {
	deleteOps, pending, err := m.loadJournal()
	if err != nil {
		return err
	}

	// Count active (not done) delete entries.
	activeDeletes := 0
	for _, e := range deleteOps {
		if !e.Done {
			activeDeletes++
		}
	}

	if len(pending) == 0 && activeDeletes == 0 {
		os.Remove(m.journalPath())
		return nil
	}

	tmpPath := m.journalPath() + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write active (not done) delete entries.
	for _, e := range deleteOps {
		if e.Done {
			continue
		}
		entry := &JournalEntry{
			Op:   JOpDelete,
			Path: e.Path,
			Fid:  e.Fid,
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.Write([]byte("\n"))
	}

	// Write active dirty-file entries.
	for _, pn := range pending {
		entry := &JournalEntry{
			Op:        JOpDirty,
			Path:      pn.Path,
			Fid:       pn.Fid,
			ParentFid: pn.ParentFid,
			Name:      pn.Name,
			Stg:       pn.LocalPath,
			Size:      pn.Size,
			IsFolder:  pn.IsFolder,
			Nonce:     pn.Nonce,
			UploadID:  pn.UploadID,
			LastPart:  pn.LastPart,
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.Write([]byte("\n"))
	}

	f.Close()
	return os.Rename(tmpPath, m.journalPath())
}

// --- Compatibility wrappers (callers in fs/ use these types) ---

func (m *CacheManager) AppendOpsLog(entry *OpsLogEntry) error {
	return m.appendJournal(&JournalEntry{
		Op:   JOpDelete,
		Path: entry.Path,
		Fid:  entry.Fid,
	})
}

func (m *CacheManager) LoadOpsLog() ([]OpsLogEntry, error) {
	deleteOps, _, err := m.loadJournal()
	return deleteOps, err
}

// MarkOpsLogDone appends a delete_done marker instead of rewriting the file.
// Compaction will remove the original delete + this marker as a pair.
func (m *CacheManager) MarkOpsLogDone(path string) error {
	return m.appendJournal(&JournalEntry{
		Op:   JOpDeleteDone,
		Path: path,
	})
}

func (m *CacheManager) PurgeOpsLog(_ time.Duration) error {
	return m.compactJournal()
}

func (m *CacheManager) Maintenance() error {
	m.EvictIfNeeded(m.maxSize * 7 / 10)
	m.CleanupStagingMetas(24 * time.Hour)
	m.compactJournal()
	m.reportStaleUploadIDs()
	return nil
}

func (m *CacheManager) reportStaleUploadIDs() {
	const staleThreshold = 2 * time.Hour
	m.mu.RLock()
	defer m.mu.RUnlock()
	for path, n := range m.pendingNodes {
		if n.UploadID != "" {
			if m.staging != nil && !m.staging.Exists(n.LocalPath) {
				fmt.Printf("[cache] WARN: stale uploadID=%s for fid=%s path=%s (staging missing), consider aborting manually via Quark API\n",
					n.UploadID, n.Fid, path)
			}
		}
	}
}

func (m *CacheManager) MaintenanceStart() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("PANIC in MaintenanceStart: %v\n", r)
			}
		}()
		m.Maintenance()
		ticker := time.NewTicker(maintenanceInterval)
		defer ticker.Stop()
		for range ticker.C {
			m.Maintenance()
		}
	}()
}

func (m *CacheManager) Close() error {
	return m.compactJournal()
}

// --- Staging Cleanup (kept for compatibility) ---

func (m *CacheManager) CleanupStagingMetas(abandonedMaxAge time.Duration) error {
	m.mu.RLock()
	activeFids := make(map[string]bool)
	for _, n := range m.pendingNodes {
		if n.Fid != "" {
			activeFids[n.Fid] = true
		}
	}
	metas := make([]StagingMeta, 0, len(m.stagingMetas))
	for _, s := range m.stagingMetas {
		metas = append(metas, *s)
	}
	m.mu.RUnlock()

	var orphanFids []string
	for _, meta := range metas {
		if meta.Status == "active" && !activeFids[meta.Fid] {
			orphanFids = append(orphanFids, meta.Fid)
		}
	}

	for _, fid := range orphanFids {
		meta := m.GetStagingMeta(fid)
		if meta != nil && meta.Status == "active" {
			m.mu.Lock()
			if s, exists := m.stagingMetas[fid]; exists {
				s.Status = "abandoned"
			}
			m.mu.Unlock()
		}
	}

	m.mu.Lock()
	for fid, s := range m.stagingMetas {
		if s.Status == "abandoned" && time.Since(s.UpdatedAt) > abandonedMaxAge {
			if s.LocalPath != "" {
				os.Remove(s.LocalPath)
			}
			delete(m.stagingMetas, fid)
		}
	}
	m.mu.Unlock()

	return nil
}

// --- Batch operations (in-memory iteration, no SQLite) ---

func (m *CacheManager) BatchDeleteNodeState(fids []string, paths []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, path := range paths {
		delete(m.pendingNodes, path)
	}

	fidSet := make(map[string]bool, len(fids))
	for _, f := range fids {
		if f != "" {
			fidSet[f] = true
		}
	}

	for path, n := range m.pendingNodes {
		if fidSet[n.Fid] {
			delete(m.pendingNodes, path)
		}
	}

	for _, f := range fids {
		delete(m.chunkIndex, f)
		delete(m.stagingMetas, f)
	}

	return nil
}
