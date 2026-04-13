## ADDED Requirements

### Requirement: macFUSE 文件系统挂载
系统必须调用 `cgofuse` 或原生 macFUSE 接口，将虚拟文件系统挂载到本地目录。

#### Scenario: 成功挂载
- **WHEN** 用户执行 `qrypt mount /path/to/mount`
- **THEN** 系统在本地创建挂载点，并在 Finder 中显示为网络磁盘

### Requirement: macOS 元数据过滤
系统必须在文件系统入口拦截以 `.` 开头的所有元数据相关请求（如 `.DS_Store`, `._*`）。

#### Scenario: 过滤 Finder 请求
- **WHEN** Finder 请求访问挂载点根目录下的 `.DS_Store`
- **THEN** FUSE 驱动层直接返回 `ENOENT` (文件不存在) 而不发起网络请求

### Requirement: 读写吞吐优化
系统必须支持 FUSE 的分块读取和写入（如 128KB 每个请求），并将其汇聚到本地分块缓存中。

#### Scenario: 大文件读取
- **WHEN** 用户播放 1GB 的视频文件
- **THEN** FUSE 驱动层按需向 VFS 请求分块，确保播放器能够获得持续的数据流
