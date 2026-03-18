## MODIFIED Requirements

### Requirement: 预配置的 Command 包装器
`TestContext` 必须能够提供预先配置好环境变量（特别是 `FUND_MANAGER_APP_DIR`）的 `assert_cmd::Command` 实例。同时，集成测试中的输出断言 SHALL 适配中文本地化后的字符串。

#### Scenario: 适配中文输出的断言
- **WHEN** 集成测试运行并断言某个操作成功
- **THEN** 测试 SHALL 检查控制台输出是否包含 "成功"、"完成" 或对应的中文提示，而非原有的英文单词。
