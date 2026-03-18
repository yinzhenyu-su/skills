## 1. CLI 界面中文化

- [x] 1.1 汉化 `cli.rs` 中的 App `about` 和子命令列表描述。
- [x] 1.2 汉化 `cli.rs` 中 `status`, `history`, `buy`, `sell` 命令的详细说明和示例。
- [x] 1.3 汉化 `cli.rs` 中 `import`, `wallet`, `fund` 命令的详细说明。
- [x] 1.4 汉化所有命令参数（Arguments）的 `help` 描述信息。

## 2. 报表与表格中文化

- [x] 2.1 汉化 `main.rs` 中 `status` 命令的表格表头（Fund -> 基金, Shares -> 份额等）。
- [x] 2.2 汉化 `main.rs` 中 `inspect` 命令的报告表头及指标名称。
- [x] 2.3 汉化 `main.rs` 中 `fund list` 和 `wallet list` 的表格表头。

## 3. 日志、确认提示与交互中文化

- [x] 3.1 汉化 `sync.rs` 中的同步进度日志输出。
- [x] 3.2 汉化 `resolver.rs` 中的搜索发现、解析提示及错误信息。
- [x] 3.3 汉化 `main.rs` 中的操作预览（PREVIEW）和确认提示。
- [x] 3.4 汉化 `main.rs` 中的成功、失败汇总报告（如 Import 汇总）。

## 4. 测试适配与验证

- [x] 4.1 更新 `tests/cli_tests.rs` 中的断言字符串以匹配中文。
- [x] 4.2 运行所有集成测试，确保汉化后逻辑依然正确且断言通过。
- [x] 4.3 在本地终端手动验证表格对齐和 Unicode 字符展示效果。
