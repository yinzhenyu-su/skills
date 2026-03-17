## Why

目前 `fund inspect` 命令在处理基金名称查询时过于僵硬：不支持本地模糊匹配，且在非交互模式下遇到重名歧义时会直接报错。这导致用户必须输入精确的基金名称或代码，体验不够友好。

## What Changes

- **增强本地搜索**: 支持在本地数据库中通过基金名称的一部分进行模糊查询（`LIKE` 搜索）。
- **优化解析逻辑**: 改进 `resolve_fund` 函数，使其在本地精确匹配失败后尝试本地模糊匹配，并能根据 `interactive` 标志决定是否弹出选择菜单。
- **提升交互性**: 将 `fund inspect` 命令默认设置为交互模式，在名称匹配有歧义时允许用户手动选择目标基金。
- **一致性重构**: 将 `sync` 和 `delete` 命令的基金解析逻辑统一接入 `resolve_fund`，确保全应用一致的智能查询体验。

## Capabilities

### New Capabilities
- `local-fuzzy-search`: 支持在本地数据库中通过 `LIKE` 语法进行基金名称的部分匹配。

### Modified Capabilities
- `smart-search`: 扩展现有的智能搜索逻辑，优先尝试本地模糊匹配，并优化多匹配场景下的交互选择行为。

## Impact

- `skills/fund-manager/src/db.rs`: 新增 `search_funds_locally` 函数。
- `skills/fund-manager/src/resolver.rs`: 重构 `resolve_fund` 逻辑。
- `skills/fund-manager/src/main.rs`: 修改 `handle_inspect` 及相关命令的处理逻辑。
