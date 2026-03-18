## ADDED Requirements

### Requirement: 外部 Mock 数据解耦
系统的测试用例不应在 Rust 代码中硬编码超长的 HTML 或 JSON 字符串，而应统一从 `tests/fixtures/` 目录加载对应的响应样本文件。

#### Scenario: 从文件加载测试数据
- **WHEN** 运行 Provider 单元测试
- **THEN** 测试代码通过文件系统读取 `tests/fixtures/<provider>_<sample>.json` 中的内容作为 Mock 输入。

### Requirement: 加载宏支持
系统应支持使用 `include_str!` 宏或类似的编译期辅助方式加载 Fixtures，以确保测试运行时的性能和独立性。

#### Scenario: 编译期 Fixture 加载
- **WHEN** 测试代码中使用 `include_str!("../fixtures/sample.json")`
- **THEN** 对应的文件内容被嵌入到测试二进制中，无需在运行时依赖相对路径。
