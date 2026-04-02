## ADDED Requirements

### Requirement: 赎回跳档检测
系统 SHALL 自动检测当前持有的基金是否临近赎回费率降档。

#### Scenario: 临近降档提醒
- **WHEN** 某基金持有 6 天，第 7 天起赎回费率由 1.5% 降至 0.5%
- **THEN** 系统 SHALL 判定为“临近降档”（剩余天数 ≤ 7）

#### Scenario: 跨多档位检测
- **WHEN** 某基金存在 7 天、30 天、365 天三个档位，当前持有 28 天
- **THEN** 系统 SHALL 判定为“临近 30 天降档”，并展示量化建议

### Requirement: 跳档量化建议展示
在 `status` 和 `preview sell` 命令中，系统 SHALL 展示临近降档的具体日期及推迟卖出可节省的手续费估值。

#### Scenario: 状态栏决策符号展示
- **WHEN** 检测到某基金临近跳档（≤ 7 天）
- **THEN** 在 `status` 表格的“提醒”列 SHALL 展示 `💡` 符号

#### Scenario: 卖出预览中的拦截提醒
- **WHEN** 用户运行 `preview sell` 且该基金临近跳档
- **THEN** 输出中 SHALL 包含具体的节省金额计算，如“建议：再持有 2 天，手续费可减少 ￥100.00”
