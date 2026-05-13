## ADDED Requirements

### Requirement: 双层缓存架构

系统采用双层缓存：在 `FileService`（quark API 层）提供 TTL 目录缓存，在 `QryptFS`（FUSE 层）提供基于节点 `lastMetadataCheck` 的过期刷新。

#### 第一层：quark.CacheService 目录缓存

##### Scenario: TTL-based listing cache
- **WHEN** `ListFiles(parentFid)` 被调用
- **AND** `CacheService.GetDir(parentFid)` 命中（TTL 未过期，默认 60s）
- **THEN** 直接返回缓存的 `[]File`，不发起网络请求
- **WHEN** 缓存过期或主动调用 `RemoveDir(fid)`
- **THEN** 下次 `ListFiles` 从远程 API 获取最新数据

##### Scenario: URL 缓存
- **WHEN** 获取文件下载 URL
- **THEN** 该 URL 在 `CacheService` 中缓存 10 分钟
- **AND** 下次读取同一文件时跳过 API 获取

##### Scenario: 否定缓存 (Negative Caching)
- **WHEN** 查找不存在的文件名（如 `.DS_Store`）
- **THEN** `CacheService.SetNeg(parentFid, name)` 记录否定结果
- **AND** 后续同类查找在否定缓存 TTL（默认 60s）内直接返回不存在

#### 第二层：QryptFS 元数据刷新控制

##### Scenario: TTL-based directory refresh
- **WHEN** `Readdir` 被调用，且距离上次 `lastMetadataCheck` 超过 `MetadataTTL`（15s）
- **THEN** 系统重新调用 `fetchFiles` 从远程获取目录列表
- **AND** 调用 `MergeRemoteChanges` 合并远程变更到本地 VFS 树

### Requirement: 并行目录列表

当目录文件数超过单页（100）时，系统必须采用 Go 协程并发抓取所有后续页面，以最小化总等待时间。

### Requirement: 子树预缓存

`Readdir` 在刷新当前目录后，自动对子目录发起后台预缓存（`prefetch` 协程），每个子目录也遵循自身的 `MetadataTTL` 检查。
