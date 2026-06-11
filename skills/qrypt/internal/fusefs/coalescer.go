//go:build !nofuse

package fusefs

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

// OpKind represents the type of FUSE operation event.
type OpKind string

const (
	OpCreate OpKind = "CREATE"
	OpWrite  OpKind = "WRITE"
	OpUpload OpKind = "UPLOAD"
	OpRename OpKind = "RENAME"
	OpDelete OpKind = "DELETE"
	OpMkdir  OpKind = "MKDIR"
)

// FileOp is a structured event submitted by FUSE handlers to the coalescer.
type FileOp struct {
	Kind      OpKind
	Path      string
	NewPath   string   // target path for rename
	Fid       string
	ParentFid string
	IsDir     bool
	Staging   string   // local staging file path
	Generation uint64  // content version, 0 means auto-assign
	Time      time.Time
	Node      *Node    // reference to the in-memory node (can be nil for late operations)
	Fids      []string // child fids for recursive directory delete (computed before tree cleanup)
}

// UploadIntent tracks a pending upload for a node.
type UploadIntent struct {
	Staging    string
	Node       *Node
	Generation uint64
	Started    bool   // upload has begun (cannot cancel, must compensate)
	Committed  bool   // upload committed to remote (rename or delete-after needed)
}

// PendingIntent tracks the coalesced state for one logical node (by fid).
type PendingIntent struct {
	Fid          string
	Path         string
	IsDir        bool
	Generation   uint64
	Upload       *UploadIntent
	RenameFrom   string  // original path before rename
	RenameTo     string  // target path after rename (set when rename is pending)
	Delete       bool    // delete has been requested
	RemoteExists bool    // file exists on remote (not local_ fid)
	Started      bool    // at least one operation has started executing remotely
	Committed    bool    // at least one operation committed to remote
	ChildFids    []string // child fids collected before tree cleanup (for recursive delete)
}

// PathTombstone marks a path as being deleted, absorbing late child operations.
type PathTombstone struct {
	Path      string
	Recursive bool        // directory tombstone absorbs children
	ExpiresAt time.Time   // after this, tombstone is cleaned up
	Done      bool        // remote delete confirmed
}

// RemoteIntent is the coalesced output - a minimal remote operation.
type RemoteIntent struct {
	Kind       string   // "UPLOAD", "DELETE", "RENAME", "MKDIR"
	Path       string
	NewPath    string   // for RENAME
	Fid        string
	ParentFid  string
	Staging    string
	Node       *Node
	Fids       []string // for batch DELETE
	Recursive  bool     // for recursive directory DELETE
	Generation uint64
	SkipRemote bool     // absorbed by coalescing, only local cleanup needed
}

// Capabilities describes what the remote driver supports.
type Capabilities struct {
	BatchDelete  bool
	Rename       bool
	Move         bool
	CancelUpload bool
}

