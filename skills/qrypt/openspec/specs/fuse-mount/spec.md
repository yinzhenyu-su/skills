## ADDED Requirements

### Requirement: macFUSE 文件系统挂载
系统必须调用 `cgofuse` 接口，将虚拟文件系统挂载到本地目录。挂载标志按平台区分：

**macOS (macFUSE)**：
- `rw`: 读写挂载
- `noappledouble`: 减少 `.DS_Store` 等苹果双叉文件的生成干扰
- `defer_permissions`: 将权限检查委托给 VFS 处理
- `volname=QuarkDrive`: 设置磁盘卷标
- `iosize=1048576`: 设置 FUSE 传输块大小为 1MB
- `attr_timeout=0`: 属性缓存超时为 0，每次访问直接从 VFS 获取
- `entry_timeout=0`: 目录项缓存超时为 0
- 显式 **不启用** `-o local` 模式，避免 macOS 尝试创建 `.Trashes` 文件夹

**Linux (libfuse)**：
- `rw`: 读写挂载
- `nonempty`: 允许挂载到非空目录
- `attr_timeout=0`: 属性缓存超时为 0
- `entry_timeout=0`: 目录项缓存超时为 0

#### Scenario: 成功挂载
- **WHEN** 用户执行 `qrypt mount -f qrypt.toml`
- **THEN** 系统在本地创建挂载点，在 Finder 中显示为网络磁盘卷标 `QuarkDrive`
- **AND** 系统在挂载后自动执行 `recoverDirtyFiles` 恢复上次未完成的脏文件上传

### Requirement: macOS 元数据过滤
系统必须在文件系统入口拦截以 `.` 开头的所有元数据相关请求（如 `.DS_Store`, `._*`）。

#### Scenario: 过滤 Finder 请求
- **WHEN** Finder 请求访问挂载点根目录下的 `.DS_Store`
- **THEN** FUSE 驱动层直接返回 `ENOENT` (文件不存在) 而不发起网络请求

### Requirement: 虚拟回收站拦截
系统必须拦截 macOS Finder 对 `.Trash`/`.Trashes` 目录的查询请求，返回虚拟的 Stat 信息，防止系统在云端尝试创建不存在的回收站目录。

### Requirement: 读写吞吐优化
系统必须支持 FUSE 的分块读取和写入，并将其汇聚到本地 staging 暂存区或分块缓存中。

#### Scenario: 大文件顺序读取
- **WHEN** 用户播放 1GB 的视频文件，读取模式表现为顺序访问（连续 2 次以上跨块读取）
- **THEN** FUSE 驱动层自动触发 prefetch，提前预取后续分块到本地缓存，确保播放器获得持续的数据流

### Requirement: macOS Release-Before-Write 兼容
系统必须在 FUSE Release 处理器中正确处理 macOS 的 `release-before-write` 行为，确保写入到 staging 的数据在文件关闭时完整 flush。

#### Scenario: Finder 写入文件
- **WHEN** 用户通过 Finder 拖入文件到挂载点
- **THEN** 系统正确处理多次 Write → Release → Write → Release 的 macOS 典型写入序列
- **AND** 最终写入内容完整无 0-byte 截断
