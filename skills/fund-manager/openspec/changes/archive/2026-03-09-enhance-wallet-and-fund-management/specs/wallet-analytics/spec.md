## ADDED Requirements

### Requirement: Wallet Summary Display
系统必须在列出钱包时，显示每个钱包的综合财务数据。

#### Scenario: Listing wallets with analytics
- **WHEN** 用户执行 `fund wallet list`
- **THEN** 系统显示一个表格，包含：
    - 是否为当前活跃 (标记 `*`)
    - 钱包名称
    - 总估值 (Valuation)
    - 总投入成本 (Net Cost)
    - 总盈亏金额 (Total P&L)
    - 总收益率 (Total P&L %)
