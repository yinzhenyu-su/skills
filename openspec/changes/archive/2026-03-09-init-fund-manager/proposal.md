## Why

当前用户缺乏一个统一、自动化的工具来管理分散在不同基金平台（如天天基金、银行等）的投资数据。手动记录不仅费时费力，且难以实时掌握最新的净值、费率和盈亏情况。通过 Rust 开发一个跨平台的命令行工具，可以实现数据的本地私密存储（SQLite）与云端数据的自动同步，提升投资决策效率。

## What Changes

- **核心基础设施**：引入基于 SQLite 的本地数据库，支持多钱包（Wallet）管理。
- **自动化同步**：实现从外部 API 自动抓取基金最新净值、累计净值及申购费率。
- **智能交易记录**：支持通过金额自动换算份额（考虑实时费率和净值）。
- **数据管理**：提供基金、钱包、交易流水及历史净值的全生命周期管理命令。
- **跨平台支持**：支持 macOS, Linux 和 Windows 的标准配置存储路径。

## Capabilities

### New Capabilities
- `wallet-management`: 提供钱包的创建、切换、修改和删除功能，作为所有基金资产的逻辑隔离层。
- `fund-sync`: 自动从互联网获取指定基金的实时数据（净值、费率、名称），并按日期持久化存储。
- `transaction-tracking`: 记录买入、卖出等流水，支持基于金额和费率的自动份额计算逻辑。
- `portfolio-analytics`: 基于最新净值和持仓流水，实时计算持仓成本、当前估值及浮动盈亏。

### Modified Capabilities
- 无（初始项目创建）

## Impact

- **存储**：在用户主目录下创建 SQLite 数据库文件。
- **网络**：需要访问外部基金数据 API。
- **依赖**：引入 `clap` (CLI), `rusqlite` (DB), `reqwest` (HTTP), `rust_decimal` (Financial Math), `tokio` (Async)。
