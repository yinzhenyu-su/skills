package vfs

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type OpKind string

const (
	OpCreate OpKind = "CREATE"
	OpWrite  OpKind = "WRITE"
	OpUpload OpKind = "UPLOAD"
	OpRename OpKind = "RENAME"
	OpDelete OpKind = "DELETE"
	OpMkdir  OpKind = "MKDIR"
)

type FileOp struct {
	Kind       OpKind
	Path       string
	NewPath    string
	Fid        string
	ParentFid  string
	IsDir      bool
	Staging    string
	Generation uint64
	Time       time.Time
	Node       *VFSNode
	Fids       []string
}

type UploadIntent struct {
	Staging    string
	Node       *VFSNode
	Generation uint64
	Started    bool
	Committed  bool
}

type PendingIntent struct {
	Fid          string
	Path         string
	IsDir        bool
	Generation   uint64
	Upload       *UploadIntent
	RenameFrom   string
	RenameTo     string
	Delete       bool
	RemoteExists bool
	Started      bool
	Committed    bool
	ChildFids    []string
	Node         *VFSNode
}

type PathTombstone struct {
	Path      string
	Recursive bool
	ExpiresAt time.Time
	Done      bool
}

type RemoteIntent struct {
	Kind       string
	Path       string
	NewPath    string
	Fid        string
	ParentFid  string
	Staging    string
	Node       *VFSNode
	Fids       []string
	Recursive  bool
	Generation uint64
	SkipRemote bool
}

type Capabilities struct {
	BatchDelete  bool
	Rename       bool
	Move         bool
	CancelUpload bool
}

type OperationCoalescer struct {
	mu      sync.Mutex
	eventCh chan FileOp

	intents    map[string]*PendingIntent
	tombstones map[string]*PathTombstone

	fileWindow   time.Duration
	renameWindow time.Duration
	deleteWindow time.Duration
	tombstoneTTL time.Duration

	genCounter uint64
	caps       Capabilities
	flushFn    func(intents []RemoteIntent)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

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

func (oc *OperationCoalescer) Submit(op FileOp) {
	select {
	case oc.eventCh <- op:
	default:
		logging.L.Warnf("coalescer: event channel full, blocking for %s on %s fid=%s\n",
			op.Kind, op.Path, op.Fid)
		oc.eventCh <- op
	}
}

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
				if len(batch) > 0 {
					oc.flushAndLog(batch)
				}
				return
			}

			batch = append(batch, op)

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

func (oc *OperationCoalescer) flushAndLog(ops []FileOp) {
	results := oc.flush(ops)
	if len(results) > 0 && oc.flushFn != nil {
		oc.flushFn(results)
	}
}

func (oc *OperationCoalescer) flush(ops []FileOp) []RemoteIntent {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	if len(ops) == 0 {
		return nil
	}

	intents := make(map[string]*PendingIntent, len(ops))
	var orderedFids []string

	for _, op := range ops {
		fid := op.Fid
		if fid == "" {
			continue
		}

		remoteExists := op.Node != nil && op.Node.IsUploaded()
		intent, exists := intents[fid]
		if !exists {
			intent = &PendingIntent{
				Fid:          fid,
				Path:         op.Path,
				IsDir:        op.IsDir,
				RemoteExists: remoteExists,
			}
			intents[fid] = intent
			orderedFids = append(orderedFids, fid)
		}
		oc.applyEventToIntent(intent, op)
	}

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

	for _, intent := range intents {
		if intent.Delete && intent.IsDir {
			oc.addTombstoneLocked(intent.Path, true)
		}
	}

	oc.cleanupExpiredTombstonesLocked()

	var results []RemoteIntent
	var canceled, renamedUploads, absorbedByDelete int

	for _, fid := range orderedFids {
		intent := intents[fid]

		if oc.isUnderRecursiveTombstoneLocked(intent.Path) {
			absorbedByDelete++
			continue
		}

		if intent.Delete && intent.RenameFrom != "" {
			if !intent.RemoteExists {
				canceled++
				continue
			}
			results = append(results, RemoteIntent{
				Kind: "DELETE",
				Path: intent.RenameFrom,
				Fid:  fid,
			})
			continue
		}

		if intent.Delete && intent.Upload != nil && !intent.Upload.Started {
			canceled++
			continue
		}

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
					Node:      intent.Node,
				})
			} else {
				results = append(results, RemoteIntent{
					Kind:       "DELETE",
					Path:       intent.Path,
					Fid:        fid,
					Fids:       fids,
					SkipRemote: true,
					Node:       intent.Node,
				})
			}
			continue
		}

		if intent.RenameFrom != "" && intent.RenameTo != "" {
			if intent.Upload == nil && intent.RemoteExists {
				results = append(results, RemoteIntent{
					Kind:    "RENAME",
					Path:    intent.RenameFrom,
					NewPath: intent.RenameTo,
					Fid:     fid,
				})
				continue
			}

			if intent.Upload != nil {
				if !intent.Upload.Started {
					renamedUploads++
					results = append(results, RemoteIntent{
						Kind:       "UPLOAD",
						Path:       intent.RenameTo,
						Fid:        fid,
						ParentFid:  intent.Upload.Node.parentFid,
						Staging:    intent.Upload.Staging,
						Node:       intent.Upload.Node,
						Generation: intent.Upload.Generation,
					})
					continue
				}

				renamedUploads++
				if oc.caps.Rename {
					results = append(results, RemoteIntent{
						Kind:    "RENAME",
						Path:    intent.RenameFrom,
						NewPath: intent.RenameTo,
						Fid:     fid,
					})
				} else {
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

		if intent.IsDir && !intent.Delete && intent.Upload == nil && intent.RenameFrom == "" {
			results = append(results, RemoteIntent{
				Kind: "MKDIR",
				Path: intent.Path,
				Fid:  fid,
			})
			continue
		}
	}

	for fid, intent := range intents {
		if intent.Upload != nil {
			oc.intents[fid] = intent
		}
	}

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
		if intent.RenameFrom == "" {
			intent.RenameFrom = op.Path
		}
		intent.RenameTo = op.NewPath
		intent.Path = op.NewPath

	case OpDelete:
		intent.Path = op.Path
		intent.Delete = true
		intent.Node = op.Node
		if len(op.Fids) > 0 {
			intent.ChildFids = op.Fids
		}

	case OpMkdir:
		intent.Path = op.Path
		intent.IsDir = true
	}
}

func (oc *OperationCoalescer) assignGeneration() uint64 {
	return atomic.AddUint64(&oc.genCounter, 1)
}

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

func (oc *OperationCoalescer) findIntent(fid string) *PendingIntent {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.intents[fid]
}

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
	for _, t := range oc.tombstones {
		if !now.After(t.ExpiresAt) && t.Path == path {
			return true
		}
	}
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

func (oc *OperationCoalescer) cleanupExpiredTombstonesLocked() {
	now := time.Now()
	for path, t := range oc.tombstones {
		if now.After(t.ExpiresAt) {
			delete(oc.tombstones, path)
		}
	}
}

func (oc *OperationCoalescer) cleanupTombstones() {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.cleanupExpiredTombstonesLocked()
}

func (oc *OperationCoalescer) Shutdown() {
	oc.cancel()
	oc.wg.Wait()
}
