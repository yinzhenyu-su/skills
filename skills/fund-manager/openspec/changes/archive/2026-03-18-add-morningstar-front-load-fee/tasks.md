## 1. 晨星 Provider 增强

- [x] 1.1 在 `FeesData` 结构体中添加 `front_load_fee: Option<Vec<FrontLoadFeeTier>>` 字段
- [x] 1.2 定义 `FrontLoadFeeTier` 结构体（floor, fee, fee_unit, floor_unit）
- [x] 1.3 在 `FeesResponse` 中添加 `fees` 字段解析
- [x] 1.4 在 `fetch()` 方法中添加 `fee_rate` 提取逻辑（取第一档费率）
- [x] 1.5 添加 `test_parse_front_load_fee` 单元测试

## 2. 457001 真实数据测试用例

- [x] 2.1 在 `cli_tests.rs` 中添加 `test_457001_real_data_purchase` 测试
- [x] 2.2 手动插入 457001 真实净值数据（2026-03-02: 2.2583, 2026-03-16: 2.1146）
- [x] 2.3 验证买入 10000 元后的份额计算（预期 4421.48 份）
- [x] 2.4 验证 status 命令的盈亏计算（预期 -650.34 元，-6.50%）
- [x] 2.5 添加卖出场景测试（卖出 5000 元后验证剩余份额和成本）

## 3. 代码验证

- [x] 3.1 运行 `cargo test test_457001_real_data_purchase` 验证测试通过
- [x] 3.2 运行 `cargo fmt` 格式化代码
- [x] 3.3 运行 `cargo clippy` 确保无警告
