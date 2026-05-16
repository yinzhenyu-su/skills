## 1. 接口定义 (internal/drive/)

- [x] 1.1 创建 `internal/drive/driver.go`，定义 `Entry` 结构体、`Meta` / `Reader` / `Writer` / `Uploader` / `Driver` 接口
- [x] 1.2 创建 `internal/drive/errors.go`，定义通用错误类型（`ErrNotImplemented`、`ErrNotFound` 等）
- [x] 1.3 确保编译通过：`go build ./internal/drive/...`

## 2. Quark 驱动迁移 (internal/drive/quark/)

- [x] 2.1 创建 `internal/drive/quark/driver.go`：`QuarkDriver` 结构体，实现 `Meta.Init()` / `Meta.Drop()` / `Reader.List()` / `Reader.Read()` / `Writer.Mkdir()` / `Writer.Move()` / `Writer.Rename()` / `Writer.Remove()` / `Uploader.Put()`
- [x] 2.2 创建 `internal/drive/quark/client.go`：从 `internal/quark/client.go` 迁移 HTTP client，保留 Cookie 续期、请求路由、多基地址回退逻辑
- [x] 2.3 创建 `internal/drive/quark/types.go`：定义 Quark API 响应结构 + `quark.File → drive.Entry` 转换函数
- [x] 2.4 在 `drive/quark/driver.go` 内部实现 `List()`：调用 `/file/sort` API，支持并发翻页，返回 `[]drive.Entry`
- [x] 2.5 在 `drive/quark/driver.go` 内部实现 `Read()`：获取 OSS 下载 URL，支持 Range 请求、URL 缓存（10min TTL）、403 重试
- [x] 2.6 在 `drive/quark/driver.go` 内部实现 `Mkdir()` / `Move()` / `Rename()` / `Remove()`：调用对应管理 API
- [x] 2.7 在 `drive/quark/driver.go` 内部实现 `Put()`：封装六步上传流程（PreUpload → Auth → UploadPart × N → Hash → Commit → Finish），支持分片重试
- [x] 2.8 确保编译通过：`go build ./internal/drive/quark/...`

## 3. FUSE 层接入 (internal/fs/)

- [x] 3.1 修改 `internal/fs/fs.go`：`NewFS` 构造函数接收 `drive.Driver` 接口，删除 `*quark.FileService` / `ManageService` / `UploadService` 参数
- [x] 3.2 修改 `internal/fs/fs.go`：创建 `QryptFS` 时通过类型断言判断 driver 是否实现 `Writer` / `Uploader`，决定是否启用写操作
- [x] 3.3 修改 `internal/fs/readdir.go`：`MergeRemoteChanges` 接收 `[]drive.Entry` 而非 `[]quark.File`
- [x] 3.4 修改 `internal/fs/readdir.go`：`fetchFiles` 改为调用 `driver.List()`，返回 `[]drive.Entry`
- [x] 3.5 修改 `internal/fs/read.go`：`getDecryptedChunk` 中读取云端数据改为调用 `driver.Read()`
- [x] 3.6 修改 `internal/fs/read.go`：`fetchBatch` / `prefetch` 中的云端读取改为调用 `driver.Read()`
- [x] 3.7 修改 `internal/fs/write.go`：`Create` / `Write` 中的目录查找改为使用 `driver.List()` 做路径解析
- [x] 3.8 修改 `internal/fs/delete.go`：`Unlink` / `Rmdir` 中的 API 调用改为 `driver.Remove()`
- [x] 3.9 修改 `internal/fs/rename.go`：`Rename` 中的 API 调用改为 `driver.Rename()` / `driver.Move()`
- [x] 3.10 修改 `internal/fs/tree.go`（或对应文件）：`lookup` 路径解析使用 `driver.List()` 逐级查找
- [x] 3.11 确保编译通过：`go build ./internal/fs/...`

## 4. Sync 层接入 (internal/sync/)

- [x] 4.1 修改 `internal/sync/uploader.go`：`Uploader` 持有 `drive.Driver` 接口，`Upload()` 内部调用 `driver.Put()`
- [x] 4.2 调整 `Request` / `Result` 结构体：使用 `drive.Entry` 替代 `quark` 类型
- [x] 4.3 确保编译通过：`go build ./internal/sync/...`

## 5. CLI / Config 层 (cmd/qrypt/)

- [x] 5.1 修改 `cmd/qrypt/mount.go`：创建 `drive/quark.Driver` 替代原有的 `quark.FileService`/`ManageService`/`UploadService` 组装逻辑
- [x] 5.2 修改 `cmd/qrypt/push.go`：使用新的 `sync.NewUploader` 签名
- [ ] 5.3 后续 CLI 工具命令转换（ls/cat/rm/mv/find/pull/status） / `find.go` / `status.go` 等工具命令：使用 `drive.Driver` 接口
- [ ] 5.3 修改 `cmd/qrypt/push.go` / `pull.go`：使用 `drive.Driver.Put()` / `drive.Driver.Read()`
- [ ] 5.4 清理不再需要的 service 创建代码
- [ ] 5.5 确保编译通过：`go build -o qrypt ./cmd/qrypt`

## 6. 本地文件系统驱动（测试用）

- [ ] 6.1 创建 `internal/drive/localfs/driver.go`：实现 `Meta` + `Reader` + `Writer` + `Uploader`
- [ ] 6.2 `List()` 实现为读取本地目录
- [ ] 6.3 `Read()` 实现为读取本地文件的指定范围
- [ ] 6.4 `Put()` 实现为写入本地文件
- [ ] 6.5 确保编译通过

## 7. 测试与验证

- [ ] 7.1 为 `drive/quark/` 添加单元测试：mock HTTP server 测试 List / Read / Upload 流程
- [ ] 7.2 为 `drive/localfs/` 添加单元测试：验证本地文件系统操作
- [ ] 7.3 运行现有测试：`go test ./...` 确保回归覆盖
- [ ] 7.4 用 localfs driver 挂载验证基本 FUSE 操作
- [ ] 7.5 用真实 Quark driver 挂载验证端到端流程（ls、cat、upload、delete、rename）