// OperationCoalescer accumulates, coalesces, and cancels FUSE operations
// within configurable time windows before producing minimal RemoteIntents.
//
// Lifecycle:
//
//	coalescer := NewOperationCoalescer(caps, flushFn)
//	defer coalescer.Shutdown()
//
//	// From FUSE handlers — non-blocking:
//	coalescer.Submit(FileOp{Kind: OpCreate, Path: "/a.txt", Fid: "local_1", ...})
//
// The background goroutine runs a timer per batch window; when the timer fires
// it calls flush() which applies the coalescing rules and passes the resulting
// []RemoteIntent to flushFn.
type OperationCoalescer struct {
	mu         sync.Mutex
	eventCh    chan FileOp

	// Intent index: fid → PendingIntent
	intents map[string]*PendingIntent

	// Tombstone index: path → PathTombstone
	tombstones map[string]*PathTombstone

	// Window durations (configurable)
	fileWindow   time.Duration // 200ms for file create/write/upload
	renameWindow time.Duration // 600ms for rename/create/upload
	deleteWindow time.Duration // 600ms for delete family
	tombstoneTTL time.Duration // 10min for absorbing late deletes

	// Generation counter
	genCounter uint64

	// Capabilities
	caps Capabilities

	// Flush output channel — sends []RemoteIntent to the flush handler
	flushFn func(intents []RemoteIntent)

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOperationCoalescer creates a new coalescer and starts its background worker goroutine.
func NewOperationCoalescer(caps Capabilities, flushFn func([]RemoteIntent)) *OperationCoalescer {
	ctx, cancel := context.WithCancel(context.Background())
	oc := &OperationCoalescer{
		eventCh:      make(chan FileOp, 4096),
		intents:      make(map[string]*PendingIntent),
		tombstones:   make(map[string]*PathTombstone),
		fileWindow:   200 * time.Millisecond,
		renameWindow: 600 * time.Millisecond,
		deleteWindow: 600 * time.Millisecond,
		tombstoneTTL: 10 * time.Minute,
		caps:         caps,
		flushFn:      flushFn,
		ctx:          ctx,
		cancel:       cancel,
	}
	oc.wg.Add(1)
	go oc.run()
	return oc
}

// ---------------------------------------------------------------------------
// Submit — non-blocking event submission from FUSE handlers
// ---------------------------------------------------------------------------

// Submit enqueues a FileOp event for coalescing. It should not block the caller.
// If the internal channel is full the event is processed synchronously instead of being dropped.
func (oc *OperationCoalescer) Submit(op FileOp) {
	select {
	case oc.eventCh <- op:
	default:
		logging.L.Warnf("coalescer: event channel full, blocking for %s on %s fid=%s\n",
			op.Kind, op.Path, op.Fid)
		oc.eventCh <- op // Block until accepted — preserves ordering
	}
}

// ---------------------------------------------------------------------------
// Background worker
// ---------------------------------------------------------------------------

// run is the background goroutine. It reads FileOp events from the channel,
// groups them into batches using a timer-based window, and flushes each batch
// when the timer fires or on shutdown.
func (oc *OperationCoalescer) run() {
	defer oc.wg.Done()

	var batch []FileOp
	var timer *time.Timer
	var timerC <-chan time.Time

	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case op, ok := <-oc.eventCh:
			if !ok {
				// Channel closed — flush remaining batch
				if len(batch) > 0 {
					oc.flushAndLog(batch)
				}
				return
			}

			batch = append(batch, op)

			// Start the batch window timer on first event
			if timerC == nil {
				d := oc.windowFor(op.Kind)
				timer = time.NewTimer(d)
				timerC = timer.C
			}

		case <-timerC:
			if len(batch) > 0 {
				oc.flushAndLog(batch)
				batch = nil
			}
			timerC = nil

		case <-cleanupTicker.C:
			oc.mu.Lock()
			oc.cleanupExpiredTombstonesLocked()
			oc.mu.Unlock()

		case <-oc.ctx.Done():
			// Drain remaining events before flushing
		drainLoop:
			for {
				select {
				case op, ok := <-oc.eventCh:
					if !ok {
						break drainLoop
					}
					batch = append(batch, op)
				default:
					break drainLoop
				}
			}
			if len(batch) > 0 {
				oc.flushAndLog(batch)
			}
			return
		}
	}
}

// windowFor returns the coalescing window duration appropriate for the event kind.
func (oc *OperationCoalescer) windowFor(kind OpKind) time.Duration {
	switch kind {
	case OpDelete:
		return oc.deleteWindow
	case OpRename:
		return oc.renameWindow
	default:
		return oc.fileWindow
	}
}

// flushAndLog calls flush() and passes the results to flushFn.
func (oc *OperationCoalescer) flushAndLog(ops []FileOp) {
	results := oc.flush(ops)
	if len(results) > 0 && oc.flushFn != nil {
		oc.flushFn(results)
	}
}

// ---------------------------------------------------------------------------
// Core flush logic
// ---------------------------------------------------------------------------

