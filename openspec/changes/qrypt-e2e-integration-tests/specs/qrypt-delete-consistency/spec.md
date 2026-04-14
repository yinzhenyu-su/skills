## MODIFIED Requirements

### Requirement: Delete operation MUST converge local and remote state

The system MUST treat successful remote deletion as the trigger to converge local state, including node index, pending upload records, dirty chunks, and in-memory cache entries. 在端到端测试环境中，系统必须能够通过标准 `os.Remove` 调用触发此流程，并在卸载并重新挂载后验证该节点在远程已不可见。

#### Scenario: Delete file successfully

- **WHEN** a file delete request succeeds on remote API via `os.Remove` in a mounted filesystem
- **THEN** the system MUST remove the file node from VFS and clear related pending/dirty/cache state for that path or fid, ensuring it is no longer listed in `os.ReadDir` after a remount

#### Scenario: Delete directory successfully

- **WHEN** a directory delete request succeeds on remote API via `os.RemoveAll` in a mounted filesystem
- **THEN** the system MUST remove the directory node and clear related local pending state under the deleted subtree
