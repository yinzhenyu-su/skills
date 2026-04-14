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

## MODIFIED Requirements

### Requirement: macFUSE 文件系统挂载
系统必须调用 `cgofuse` 或原生 macFUSE 接口，将虚拟文件系统挂载到本地目录。在 macOS 上，必须启用以下兼容性标志：
- `defer_permissions`: 将权限检查委托给 VFS 处理。
- `volname=QuarkDrive`: 设置磁盘卷标。
- `noappledouble`: 减少 `.DS_Store` 等苹果双叉文件的生成干扰。

**注意**：显式 **禁用** `-o local` 模式。虽然该标志能提高 Finder 响应速度，但在 macOS 上会导致系统尝试创建 `.Trashes` 文件夹。在云盘这种基于加密、异构存储的文件系统上，该行为会触发权限错误或 API 限制，导致无法删除文件。禁用后，Finder 将把该卷视为网络位置并执行“立即删除”操作。

#### Scenario: 成功挂载并写入
- **WHEN** 用户执行 `qrypt mount /path/to/mount` 并在 Finder 中尝试拖入文件
- **THEN** 系统能够成功创建文件并开始写入，而不报告 `Operation not permitted`

### Requirement: 用户身份模拟
系统必须正确获取当前进程的 UID/GID，并在 FUSE `Getattr` 中如实返回，以通过操作系统的权限校验。

#### Scenario: UID 匹配
- **WHEN** 用户执行 `ls -l` 查看挂载的文件
- **THEN** 文件的所有者和组应当与当前登录用户一致