// flush processes a batch of accumulated FileOp events and produces a minimal
// set of RemoteIntent values by applying the coalescing and cancellation rules.
//
// The method holds oc.mu for the entire duration. It modifies the tombstone
// index but does NOT modify oc.intents (that is rebuilt fresh each call).
func (oc *OperationCoalescer) flush(ops []FileOp) []RemoteIntent {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if len(ops) == 0 {
		return nil
	}

	// ---- Phase 1: Build pending intents grouped by fid ----
	intents := make(map[string]*PendingIntent, len(ops))
	var orderedFids []string

	for _, op := range ops {
		fid := op.Fid
		if fid == "" {
			continue
		}

		intent, exists := intents[fid]
		if !exists {
			intent = &PendingIntent{
				Fid:          fid,
				Path:         op.Path,
				IsDir:        op.IsDir,
				RemoteExists: !strings.HasPrefix(fid, "local_"),
			}
			intents[fid] = intent
			orderedFids = append(orderedFids, fid)
		}
		oc.applyEventToIntent(intent, op)
	}

	// ---- Phase 1.5: Carry over UploadIntent lifecycle from oc.intents ----
	// MarkUploadStarted / MarkUploadCommitted update the persistent intent map.
	// When flush() rebuilds intents from a new batch of ops, any UploadIntent
	// that existed previously must retain its Started/Committed flags so the
	// coalescer doesn't incorrectly cancel an in-flight upload.
	// Carry over even when the new batch has no Upload op for this fid
	// (e.g. a rename or delete that arrived during an in-flight upload).
	for fid, intent := range intents {
		if existing, ok := oc.intents[fid]; ok && existing.Upload != nil {
			if intent.Upload == nil {
				intent.Upload = existing.Upload
			} else {
				intent.Upload.Started = existing.Upload.Started
				intent.Upload.Committed = existing.Upload.Committed
			}
		}
	}

	// ---- Phase 2: Add tombstones for directory deletes ----
	// This must happen before child-intent processing so that children
	// arriving in the same batch are absorbed.
	for _, intent := range intents {
		if intent.Delete && intent.IsDir {
			oc.addTombstoneLocked(intent.Path, true)
		}
	}

	// Clean expired tombstones before checking
	oc.cleanupExpiredTombstonesLocked()

	// ---- Phase 3: Generate coalesced RemoteIntents ----
	var results []RemoteIntent
	var canceled, renamedUploads, absorbedByDelete int

	for _, fid := range orderedFids {
		intent := intents[fid]

		// --- Tombstone absorption ---
		// An intent whose path falls under a *recursive* tombstone (but is NOT
		// the tombstone root itself) is absorbed — no remote operation needed.
		if oc.isUnderRecursiveTombstoneLocked(intent.Path) {
			absorbedByDelete++
			continue
		}

		// --- Rule 4: Rename → Delete (skip rename) ---
		if intent.Delete && intent.RenameFrom != "" {
			if !intent.RemoteExists {
				// Was local-only — cancel everything (local cleanup only)
				canceled++
				continue
			}
			// Was remote — skip rename, just delete the original path
			results = append(results, RemoteIntent{
				Kind: "DELETE",
				Path: intent.RenameFrom,
				Fid:  fid,
			})
			continue
		}

		// --- Rule 1: Create → Delete (cancel upload if not started) ---
		if intent.Delete && intent.Upload != nil && !intent.Upload.Started {
			canceled++
			continue
		}

		// Pure delete (no pending upload, or upload was already handled above)
		if intent.Delete {
			fids := []string{fid}
			if len(intent.ChildFids) > 0 {
				fids = intent.ChildFids
			}
			if intent.RemoteExists {
				results = append(results, RemoteIntent{
					Kind:      "DELETE",
					Path:      intent.Path,
					Fid:       fid,
					Fids:      fids,
					Recursive: intent.IsDir,
				})
			} else {
				results = append(results, RemoteIntent{
					Kind:       "DELETE",
					Path:       intent.Path,
					Fid:        fid,
					Fids:       fids,
					SkipRemote: true,
				})
			}
			continue
		}

		// --- Handle rename (Rules 2, 3, 7) ---
		if intent.RenameFrom != "" && intent.RenameTo != "" {
			// Rule 3: Pure rename chain — no pending upload, non-local file
			if intent.Upload == nil && intent.RemoteExists {
				results = append(results, RemoteIntent{
					Kind:    "RENAME",
					Path:    intent.RenameFrom,
					NewPath: intent.RenameTo,
					Fid:     fid,
				})
				continue
			}

			// Rules 2 & 7: Rename with pending upload
			if intent.Upload != nil {
				if !intent.Upload.Started {
					// Rule 2: Upload not started — migrate upload to new path
					renamedUploads++
					results = append(results, RemoteIntent{
						Kind:       "UPLOAD",
						Path:       intent.RenameTo,
						Fid:        fid,
						ParentFid:  intent.Upload.Node.parentFid, // best-effort
						Staging:    intent.Upload.Staging,
						Node:       intent.Upload.Node,
						Generation: intent.Upload.Generation,
					})
					continue
				}

				// Rule 7: Upload already started — compensation required
				renamedUploads++
				if oc.caps.Rename {
					// Driver supports remote rename — rename after upload completes
					results = append(results, RemoteIntent{
						Kind:    "RENAME",
						Path:    intent.RenameFrom,
						NewPath: intent.RenameTo,
						Fid:     fid,
					})
				} else {
					// No rename support — upload new path + delete old path
					results = append(results, RemoteIntent{
						Kind:       "UPLOAD",
						Path:       intent.RenameTo,
						Fid:        fid,
						Staging:    intent.Upload.Staging,
						Node:       intent.Upload.Node,
						Generation: intent.Upload.Generation,
					})
					results = append(results, RemoteIntent{
						Kind: "DELETE",
						Path: intent.RenameFrom,
						Fid:  fid,
					})
				}
				continue
			}
		}

		// --- Plain upload (no rename, no delete) ---
		if intent.Upload != nil {
			results = append(results, RemoteIntent{
				Kind:       "UPLOAD",
				Path:       intent.Path,
				Fid:        fid,
				Staging:    intent.Upload.Staging,
				Node:       intent.Upload.Node,
				Generation: intent.Upload.Generation,
			})
			continue
		}

		// --- Plain mkdir (no delete, no rename, no upload) ---
		if intent.IsDir && !intent.Delete && intent.Upload == nil && intent.RenameFrom == "" {
			results = append(results, RemoteIntent{
				Kind: "MKDIR",
				Path: intent.Path,
				Fid:  fid,
			})
			continue
		}
	}

	// ---- Persist intents with upload state for lifecycle tracking ----
	// UploadIntent with Started/Committed flags must survive between flush
	// calls so MarkUploadStarted/MarkUploadCommitted can update them.
	for fid, intent := range intents {
		if intent.Upload != nil {
			oc.intents[fid] = intent
		}
	}

	// ---- Log summary ----
	logging.L.Infof(
		"coalescer: flush %d ops -> %d remote intents, canceled=%d renamed_uploads=%d absorbed_by_delete=%d\n",
		len(ops), len(results), canceled, renamedUploads, absorbedByDelete,
	)
	for _, r := range results {
		logging.L.Debugf(
			"coalescer: intent %s path=%s fid=%s generation=%d skip_remote=%v\n",
			r.Kind, r.Path, r.Fid, r.Generation, r.SkipRemote,
		)
	}

	return results
}

