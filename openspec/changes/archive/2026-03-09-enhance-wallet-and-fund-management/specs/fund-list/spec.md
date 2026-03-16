## ADDED Requirements

### Requirement: Global Fund Inventory
系统必须支持列出当前所有已追踪的基金。

#### Scenario: Listing all funds
- **WHEN** 用户执行 `fund fund list`
- **THEN** 系统显示一个表格，包含：
    - 基金代码
    - 基金名称
    - 当前申购费率 (Fee Rate)
    - 最后同步时间
