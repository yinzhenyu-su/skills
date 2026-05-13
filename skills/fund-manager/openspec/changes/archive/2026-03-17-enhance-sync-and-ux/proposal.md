## Why

目前的 `sync` 命令功能过于单一，仅能同步最新的净值和 30 天过期的元数据。这导致历史净值数据经常出现断档，且无法自动结算过去某天的 `pending` 交易。同时，现有命令的帮助信息缺乏示例和详细描述，对新用户不够友好。

## What Changes

- **增强 `sync` 命令**:
    - 支持 `--start` 和 `--end` 参数，实现指定日期区间的历史净值回填。
    - 引入 `--auto-fill` 逻辑，自动识别 `pending` 交易所需的净值日期并进行针对性同步。
- **完善 CLI 帮助系统**:
    - 为所有子命令增加详细的 `long_about` 描述。
    - 在帮助信息中增加常用的操作示例（Examples）。
    - 改进参数的帮助描述，明确参数格式（如 YYYY-MM-DD）。
- **代码结构优化**: 将同步逻辑从 `main.rs` 抽离，提升可维护性。

## Capabilities

### New Capabilities
- `history-backfill`: 支持批量拉取历史净值并填充数据库。
- `smart-settlement-sync`: 同步命令能够感知待结算交易的日期空缺并自动补全。

### Modified Capabilities
- `unified-http-client`: 需要支持带起始/截止日期的历史净值接口调用。
- `metadata-sync`: 完善元数据同步时的用户反馈。

## Impact

- `src/cli.rs`: 扩展 `Sync` 结构体的参数，并完善各命令的 `help` 文档。
- `src/provider/eastmoney_lsjz.rs`: 扩展以支持日期区间查询。
- `src/main.rs`: 抽离 `sync_funds` 逻辑，更新命令分发。
- `src/resolver.rs` 或新模块: 承载增强后的同步逻辑。
