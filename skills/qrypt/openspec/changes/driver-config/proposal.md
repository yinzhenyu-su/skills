## Why

当前 `config.Config` 硬编码 `Quark` 字段，驱动参数与加密配置耦合在同一个结构体中。新增驱动（yun139、S3 等）需要修改 Config 结构体，且不同驱动的参数（cookie vs authorization token vs access_key）无法统一表达。

Drive Abstraction Layer 已完成接口抽象，但配置层仍停留在"只支持 Quark"阶段——驱动可以换，配置告诉不了系统用哪个驱动。

## What Changes

- **`internal/config/config.go` 重构**：新增 `DriveConfig`，用 `type` + 按驱动分组的指针字段表达不同驱动的参数。
- **新增工厂函数**：`drive.NewDriverFromConfig(cfg DriveConfig) (drive.Driver, error)`，按 `type` 字段实例化对应驱动。
- **TOML 配置格式变更**：`[quark]` → `[drive]` + `[drive.quark]` 二层结构，加密参数剥离为独立 `[encryption]`。
- **向后兼容过渡**：未设置 `drive.type` 时默认走 Quark 旧路径。
- **CLI 参数调整**：`--cookie` 等 Quark 专用 flag 改为可选，新增 `--drive-type` 选择和未来扩展。

## Capabilities

### New Capabilities
- `driver-config`: 统一驱动配置模型。通过 `drive.type` 选择后端，每种驱动有独立的 TOML 配置节。配置层与驱动层解耦，新驱动只需加一个新 Options 结构体。

### Modified Capabilities
- `quark-driver`: 配置方式从顶层 `[quark]` 迁移到 `[drive.quark]`，行为不变。

## Impact

- `internal/config/config.go` — **重构**：`Config` 结构体新增 `DriveConfig`，`QuarkConfig` 迁移为 `QuarkOptions` 作为 `DriveConfig` 的指针字段
- `internal/drive/factory/factory.go` — **新增**工厂函数 `NewDriverFromConfig`
- `cmd/qrypt/mount.go` — **修改**：使用工厂函数创建驱动，支持 `--drive-type` flag
- `cmd/qrypt/util.go` — **修改**：`loadToolDriver` 支持多种驱动
- `qrypt.toml` — **格式变更**：`[quark]` 变为 `[drive]\ntype="quark"\n\n[drive.quark]`
- 依赖无变化
