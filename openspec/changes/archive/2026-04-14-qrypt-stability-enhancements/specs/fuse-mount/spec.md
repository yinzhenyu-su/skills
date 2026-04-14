## MODIFIED Requirements

### Requirement: macFUSE 文件系统挂载
系统必须调用 `cgofuse` 或原生 macFUSE 接口，将虚拟文件系统挂载到本地目录。在 macOS 上，必须启用兼容性标志（如 `defer_permissions`, `local`, `volname`）以确保 Finder 和命令行写入权限正常。

#### Scenario: 成功挂载并写入
- **WHEN** 用户执行 `qrypt mount /path/to/mount` 并在 Finder 中尝试拖入文件
- **THEN** 系统能够成功创建文件并开始写入，而不报告 `Operation not permitted`

### Requirement: 用户身份模拟
系统必须正确获取当前进程的 UID/GID，并在 FUSE `Getattr` 中如实返回，以通过操作系统的权限校验。

#### Scenario: UID 匹配
- **WHEN** 用户执行 `ls -l` 查看挂载的文件
- **THEN** 文件的所有者和组应当与当前登录用户一致
