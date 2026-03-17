## 1. 基础逻辑与测试

- [x] 1.1 在 `src/finance.rs` 中新增 `parse_percentage_rate(input: &str) -> Decimal` 函数，并确保其能够正确处理带百分号的字符串（例如 "0.15%" 转换为 0.0015）。
- [x] 1.2 在 `src/finance.rs` 中增加单元测试，重点验证极低费率（如 0.15% 和 0.12%）下的解析与计算准确性。

## 2. 命令逻辑修复

- [x] 2.1 修改 `src/main.rs` 中的 `Buy` 命令分支，将获取费率的逻辑从 `management_fee` 调整为 `sales_fee`。
- [x] 2.2 修改 `src/main.rs` 中的 `handle_import` 函数，同步应用新的费率解析和字段逻辑。
- [x] 2.3 确保在费率不可用时，系统降级使用 0.15% 默认费率，并向用户打印使用的费率信息。

## 3. 验证与回归

- [x] 3.1 运行 `cargo test` 确保所有单元测试和集成测试通过。
- [x] 3.2 手动验证：重新执行 `fund-manager buy --money 100 160119 --date 2026-03-13`，确认手续费为 0.15 元。
