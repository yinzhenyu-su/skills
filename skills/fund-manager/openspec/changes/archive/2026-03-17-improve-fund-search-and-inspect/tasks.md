## 1. 数据库层支持

- [x] 1.1 在 `src/db.rs` 中新增 `search_funds_locally(conn, identifier)` 函数，实现 `LIKE` 搜索逻辑。
- [x] 1.2 编写单元测试验证 `search_funds_locally` 在不同匹配场景下的正确性。

## 2. 解析逻辑重构

- [x] 2.1 修改 `src/resolver.rs` 中的 `resolve_fund` 函数签名，支持 `local_only: bool` 标志。
- [x] 2.2 在 `resolve_fund` 中集成本地模糊匹配逻辑，并优化多匹配场景下的 `inquire::Select` 处理。
- [x] 2.3 通过集成手动测试验证 `resolve_fund` 的新查找链路优先级。

## 3. 子命令适配与优化

- [x] 3.1 修改 `src/main.rs` 中的 `handle_inspect`，将基金解析设置为交互模式 (`interactive: true`)。
- [x] 3.2 重构 `src/main.rs` 中的 `Sync` 命令处理逻辑，使用 `resolve_fund` 替换直接数据库查询。
- [x] 3.3 重构 `src/main.rs` 中的 `Delete` 命令，使用 `resolve_fund(..., local_only: true)`。
- [x] 3.4 修改 `src/main.rs` 中的 `handle_import` 及其他位置，确保 `resolve_fund` 调用符合新签名。

## 4. 验证与集成测试

- [x] 4.1 运行现有测试确保无回归风险。
- [x] 4.2 通过手动模拟名称查询 (`inspect` 和 `delete`) 完成功能验证。
