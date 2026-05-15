package cache

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/staging"
)

type OpsLogEntry struct {
	OpType    string `json:"op"`
	Path      string `json:"path"`
	Fid       string `json:"fid,omitempty"`
	Timestamp int64  `json:"ts"`
	Done      bool   `json:"done"`
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
	staging  *staging.Store
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

func (m *CacheManager) Staging() *staging.Store {
	return m.staging
}

func NewCacheManager(cacheDir string, maxSize int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	stagingDir := filepath.Join(cacheDir, "staging")
	store, err := staging.NewStore(stagingDir)
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

	// Recover dirty-file state from journal before cleaning up, so
	// staging files referenced by journal entries are not treated as orphans.
	recovered, err := m.LoadPendingJournal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadPendingJournal: %v (proceeding with empty state)\n", err)
	} else if len(recovered) > 0 {
		for path, pn := range recovered {
			m.pendingNodes[path] = pn
		}
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

	m.appendPendingJournal(&PendingJournalEntry{
		Op:        PJOpDirty,
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

	m.appendPendingJournal(&PendingJournalEntry{
		Op:   PJOpClean,
		Path: path,
	})
	return nil
}

func (m *CacheManager) UpdatePendingNodeUpload(path, uploadID string) error {
	m.mu.Lock()
	if n, ok := m.pendingNodes[path]; ok {
		n.UploadID = uploadID
	}
	m.mu.Unlock()

	m.appendPendingJournal(&PendingJournalEntry{
		Op:       PJOpUpdate,
		Path:     path,
		UploadID: uploadID,
	})
	return nil
}

func (m *CacheManager) UpdatePendingNodeLastPart(path string, lastPart int) error {
	m.mu.Lock()
	if n, ok := m.pendingNodes[path]; ok {
		n.LastPart = lastPart
	}
	m.mu.Unlock()

	m.appendPendingJournal(&PendingJournalEntry{
		Op:       PJOpUpdate,
		Path:     path,
		LastPart: lastPart,
	})
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

// --- Ops Log (Journaling) ---

func (m *CacheManager) opsLogPath() string {
	return filepath.Join(m.cacheDir, "ops.jsonl")
}

func (m *CacheManager) AppendOpsLog(entry *OpsLogEntry) error {
	entry.Timestamp = time.Now().UnixNano()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(m.opsLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
	return nil
}

func (m *CacheManager) LoadOpsLog() ([]OpsLogEntry, error) {
	f, err := os.Open(m.opsLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []OpsLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry OpsLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

func (m *CacheManager) MarkOpsLogDone(path string) error {
	entries, err := m.LoadOpsLog()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].Path == path && !entries[i].Done {
			entries[i].Done = true
		}
	}
	return m.rewriteOpsLog(entries)
}

func (m *CacheManager) PurgeOpsLog(olderThan time.Duration) error {
	entries, err := m.LoadOpsLog()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-olderThan)
	var kept []OpsLogEntry
	for _, e := range entries {
		ts := time.Unix(0, e.Timestamp)
		if e.Done && ts.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	return m.rewriteOpsLog(kept)
}

func (m *CacheManager) rewriteOpsLog(entries []OpsLogEntry) error {
	if len(entries) == 0 {
		os.Remove(m.opsLogPath())
		return nil
	}
	f, err := os.Create(m.opsLogPath())
	if err != nil {
		return err
	}
	defer f.Close()
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	return nil
}

// --- Pending Journal (crash-recovery for dirty files) ---

type PendingJournalOp string

const (
	PJOpDirty  PendingJournalOp = "dirty"
	PJOpUpdate PendingJournalOp = "update"
	PJOpClean  PendingJournalOp = "clean"
)

type PendingJournalEntry struct {
	Op        PendingJournalOp `json:"op"`
	Path      string           `json:"path"`
	Fid       string           `json:"fid,omitempty"`
	ParentFid string           `json:"parent_fid,omitempty"`
	Name      string           `json:"name,omitempty"`
	Stg       string           `json:"stg,omitempty"`
	Size      int64            `json:"size,omitempty"`
	IsFolder  bool             `json:"is_folder,omitempty"`
	Nonce     []byte           `json:"nonce,omitempty"`
	BaseMtime int64            `json:"base_mtime,omitempty"`
	BaseSize  int64            `json:"base_size,omitempty"`
	UploadID  string           `json:"upload_id,omitempty"`
	LastPart  int              `json:"last_part,omitempty"`
	Mtime     int64            `json:"ts"`
}

func (m *CacheManager) pendingJournalPath() string {
	return filepath.Join(m.cacheDir, "pending.journal")
}

func (m *CacheManager) appendPendingJournal(entry *PendingJournalEntry) error {
	entry.Mtime = time.Now().UnixNano()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(m.pendingJournalPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
	return nil
}

// LoadPendingJournal reads pending.journal and returns the latest non-clean
// state for each path. The staging file is cross-checked: entries whose
// staging file no longer exists are dropped and a 'clean' entry is written.
func (m *CacheManager) LoadPendingJournal() (map[string]*PendingNode, error) {
	f, err := os.Open(m.pendingJournalPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type state struct {
		entry  *PendingJournalEntry // latest dirty
		update *PendingJournalEntry // latest update after dirty
		cleaned bool                // true if a clean follows dirty
	}
	byPath := make(map[string]*state)
	order := make([]string, 0) // to preserve ordering for stable results

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry PendingJournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Op {
		case PJOpDirty:
			if _, ok := byPath[entry.Path]; !ok {
				order = append(order, entry.Path)
			}
			byPath[entry.Path] = &state{
				entry:  &entry,
				update: nil,
				cleaned: false,
			}
		case PJOpUpdate:
			if s, ok := byPath[entry.Path]; ok && !s.cleaned {
				if s.update == nil {
					s.update = &PendingJournalEntry{}
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
		case PJOpClean:
			if _, ok := byPath[entry.Path]; ok {
				byPath[entry.Path] = &state{cleaned: true}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := make(map[string]*PendingNode)
	var toClean []string

	for _, path := range order {
		s := byPath[path]
		if s == nil || s.cleaned || s.entry == nil {
			continue
		}

		// Cross-check: staging file must exist on disk.
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
		}

		// Merge update fields if present.
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

		result[path] = pn
	}

	// Clean up dangling journal entries (staging file missing).
	for _, path := range toClean {
		m.appendPendingJournal(&PendingJournalEntry{Op: PJOpClean, Path: path})
	}

	return result, nil
}

// rewritePendingJournal atomically rewrites the journal file for the currently
// active pending nodes, removing stale entries.
func (m *CacheManager) rewritePendingJournal() error {
	m.mu.RLock()
	nodes := make([]*PendingNode, 0, len(m.pendingNodes))
	for _, n := range m.pendingNodes {
		nodes = append(nodes, n)
	}
	m.mu.RUnlock()

	if len(nodes) == 0 {
		os.Remove(m.pendingJournalPath())
		return nil
	}

	tmpPath := m.pendingJournalPath() + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, n := range nodes {
		entry := &PendingJournalEntry{
			Op:        PJOpDirty,
			Path:      n.Path,
			Fid:       n.Fid,
			ParentFid: n.ParentFid,
			Name:      n.Name,
			Stg:       n.LocalPath,
			Size:      n.Size,
			IsFolder:  n.IsFolder,
			Nonce:     n.Nonce,
			BaseMtime: n.BaseServerMtime,
			BaseSize:  n.BaseServerSize,
			UploadID:  n.UploadID,
			LastPart:  n.LastPart,
		}
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()
	return os.Rename(tmpPath, m.pendingJournalPath())
}

func (m *CacheManager) compactPendingJournal() error {
	return m.rewritePendingJournal()
}

func (m *CacheManager) Maintenance() error {
	m.EvictIfNeeded(m.maxSize * 7 / 10)
	m.CleanupStagingMetas(24 * time.Hour)
	m.PurgeOpsLog(72 * time.Hour)
	m.compactPendingJournal()
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
	return nil
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
