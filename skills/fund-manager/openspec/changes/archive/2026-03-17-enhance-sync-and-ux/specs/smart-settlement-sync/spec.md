## ADDED Requirements

### Requirement: Automatic Gap Filling
系统应当能够自动识别待处理交易所需的净值数据缺失，并在同步时优先补全。

#### Scenario: Auto-sync for pending transactions
- **WHEN** 数据库中存在一笔日期为 `2024-03-01` 的 `pending` 交易，且本地无该日净值
- **THEN** 用户执行 `fund sync --auto-fill` 时，系统自动将 `2024-03-01` 纳入同步日期区间
