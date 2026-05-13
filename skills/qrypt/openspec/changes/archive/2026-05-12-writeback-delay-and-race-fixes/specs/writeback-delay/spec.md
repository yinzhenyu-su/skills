## ADDED Requirements

### Requirement: Staging write coalescing
The staging store SHALL coalesce consecutive writes to the same staging file into a memory buffer, flushing to disk only when a flush trigger fires. This does NOT change the `staging.WriteAt` public signature — callers remain unchanged.

- The system SHALL maintain one memory Page per staging file being actively written to
- A Page SHALL hold a `[]byte` buffer large enough to cover the maximum offset written
- Pages SHALL be protected by a mutex for concurrent access

#### Scenario: Consecutive writes merged into single disk write
- **WHEN** a caller calls `staging.WriteAt(fd, data1, off1)` then `staging.WriteAt(fd, data2, off2)` within 250ms
- **THEN** the data SHALL be written to the memory buffer only
- **AND** no disk write SHALL occur until the flush timer fires

### Requirement: Flush triggers
The staging system SHALL flush pending buffers to disk on the following events:
1. **Timer expiry**: 250ms after the last write to a Page (per-Page timer)
2. **Sync request**: `staging.Sync(path)` SHALL force an immediate flush
3. **File close**: `staging.Close(path)` SHALL force an immediate flush
4. **Buffer full**: When a Page exceeds 1MB, SHALL flush immediately

#### Scenario: Sync triggers flush
- **WHEN** `Fsync` calls `staging.Sync(path)`
- **THEN** the corresponding Page SHALL be flushed to disk immediately
- **AND** the file on disk SHALL contain all data written up to that point

#### Scenario: Release triggers flush
- **WHEN** `Release` calls `staging.Sync(path)`
- **THEN** the corresponding Page SHALL be flushed to disk immediately

#### Scenario: Buffer full flushes early
- **WHEN** writes cause a Page buffer to exceed 1MB
- **THEN** the Page SHALL flush to disk immediately
- **AND** the buffer SHALL be cleared after flush

### Requirement: Offset-correct buffer writes
The Page buffer SHALL support offset-based writes. A write at a given offset SHALL overwrite the corresponding bytes in the buffer (previously written data at overlapping offsets SHALL be superseded).

#### Scenario: Overlapping write replaces earlier data
- **WHEN** Page contains `[0x00, 0x01]` at offset 0
- **AND** `Page.WriteAt([0xFF], 0)` is called
- **THEN** the buffer SHALL contain `[0xFF, 0x01]` at offset 0

### Requirement: Page lifecycle
The staging system SHALL manage Page lifecycle: create on first write to a new staging file, flush and destroy on sync/close.

#### Scenario: First write creates page
- **WHEN** `staging.WriteAt(newFid, data, off)` is called for the first time on a staging file
- **THEN** a new Page SHALL be created for that fid
- **AND** the data SHALL be written to the Page buffer

#### Scenario: Close destroys page
- **WHEN** `staging.Sync(path)` completes successfully
- **THEN** the Page for that fid SHALL be flushed
- **AND** the Page SHALL be destroyed (no longer referenced)
