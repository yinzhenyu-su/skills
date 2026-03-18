## Why

虽然大部分核心界面已经汉化，但在错误处理信息、深度分析解释以及批量导入的分行提示中仍遗留了约 10% 的英文内容。为了提供完全一致的母语操作体验，需要对这些细节进行彻底补完。

## What Changes

- **错误信息补完**：翻译钱包未找到、未选择活跃钱包、基金未在 DB 发现等底层逻辑报错。
- **深度分析解释补完**：汉化 `inspect` 命令中关于投资者获得感的详细说明。
- **批量导入提示补完**：翻译 CSV 解析相关的分行提示、文件打开失败及金额校验失败的提示。
- **状态日志精修**：统一翻译 `Sync completed` 和 `PREVIEW` 等交互式细节。

## Capabilities

### Modified Capabilities
- `chinese-localization`: 扩展汉化规范，覆盖到底层逻辑报错和详细背景解释。
- `history-backfill`: 调整同步历史过程中的英文提示。

## Impact

- `skills/fund-manager/src/main.rs`: 修改剩余的英文字符串常量。
- `skills/fund-manager/src/sync.rs`: 翻译同步和结算过程中的英文日志。
- `skills/fund-manager/tests/`: 更新测试断言以适配新的中文提示。
