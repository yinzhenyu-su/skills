## Context

当前路径选择完全依赖 `dirs::config_dir()`。在 macOS 上这映射到 `~/Library/Application Support`，这对于用户手动检查数据库或配置文件并不方便。

## Goals / Non-Goals

**Goals:**
- 在 Unix 系统（macOS/Linux）上实现 XDG 风格的配置路径。
- 在 Windows 上保留原有的 AppData 路径。
- 保证环境变量优先级。

**Non-Goals:**
- 不实现复杂的自动数据迁移（由于用户反馈旧路径可能每次都在变，默认重新初始化新路径即可）。

## Decisions

### 1. 路径构建方式
- **Decision**: 使用 `dirs::home_dir()` + `.config/fund-manager` 来手动构建 Unix 路径。
- **Rationale**: `dirs::config_dir()` 在不同 Unix 发行版上表现不一，显式定义 `.config` 更符合当前开发者的直觉。

### 2. 条件编译
- **Decision**: 使用 `#[cfg(unix)]` 和 `#[cfg(windows)]` 或简单的逻辑分支。
- **Rationale**: 静态区分平台比运行时检测更清晰且能优化掉不相关的依赖路径。

## Risks / Trade-offs

- **[Risk] 旧数据丢失** → **Mitigation**: 文档说明路径变更。由于当前版本仍在开发早期，路径稳定性比数据迁移更重要。
