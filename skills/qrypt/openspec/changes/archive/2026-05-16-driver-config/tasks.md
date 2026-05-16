## 1. Config 结构改造

- [x] 1.1 修改 `internal/config/config.go`：新增 `DriveConfig`、`QuarkOptions`、`Yun139Options` 结构体
- [x] 1.2 修改 `LoadConfig`：添加向后兼容逻辑，检测旧 `[quark]` 节并自动迁移到 `drive.type="quark"`
- [x] 1.3 修改 `DefaultConfig`：默认 `drive.type = "quark"`，填充 `Drive.Quark` 默认值
- [x] 1.4 确保 `go build ./internal/config/...` 通过

## 2. 工厂函数

- [x] 2.1 创建 `internal/drive/factory/factory.go`：`NewDriverFromConfig(cfg DriveConfig) (Driver, error)` 工厂函数，支持 quark/yun139（因 import cycle 放子包）
- [x] 2.2 确保 `go build ./internal/drive/...` 通过

## 3. CLI 接入

- [x] 3.1 修改 `cmd/qrypt/mount.go`：使用 `factory.NewDriverFromConfig` 替代手动 `quark.NewDriver` 调用
- [x] 3.2 修改 `cmd/qrypt/mount.go`：新增 `--drive-type` flag，覆盖配置文件 `drive.type`
- [x] 3.3 修改 `cmd/qrypt/util.go`：`loadToolDriver` 使用工厂函数创建驱动
- [x] 3.4 确保 `go build ./cmd/qrypt/...` 通过

## 4. 配置示例更新

- [x] 4.1 更新 `qrypt.toml`：添加 `[drive]` 和 `[drive.quark]` 节示例
- [x] 4.2 更新 `init.go` 模板：`qrypt init` 生成新格式配置文件
- [x] 4.3 确保 `go build ./...` 全量通过

## 5. 测试与验证

- [x] 5.1 运行 `go test ./...` —— 回归覆盖通过（仅 1 个预存在的 TestE2E_RenameDir 失败，非本次变更导致）
- [x] 5.2 验证旧 `qrypt.toml` 格式（无 `[drive]` 节）仍可正常解析
