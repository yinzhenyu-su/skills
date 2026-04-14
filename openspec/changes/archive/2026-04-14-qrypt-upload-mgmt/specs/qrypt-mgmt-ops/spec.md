## ADDED Requirements

### Requirement: 文件夹管理 (Mkdir/Rmdir)
系统必须支持在挂载点创建和删除文件夹。

#### Scenario: 成功创建文件夹
- **WHEN** 用户在挂载点执行 `mkdir new_folder`
- **THEN** 系统调用夸克 API 在对应父目录下创建文件夹，并在本地 VFS 中立即可见

#### Scenario: 成功删除空文件夹
- **WHEN** 用户执行 `rmdir empty_folder`
- **THEN** 系统调用夸克 API 删除该目录，并从本地 VFS 中移除

### Requirement: 文件删除与重命名 (Unlink/Rename)
系统必须支持删除文件以及在不同目录间移动/重命名文件或文件夹。

#### Scenario: 成功删除文件
- **WHEN** 用户执行 `rm file.txt`
- **THEN** 系统调用夸克 API 删除文件，并清理本地相关缓存

#### Scenario: 成功重命名
- **WHEN** 用户执行 `mv old.txt new.txt`
- **THEN** 系统调用夸克 API 更新文件名，并在本地 VFS 中更新映射
