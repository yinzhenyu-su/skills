## 1. 数据库层

- [x] 1.1 在 `db.rs` 中新增 `find_next_available_nav(conn, code, start_date, max_days)` 函数，返回 `(实际日期, NAV)`
- [x] 1.2 编写单元测试，覆盖：当天有 NAV、顺延 1-3 天、20 天都无 NAV 的场景

## 2. CLI 定义调整

- [x] 2.1 在 `cli.rs` 中移除 `Buy` 命令的 `auto` 参数
- [x] 2.2 在 `cli.rs` 中移除 `Sell` 命令的 `auto` 参数
- [x] 2.3 验证 `fund buy --auto` 会报错

## 3. buy 命令集成

- [x] 3.1 修改 `main.rs` 中 `buy` 命令的处理逻辑，调用 `find_next_available_nav()` 替代 `get_nav_at_date()`
- [x] 3.2 当实际日期与指定日期不同时，输出提示信息
- [x] 3.3 处理 20 天内无 NAV 的情况（pending）

## 4. sell 命令集成

- [x] 4.1 修改 `main.rs` 中 `sell` 命令的处理逻辑，同样使用智能查找
- [x] 4.2 sell 的 NAV 查找方向：查找指定日期**之前**最近的可用 NAV（因为是卖出时看前一天的净值）

## 5. import 命令集成

- [x] 5.1 修改 `main.rs` 中 `import` 命令的处理逻辑，使用智能查找
- [x] 5.2 保持原有的 merge/override 逻辑不变

## 6. 本地找不到 NAV 时调用 API 获取

- [x] 6.1 在 `main.rs` 中，当 `find_next_available_nav` 返回 None 时，尝试调用 API 获取指定日期的 NAV
- [x] 6.2 调用 `EastmoneyLsjzProvider::fetch_by_date()` 获取指定日期的净值
- [x] 6.3 将 API 返回的 NAV 保存到本地数据库
- [x] 6.4 如果 API 返回有效 NAV，重新计算并创建交易
- [x] 6.5 如果 API 也找不到，再往后查找 20 天

## 7. sync 命令调整

- [x] 7.1 在 `cli.rs` 中添加 `--all` 参数到 `Sync` 命令
- [x] 7.2 修改 `sync_funds()` 函数，支持 `--all` 模式：
  - 遍历本地所有基金
  - 同步基金基本信息
  - 同步最近 30 天的净值数据
- [x] 7.3 保持现有的单独指定基金代码和起止日期的功能
- [x] 7.4 运行 `cargo test` 确保测试通过

## 8. 验证与测试

- [x] 8.1 运行 `cargo check` 确保编译通过
- [x] 8.2 运行 `cargo test` 确保单元测试通过
- [x] 8.3 手动测试：指定节假日日期，验证顺延行为
- [x] 8.4 手动测试：验证 --auto 参数已被移除