// applyEventToIntent applies a single FileOp event to an existing PendingIntent.
// It is called during Phase 1 of flush().
func (oc *OperationCoalescer) applyEventToIntent(intent *PendingIntent, op FileOp) {
	switch op.Kind {
	case OpCreate:
		intent.Path = op.Path
		intent.IsDir = op.IsDir
		if intent.Generation == 0 {
			intent.Generation = oc.assignGeneration()
		}

	case OpWrite:
		intent.Path = op.Path
		if op.Generation > intent.Generation {
			intent.Generation = op.Generation
		}

	case OpUpload:
		intent.Path = op.Path
		if intent.Upload == nil {
			intent.Upload = &UploadIntent{
				Staging:    op.Staging,
				Node:       op.Node,
				Generation: op.Generation,
			}
		} else {
			if op.Staging != "" {
				intent.Upload.Staging = op.Staging
			}
			if op.Generation > intent.Upload.Generation {
				intent.Upload.Generation = op.Generation
			}
		}
		intent.Started = true

	case OpRename:
		// Only capture the *original* rename-from path; always update
		// rename-to to the latest target (Rules 2, 3).
		if intent.RenameFrom == "" {
			intent.RenameFrom = op.Path
		}
		intent.RenameTo = op.NewPath
		intent.Path = op.NewPath

	case OpDelete:
		intent.Path = op.Path
		intent.Delete = true
		if len(op.Fids) > 0 {
			intent.ChildFids = op.Fids
		}

	case OpMkdir:
		intent.Path = op.Path
		intent.IsDir = true
	}
}

// ---------------------------------------------------------------------------
// Generation counter
// ---------------------------------------------------------------------------

// assignGeneration atomically increments and returns the generation counter.
func (oc *OperationCoalescer) assignGeneration() uint64 {
	return atomic.AddUint64(&oc.genCounter, 1)
}

// ---------------------------------------------------------------------------
// Upload lifecycle tracking
// ---------------------------------------------------------------------------

