## ADDED Requirements

### Requirement: 止盈止损目标设置
系统 SHALL 支持通过 `fund config` 命令为每只基金设置个人盈亏目标比例。

#### Scenario: 设置止盈目标
- **WHEN** 用户运行 `fund-manager fund config 000300 --target-profit 15%`
- **THEN** 数据库 SHALL 存储该目标的原始值 `0.15`

#### Scenario: 止盈止损联动
- **WHEN** 用户设置 `--target-profit 15% --stop-loss 10%`
- **THEN** 数据库 SHALL 同时记录两个目标

### Requirement: 盈亏报警展示
系统 SHALL 在 `status` 命令展示持仓时，将当前收益率与设置的目标进行对比，达标时高亮展示。

#### Scenario: 达到止盈目标提醒
- **WHEN** 某基金当前收益率为 18%，目标止盈为 15%
- **THEN** 在 `status` 表格的“提醒”列 SHALL 展示 `🎯` 符号，并在收益率列高亮显示

#### Scenario: 触发止损提醒
- **WHEN** 某基金当前收益率为 -12%，目标止损为 10%
- **THEN** 在 `status` 表格的“提醒”列 SHALL 展示 `⚠️` 符号，并在收益率列显著提示
