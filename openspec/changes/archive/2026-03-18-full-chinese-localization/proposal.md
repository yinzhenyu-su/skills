## Why

目前 `fund-manager` 的命令行帮助信息、表格表头、运行日志等大部分内容仍为英文。为了提升中国用户的操作体验，降低使用门槛，并使报表展示更符合中文金融术语习惯，需要进行全方位的中文本地化。

## What Changes

- **CLI 帮助信息汉化**：修改 `cli.rs`，将命令描述、参数说明、示例及应用说明全部翻译为中文。保持一级命令名（如 `buy`, `status` 等）为英文不变。
- **报表与表头汉化**：修改 `main.rs` 中的表格输出逻辑，将 `status`, `inspect`, `fund list`, `wallet list` 等命令生成的表格表头改为中文。
- **运行日志与交互汉化**：翻译 `main.rs`, `sync.rs`, `resolver.rs` 中的运行状态日志、确认提示、成功/警告信息。
- **视觉风格统一**：在汉化过程中统一使用中文标点符号和金融术语（如 "Valuation" -> "当前市值"）。

## Capabilities

### New Capabilities
- `chinese-localization`: 定义系统全方位的中文展示规范，包括帮助文案、表格模板和交互语汇。

### Modified Capabilities
- `smart-search`: 调整搜索过程中的提示信息。
- `metadata-sync`: 调整同步进度的显示文案。
- `test-infrastructure`: 更新测试中的断言以匹配中文输出。

## Impact

- `skills/fund-manager/src/cli.rs`: 大量字符串替换。
- `skills/fund-manager/src/main.rs`: 修改表格表头和 `println!/eprintln!` 内容。
- `skills/fund-manager/src/sync.rs`: 修改进度日志。
- `skills/fund-manager/src/resolver.rs`: 修改搜索和解析相关的日志。
- `skills/fund-manager/tests/`: 需更新集成测试以匹配新的中文输出。
