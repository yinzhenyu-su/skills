## MODIFIED Requirements

### Requirement: File modification MUST follow write-back state transitions

The system MUST track modified files as dirty after write and MUST enqueue synchronization on flush/release-equivalent completion points. 在端到端测试中，系统必须能够通过标准 `os.WriteFile` 触发此状态机转换。

#### Scenario: File modified and flushed

- **WHEN** a file receives write operations via standard IO and then flush is triggered by `f.Close()`
- **THEN** the system MUST mark the file dirty and enqueue a sync task

### Requirement: Successful sync MUST clear dirty and pending markers

On successful upload finalization, the system MUST clear dirty markers and pending records, and MUST update node metadata to the latest fid/size/encryption nonce state. 在端到端测试中，当 `isDirty` 变为 `false` 时，系统必须确保该文件在卸载重挂载后其内容与写入时一致。

#### Scenario: Sync success cleanup

- **WHEN** a file sync completes successfully in a mounted environment
- **THEN** the system MUST clear pending/dirty state, allowing the test to verify consistency by reading the file back after its `isDirty` flag is cleared
