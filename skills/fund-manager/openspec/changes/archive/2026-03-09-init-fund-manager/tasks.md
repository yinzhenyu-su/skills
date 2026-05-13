## 1. 项目基础与环境搭建 (Infrastructure)

- [x] 1.1 初始化 Rust 项目，添加 `clap`, `rusqlite`, `rust_decimal`, `tokio`, `reqwest`, `dirs`, `mockall`, `assert_cmd` 依赖.
- [x] 1.2 实现符合操作系统的配置与数据库路径探测逻辑.
- [x] 1.3 编写数据库初始化 SQL 脚本，并为 `NAV History` 添加 `UNIQUE(fund_code, date)` 约束.

## 2. 钱包管理 (TDD: wallet-management)

- [x] 2.1 **[RED]** 编写 `fund wallet add` 的集成测试，断言数据库记录的生成.
- [x] 2.2 **[GREEN]** 实现 `add` 逻辑使测试通过.
- [x] 2.3 **[RED]** 编写切换活跃钱包 (`fund wallet use`) 的测试，验证 context 切换.
- [x] 2.4 **[GREEN]** 实现切换逻辑并持久化活跃 ID 到本地配置.

## 3. 基金同步引擎 (TDD: fund-sync)

- [x] 3.1 **[RED]** 编写测试验证“一天内多次同步只留一条记录”的幂等性逻辑.
- [x] 3.2 **[GREEN]** 实现 `INSERT OR REPLACE` 逻辑使测试通过.
- [x] 3.3 **[RED]** 模拟网络故障，编写测试验证 `fund status` 是否显示“数据可能非最新”警告.
- [x] 3.4 **[GREEN]** 实现捕获 `SyncError` 并展示 UI 警告的逻辑.

## 4. 交易与计算精度 (TDD: transaction-tracking)

- [x] 4.1 **[RED]** 编写单元测试验证 `calculate_purchase` 的份额、费率及舍入精度.
- [x] 4.2 **[GREEN]** 使用 `rust_decimal` 实现计算逻辑，确保高精度计算通过.
- [x] 4.3 **[RED]** 编写买入流水记录的集成测试，断言份额被正确持久化.
- [x] 4.4 **[GREEN]** 实现 `fund buy --auto` 逻辑.

## 5. 投资分析与报表 (TDD: portfolio-analytics)

- [x] 5.1 **[RED]** 编写测试验证在有本地缓存净值的情况下，离线计算盈亏的正确性.
- [x] 5.2 **[GREEN]** 实现盈亏算法，展示当前估值.
- [x] 5.3 **[REFACTOR]** 引入 `comfy-table` 优化输出布局.

## 6. 全流程集成验证

- [x] 6.1 编写“冷启动”端到端测试：初始化 -> 创建钱包 -> 添加基金 -> 自动同步 -> 买入 -> 查看盈亏.
