## ADDED Requirements

### Requirement: Preview Buy - Input and NAV Resolution
系统必须支持买入预览功能，用户输入基金和金额/份额后，系统查询净值并展示手续费和份额信息。

#### Scenario: Preview buy with money input
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000`
- **THEN** 系统查询基金 000312 的当前最新净值，计算手续费和获得份额

#### Scenario: Preview buy with shares input
- **WHEN** 用户执行 `fund preview buy 000312 --shares 4538.65`
- **THEN** 系统查询基金 000312 的当前最新净值，计算需要投入的金额和手续费

#### Scenario: Preview buy with explicit nav
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000 --nav 1.05`
- **THEN** 系统使用指定净值 1.05 进行计算，不查询 API

#### Scenario: Preview buy with date
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000 --date 2024-01-01`
- **THEN** 系统查询 2024-01-01 之前的最近净值进行计算

### Requirement: Preview Buy - Output Display
买入预览必须展示投入金额、手续费、获得份额，以及买入后持仓变化。

#### Scenario: Preview buy output format
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000` 且当前净值为 1.1、费率为 0.15%
- **THEN** 系统展示：投入金额、费率、手续费、获得份额，以及当前持仓到买入后的变化（份额、成本、均价）

#### Scenario: Preview buy with zero existing holdings
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000`
- **AND** 钱包中该基金无持仓
- **THEN** 系统展示买入后持仓 = 买入数量，成本 = 投入金额

### Requirement: Preview Sell - Input and NAV Resolution
系统必须支持卖出预览功能，用户输入基金和份额/金额后，系统查询净值并展示赎回金额和手续费。

#### Scenario: Preview sell with shares input
- **WHEN** 用户执行 `fund preview sell 000312 --shares 5000`
- **THEN** 系统查询基金 000312 的当前最新净值，计算赎回金额和手续费

#### Scenario: Preview sell with money input
- **WHEN** 用户执行 `fund preview sell 000312 --money 5500`
- **THEN** 系统查询基金 000312 的当前最新净值，计算需要卖出的份额和手续费

#### Scenario: Preview sell with explicit nav
- **WHEN** 用户执行 `fund preview sell 000312 --shares 5000 --nav 1.05`
- **THEN** 系统使用指定净值 1.05 进行计算，不查询 API

#### Scenario: Preview sell with date
- **WHEN** 用户执行 `fund preview sell 000312 --shares 5000 --date 2024-01-01`
- **THEN** 系统查询 2024-01-01 之前的最近净值进行计算

### Requirement: Preview Sell - Output Display
卖出预览必须展示卖出份额、赎回金额、手续费、实际到账，以及卖出后持仓变化。

#### Scenario: Preview sell output format
- **WHEN** 用户执行 `fund preview sell 000312 --shares 5000` 且当前净值为 1.1、费率为 0%
- **THEN** 系统展示：卖出份额（占当前持仓比例）、赎回金额、费率、手续费、实际到账，以及当前持仓到卖出后的变化（份额、成本、均价）

#### Scenario: Preview sell exceeds holding
- **WHEN** 用户执行 `fund preview sell 000312 --shares 20000`
- **AND** 钱包中该基金仅持有 10000 份
- **THEN** 系统展示警告："卖出份额超出当前持仓 10000 份"

### Requirement: Preview Does Not Execute Transaction
预览功能仅做展示，不执行实际交易。

#### Scenario: Preview buy does not create transaction
- **WHEN** 用户执行 `fund preview buy 000312 --money 5000`
- **THEN** 系统仅展示预览信息，不创建任何交易记录

#### Scenario: Preview sell does not create transaction
- **WHEN** 用户执行 `fund preview sell 000312 --shares 5000`
- **THEN** 系统仅展示预览信息，不创建任何交易记录
