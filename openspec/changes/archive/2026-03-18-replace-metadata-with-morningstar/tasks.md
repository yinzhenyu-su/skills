## 1. 环境准备与 Provider 实现

- [x] 1.1 在 `skills/fund-manager/Cargo.toml` 中确认或添加 `serde_json` 依赖。
- [x] 1.2 创建 `skills/fund-manager/src/provider/morningstar.rs` 文件，并定义 `MorningstarProvider` 结构体。
- [x] 1.3 实现 `MorningstarProvider` 的核心抓取逻辑（调用晨星的 `/common-data` 和 `/fees` 接口）。
- [x] 1.4 完成晨星响应 JSON 的反序列化结构体定义。

## 2. 聚合器与同步逻辑优化

- [x] 2.1 在 `skills/fund-manager/src/provider/mod.rs` 中注册新 Provider。
- [x] 2.2 修改 `skills/fund-manager/src/provider/aggregator.rs`，将 `MorningstarProvider` 插入到 Provider 链的首位（用于高优先级元数据抓取）。
- [x] 2.3 修改 `skills/fund-manager/src/sync.rs` 的同步循环逻辑，支持在同步净值时并行（或按序）调用聚合器更新晨星元数据。

## 3. 测试与验证

- [x] 3.1 为 `MorningstarProvider` 编写单元测试（可模拟 HTTP 响应）。
- [x] 3.2 运行一次真实的同步测试：`cargo run -- fund sync <code-of-a-fund>`，观察数据库中的元数据列是否被正确填充。
- [x] 3.3 运行 `cargo test` 确保无回归。
- [x] 3.4 验证 `fund list` 是否能正确显示以前缺失的基金类型、风险等级等信息。
