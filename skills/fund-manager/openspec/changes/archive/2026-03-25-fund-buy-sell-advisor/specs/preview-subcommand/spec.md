## MODIFIED Requirements

### Requirement: preview sell 集成分档赎回费率

`preview sell` 命令在计算预计到手金额时 SHALL 自动查询 `redemption_fee_tiers` 数据，根据持有天数选取正确档位，并在输出中明确展示使用的费率档位和持有天数依据。若用户显式传入 `--fee` 参数，则以用户输入优先。

#### Scenario: preview sell 自动选取持有天数对应档位

- **WHEN** 用户运行 `fund-manager preview sell 000300 --shares 500`，系统检测到持有 200 天，对应赎回费率 0.5%
- **THEN** 输出 SHALL 显示：
  - `赎回份额: 500 份`
  - `持有天数: 200 天`
  - `适用赎回费率: 0.5%（持有 7~364 天档）`
  - `赎回费: <计算值> 元`
  - `预计到手: <计算值> 元`

#### Scenario: preview sell 展示免赎回费的情况

- **WHEN** 用户运行 `fund-manager preview sell 000300 --shares 500`，持有天数 ≥ 365 天，赎回费率 0%
- **THEN** 输出 SHALL 显示"适用赎回费率: 0%（持有满 365 天，免费赎回）"

#### Scenario: preview sell 无费率数据时的提示

- **WHEN** `redemption_fee_tiers` 表中无该基金的赎回费率数据
- **THEN** `preview sell` 输出 SHALL 包含"⚠️ 赎回费率数据不可用，以下金额仅供参考，实际以基金公司为准"，并使用默认费率 0.5% 计算

#### Scenario: preview sell --fee 手动指定时跳过自动查询

- **WHEN** 用户运行 `fund-manager preview sell 000300 --shares 500 --fee 0%`
- **THEN** 系统 SHALL 使用用户指定的费率 0%，输出中显示"赎回费率: 0%（手动指定）"
