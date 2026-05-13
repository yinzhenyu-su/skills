## 1. Writeback: Page buffer in staging store

- [x] 1.1 Add `Page` struct to `internal/staging/store.go`: buffer (`[]byte`), dirty flag, timer, mutex
- [x] 1.2 Add `PageMap sync.Map` to `Store` for active pages (keyed by fid)
- [x] 1.3 Implement `Page.WriteAt(data []byte, off int64)`: buffer write with offset support
- [x] 1.4 Implement `Page.Flush(stagingDir string, fid string)`: write buffer to disk, clear buffer
- [x] 1.5 Implement per-page flush timer: 250ms after last write, reset on each write
- [x] 1.6 Modify `Store.WriteAt(path, data, off)`: check page cache first, buffer if hit, fallback to direct write
- [x] 1.7 Modify `Store.Sync(path)`: flush page for the given fid, then fsync disk file
- [x] 1.8 Handle page full condition (>1MB): flush immediately, re-buffer excess
- [x] 1.9 Implement `Page.Close()`: flush + destroy timer + remove from PageMap
- [x] 1.10 Verify: `staging.Sync` is called from FUSE `Fsync` and `Release` paths

## 2. expectedFid clear on re-dirty

- [x] 2.1 In `internal/fs/write.go` Write/Truncate handler: clear `node.expectedFid = ""` when node transitions to dirty

## 3. API request merge

- [x] 3.1 `ManageService.Delete(fids []string)` already exists and accepts batch — no change needed
- [x] 3.2 `processBatchMetadataTasks` already collects fids and passes all at once to `asyncDelete`
- [x] 3.3 `asyncDelete` already has retry + fallback logic

## 4. Staging crash cleanup on startup

- [x] 4.1-4.3 `ListStagingFiles` + `CleanupOrphanedStagingFiles` already exist in store.go; wired into `NewFS` in fs.go
- [x] 4.4 Log orphan count on startup
