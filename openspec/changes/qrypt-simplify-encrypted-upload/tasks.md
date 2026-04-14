## 1. File-level staging foundation

- [x] 1.1 Add a file-level staging module that creates, opens, truncates, and removes local staging files for modified qrypt paths
- [x] 1.2 Update `QryptFS.Create`, `QryptFS.Write`, and `QryptFS.Truncate` to write plaintext into staging files instead of dirty chunk persistence
- [x] 1.3 Replace chunk-driven pending upload persistence with file-level pending metadata needed to retry staged uploads

## 2. Upload pipeline refactor

- [x] 2.1 Add an upload manager that owns hash computation, upload preparation, multipart upload sequencing, and completion cleanup
- [x] 2.2 Add a streaming encryption reader that emits rclone-compatible encrypted bytes from a plaintext staging file
- [x] 2.3 Refactor sync execution to consume staged files through the encryption reader instead of reconstructing multipart parts from plaintext chunks

## 3. Quark upload sequencing alignment

- [x] 3.1 Reorder the upload flow to `UploadPre -> multipart upload -> UpdateHash -> Commit fallback -> Finish`
- [x] 3.2 Switch multipart upload buffering to `pre.Metadata.PartSize` rather than the fixed local part size
- [x] 3.3 Update sync success and failure paths so file-level staging, pending state, fid updates, and retry state remain consistent

## 4. Cleanup and validation

- [x] 4.1 Remove chunk-specific upload preparation logic that is no longer used by the write-back path
- [x] 4.2 Update or add unit/perf tests for staged writes, pre-upload hash completion, encrypted stream multipart upload, and retryable failure handling
- [x] 4.3 Run qrypt test coverage relevant to the refactor and confirm the new upload flow is ready for apply-phase implementation
