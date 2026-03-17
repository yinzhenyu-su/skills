## 1. 基础设施与模块化

- [x] 1.1 创建 `src/sync.rs` 模块，并将 `main.rs` 中的 `sync_funds` 和 `settle_pending_transactions` 迁移至此。
- [x] 1.2 在 `src/provider/mod.rs` 的 `Provider` trait 中增加 `fetch_range` 异步方法定义。
- [x] 1.3 在 `src/provider/eastmoney_lsjz.rs` 中实现 `fetch_range`，利用 `startDate` 和 `endDate` 参数进行批量抓取。

## 2. CLI 接口与帮助信息

- [x] 2.1 更新 `src/cli.rs` 中的 `Sync` 子命令，增加 `start`, `end`, `auto_fill` 可选参数。
- [x] 2.2 在 `src/cli.rs` 中使用 `long_about` 为所有子命令添加详细描述和 EXAMPLES 章节。
- [x] 2.3 优化参数帮助信息，增加日期格式提示。

## 3. 同步逻辑增强

- [x] 3.1 在 `src/sync.rs` 中实现基于区间的同步逻辑，并确保 `nav_history` 的幂等插入。
- [x] 3.2 实现 `auto_fill` 策略逻辑：扫描 `pending` 交易日期并自动计算同步区间。
- [x] 3.3 在 `main.rs` 中更新命令分发逻辑，调用新的同步接口。

## 4. 验证与测试

- [x] 4.1 编写单元测试验证 `auto_fill` 的日期区间计算算法。
- [x] 4.2 运行集成测试，确保同步命令的增强没有破坏现有功能。
- [x] 4.3 运行 `fund sync --help` 手动验证帮助信息的展示效果。
