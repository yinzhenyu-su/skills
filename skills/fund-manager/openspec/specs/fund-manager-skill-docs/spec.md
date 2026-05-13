## ADDED Requirements

### Requirement: SKILL.md 命令示例与 CLI 代码一致

SKILL.md 中所有命令示例的层级和名称必须与 `src/cli.rs` 中的定义完全一致，AI 复制示例时能直接成功执行。

#### Scenario: market 命令示例正确

- **WHEN** AI 读取 SKILL.md 中 market 相关内容
- **THEN** 示例使用 `fund-manager market` 而非已废弃的 `fund-manager index`

#### Scenario: import-holding 命令层级正确

- **WHEN** AI 读取 SKILL.md 中 import-holding 相关内容
- **THEN** 示例使用顶层命令 `fund-manager import-holding`，而非 `fund import-holding`

#### Scenario: fund config 命令已补全

- **WHEN** AI 需要配置基金分红方式
- **THEN** SKILL.md 中包含 `fund fund config <基金> --dividend-mode reinvest` 示例

#### Scenario: 保留探索口子

- **WHEN** AI 需要了解命令的完整参数
- **THEN** SKILL.md 通过 `--help` 引导 AI 进一步探索，不穷举所有参数
