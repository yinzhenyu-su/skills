## Why

In `qrypt`, when a file is being uploaded (marked as `isDirty`), operations like `ls` (Readdir) or `lookup` fetch remote metadata and overwrite the local node state in the cache. This causes the file size to appear as 0 bytes or its old remote size until the sync completes. Additionally, multiple calls to `Flush` and `Release` during a single write operation results in redundant `syncTask` queuing, which creates log noise and minor overhead.

## What Changes

- **Metadata Protection**: Modify `Readdir` and `lookup` to avoid overwriting existing `isDirty` nodes with remote metadata.
- **Deduplicate Upload Queue**: Improve the `Flush` logic to check if a node is already being synchronized before queuing a new task.
- **Refined Logging**: Reduce the frequency of "queued for upload" messages to only occur when a unique sync task is actually added to the queue.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `qrypt-metadata-caching`: Ensure that the cache prioritized local modifications (`isDirty`) over remote state.
- `qrypt-modify-writeback-stability`: Improve the deduplication and reporting of the background sync process.

## Impact

- `internal/vfs/fs.go`: Logic in `lookup`, `Readdir`, and `Flush` will be modified.
- User Experience: Files will correctly show their locally-written sizes even while an upload is in progress.
