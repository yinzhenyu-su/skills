## ADDED Requirements

### Requirement: 赎回费率分档数据存储

系统 SHALL 将每只基金的赎回费率分档存储在 `redemption_fee_tiers` 表中，字段为 `(fund_code, min_days, max_days, fee_rate)`，支持一只基金存储多个档位记录。`fund sync` 时更新。

数据来源优先使用 Morningstar `fees.redemptionFee`，并对 `feeUnit/floorUnit` 执行标准化映射。

#### Scenario: 存储标准三档赎回费率

- **WHEN** `fund sync` 获取到基金 `000300` 的赎回费率为：持有 < 7 天 1.50%、7~364 天 0.50%、≥ 365 天 0%
- **THEN** 系统 SHALL 删除该基金旧有档位并插入三条新记录至 `redemption_fee_tiers` 表

#### Scenario: 存储单档零费率

- **WHEN** `fund sync` 获取到某货币基金赎回费率为 0%（无限制）
- **THEN** 系统 SHALL 存储单条 `fee_rate = 0` 的档位记录

#### Scenario: 无费率数据时优雅处理

- **WHEN** `fund sync` 未能获取到费率分档数据
- **THEN** 系统 SHALL 保留上次已有数据（若有），或将该基金费率记录清空，不向用户显示错误

### Requirement: 分段费率单位标准化

系统 SHALL 对外部分段费率执行统一单位映射，确保数据库层查询和命令层计算口径一致。

#### Scenario: 百分比费率映射

- **WHEN** 外部档位 `feeUnit = 2.0` 且 `fee = 1.5`
- **THEN** 系统 SHALL 将费率按百分比转换为小数 `0.015` 存储

#### Scenario: 固定金额费率映射

- **WHEN** 外部档位 `feeUnit = 1.0` 且 `fee = 1000`
- **THEN** 系统 SHALL 将该档位标记为固定金额类型并保留 `1000` 的数值

#### Scenario: 阈值单位映射

- **WHEN** 外部档位 `floorUnit = 10.0` 且 `floor = 31`
- **THEN** 系统 SHALL 将该档位解释为“持有 31 天起”阈值

### Requirement: 按持有天数查找适用赎回费率

系统 SHALL 提供一个依据 `fund_code` 和 `holding_days` 查询 `redemption_fee_tiers` 表、返回当前适用费率的函数，供 `sell` 和 `preview sell` 命令使用。

#### Scenario: 持有天数落在最低档

- **WHEN** 基金 `000300` 持有 3 天，赎回费率分档为（<7 天 1.5%，7~364 天 0.5%，≥365 天 0%）
- **THEN** 查询函数 SHALL 返回 `1.5%`

#### Scenario: 持有天数落在中间档

- **WHEN** 基金 `000300` 持有 180 天
- **THEN** 查询函数 SHALL 返回 `0.5%`

#### Scenario: 持有天数达到免费条件

- **WHEN** 基金 `000300` 持有 400 天
- **THEN** 查询函数 SHALL 返回 `0%`

#### Scenario: 无费率档位数据时返回默认值

- **WHEN** `redemption_fee_tiers` 表中无该基金记录
- **THEN** 查询函数 SHALL 返回 `None`，调用方应提示用户费率数据不可用

### Requirement: sell 命令集成分档赎回费率

`sell` 命令在记录卖出交易时 SHALL 自动根据持有天数查找赎回费率；若命令行显式传入 `--fee` 参数，则以用户输入优先。

#### Scenario: 自动应用分档赎回费率

- **WHEN** 用户执行 `fund-manager sell 000300 --shares 500`，系统检测到持有 200 天，对应费率 0.5%
- **THEN** 赎回费自动按 0.5% 计算，成交记录写入数据库，输出中显示实际费率来源

#### Scenario: 手动指定费率覆盖自动计算

- **WHEN** 用户执行 `fund-manager sell 000300 --shares 500 --fee 0%`
- **THEN** 系统 SHALL 使用用户指定的 0% 费率，不使用分档查询结果

#### Scenario: 费率数据缺失时的提示

- **WHEN** 执行 `sell` 时无赎回费率档位数据
- **THEN** 系统 SHALL 提示"⚠️ 未找到该基金的赎回费率数据，请通过 --fee 手动指定费率"，并以默认费率 0.5% 继续（非阻断）