// MarkUploadStarted records that an upload for the given fid has begun
// executing remotely. Once started, the coalescer cannot cancel the upload;
// it must compensate (rename after commit, delete after commit).
func (oc *OperationCoalescer) MarkUploadStarted(fid string) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	intent, ok := oc.intents[fid]
	if !ok || intent.Upload == nil {
		return
	}
	intent.Upload.Started = true
	intent.Started = true
	logging.L.Debugf("coalescer: upload started for fid=%s generation=%d\n", fid, intent.Upload.Generation)
}

// MarkUploadCommitted records that an upload for the given fid has completed
// successfully on the remote side. After commit, the coalescer must issue a
// remote delete or rename rather than cancelling.
func (oc *OperationCoalescer) MarkUploadCommitted(fid string) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	intent, ok := oc.intents[fid]
	if !ok || intent.Upload == nil {
		return
	}
	intent.Upload.Committed = true
	intent.Committed = true
	logging.L.Debugf("coalescer: upload committed for fid=%s generation=%d\n", fid, intent.Upload.Generation)
}

// ---------------------------------------------------------------------------
// Intent lookup
// ---------------------------------------------------------------------------

// findIntent returns the PendingIntent for the given fid, or nil.
func (oc *OperationCoalescer) findIntent(fid string) *PendingIntent {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.intents[fid]
}

// findIntentByPath returns the first PendingIntent that matches the given path.
// This is an O(n) scan of the intents map and should be used sparingly.
func (oc *OperationCoalescer) findIntentByPath(path string) *PendingIntent {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	for _, intent := range oc.intents {
		if intent.Path == path {
			return intent
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tombstone management
// ---------------------------------------------------------------------------

// addTombstone adds (or replaces) a tombstone entry for the given path.
func (oc *OperationCoalescer) addTombstone(path string, recursive bool) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.addTombstoneLocked(path, recursive)
}

func (oc *OperationCoalescer) addTombstoneLocked(path string, recursive bool) {
	oc.tombstones[path] = &PathTombstone{
		Path:      path,
		Recursive: recursive,
		ExpiresAt: time.Now().Add(oc.tombstoneTTL),
	}
}

// isUnderTombstone reports whether the given path is covered by any active
// tombstone (exact match or prefix match under a recursive tombstone).
func (oc *OperationCoalescer) isUnderTombstone(path string) bool {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.isUnderTombstoneLocked(path)
}

func (oc *OperationCoalescer) isUnderTombstoneLocked(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	now := time.Now()
	// Check exact tombstone match first (but this method is called from flush
	// for absorption, so exact-match tombstones should NOT absorb the root
	// path; the caller in Phase 3 handles that distinction).
	for _, t := range oc.tombstones {
		if !now.After(t.ExpiresAt) && t.Path == path {
			// Exact match — this IS the tombstone root. Only absorb under
			// recursive tombstones, not the root itself.
			// (The caller decides, but we report it as matched so the caller
			// can distinguish root from child.)
			return true
		}
	}
	// Check prefix match for recursive tombstones
	for _, t := range oc.tombstones {
		if !now.After(t.ExpiresAt) && t.Recursive {
			prefix := t.Path
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

// isUnderRecursiveTombstone reports whether the given path falls under a
// *recursive* tombstone but is NOT the tombstone root itself. This is the
// predicate used in flush() for absorbing child operations.
func (oc *OperationCoalescer) isUnderRecursiveTombstoneLocked(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	now := time.Now()
	for _, t := range oc.tombstones {
		if now.After(t.ExpiresAt) {
			continue
		}
		if !t.Recursive {
			continue
		}
		// Check prefix match — path must be a strict descendant
		prefix := t.Path
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// cleanupExpiredTombstonesLocked removes tombstones whose TTL has expired.
// Must be called with oc.mu held.
func (oc *OperationCoalescer) cleanupExpiredTombstonesLocked() {
	now := time.Now()
	for path, t := range oc.tombstones {
		if now.After(t.ExpiresAt) {
			delete(oc.tombstones, path)
		}
	}
}

// cleanupTombstones is a public helper that removes expired tombstones.
func (oc *OperationCoalescer) cleanupTombstones() {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.cleanupExpiredTombstonesLocked()
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

// Shutdown gracefully stops the background worker. It cancels the context,
// waits for the worker to drain pending events and flush the final batch,
// then returns.
func (oc *OperationCoalescer) Shutdown() {
	oc.cancel()
	oc.wg.Wait()
}

// ensure the type-level doc comment does not drift — referenced here for
// compiler satisfaction (no-op).
var _ = (*OperationCoalescer)(nil)
