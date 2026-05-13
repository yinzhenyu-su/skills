## Why

当前 qrypt 的 staging 写入策略是每次 `Write` 直接刷入磁盘（open+write+close），编辑器保存大文件时每秒触发数十次磁盘 IO，效率低且增加 staging 分片。同时存在两处已知的竞态窗口：`expectedFid` 在重新编辑后未清除导致 `MergeRemoteChanges` 误判；上传失败 crash 后 staging 文件残留。需要一并将路径上的低挂果实摘掉。

## What Changes

- **Writeback 延迟合并**（P0）：将 staging 写入改为定时 flush（类似 Linux pdflush），内存缓冲后批量写入磁盘。编辑器连续 Write 在同一 staging 文件时合并为更少的磁盘写入。
- **API 请求合并**（P2）：批量删除（rm）改为一次 API 请求；批量 rename 合并；ListFiles 父目录缓存期内去重。
- **expectedFid 清除**（P1）：节点重新变脏时清空 `expectedFid`，消除 `MergeRemoteChanges` 误判窗口。
- **Crash 残留清理**（P3）：启动时扫描 staging 目录，清理无对应活跃节点的 `.staging` 文件。

## Capabilities

### New Capabilities
- `writeback-delay`: staging 写入延迟合并，Write 操作不直接刷盘，由定时 flush 统一写入
- `request-merge`: Quark API 请求合并（批量删除/重命名、ListFiles 缓存去重）

### Modified Capabilities

无。本次涉及的全部是内部实现变更，不改变用户可见的能力边界。

## Impact

- `internal/staging/store.go`: WriteAt 不再是即时写入点，改为 buffer + flush 模式
- `internal/fs/write.go`: Write 处理器不再直接调用 staging.WriteAt
- `internal/fs/sync.go`: expectedFid 清除（2 行改动）
- `internal/quark/client.go` / `api_manage.go`: 批量操作入口
- `internal/fs/recovery.go` / `fs.go`: 启动时 staging 残留清理
- `internal/fs/readdir.go`: MergeRemoteChanges 不再依赖 `expectedFid`

依赖无变化。
