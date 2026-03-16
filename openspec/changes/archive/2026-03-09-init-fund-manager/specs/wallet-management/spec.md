## ADDED Requirements

### Requirement: Wallet Creation
系统必须允许用户创建一个命名的钱包。

#### Scenario: Successful wallet creation
- **WHEN** 用户执行 `fund wallet add "Savings"`
- **THEN** 系统在数据库中创建一个名为 "Savings" 的新钱包，并提示创建成功。

### Requirement: Active Wallet Selection
系统必须支持设置一个“当前活跃”的钱包，作为后续操作的上下文。

#### Scenario: Switching active wallet
- **WHEN** 用户执行 `fund wallet use "Savings"`
- **THEN** 系统更新配置，使后续所有基金操作（如买入、查看状态）都默认针对 "Savings" 钱包进行。
