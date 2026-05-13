## Why

目前在 Unix 系（macOS 和 Linux）上使用的默认配置目录 `~/Library/Application Support` 路径过深，且可能在某些环境下导致重复创建目录或持久化失效。通过将路径标准化为 `~/.config/fund-manager`，可以提升工具的标准性、可维护性和数据的可见性。同时，在 Windows 上继续保持 `%APPDATA%` 以遵循该系统的原生规范。

## What Changes

- **路径重构**：修改 `get_app_dir` 逻辑，在 Unix 系统上使用 `$HOME/.config/fund-manager` 作为默认路径。
- **平台差异化处理**：通过条件编译或运行时检测，确保 Windows 用户依然使用原有的 `%APPDATA%` 路径。
- **环境隔离**：确保 `FUND_MANAGER_APP_DIR` 环境变量依然保持最高优先级。

## Capabilities

### New Capabilities
- 无

### Modified Capabilities
- `standard-paths`: 规范化跨平台的数据存储路径。

## Impact

- **数据位置**：macOS 和 Linux 用户的数据将从 `Library/Application Support` 迁移或重新创建在 `~/.config` 下。
- **配置模块**：修改 `src/config.rs`。
- **测试**：更新路径相关的单元测试。
