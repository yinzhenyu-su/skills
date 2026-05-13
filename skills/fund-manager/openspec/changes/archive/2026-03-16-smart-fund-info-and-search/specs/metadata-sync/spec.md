## ADDED Requirements

### Requirement: 基金详情自动补全
系统在获取到基金代码后，如果本地数据库中缺乏该基金的元数据（如风险等级、经理、费率等），必须自动调用详情接口抓取。

#### Scenario: 首次添加基金同步详情
- **WHEN** 用户尝试买入从未记录过的基金 "020988"
- **THEN** 系统在记录交易前，先调用东方财富详情接口获取详情，并存入数据库 `fund` 表

### Requirement: 基于 TTL 的增量同步
系统必须根据 `last_sync_at` 字段判断是否需要更新本地详情缓存。

#### Scenario: 数据过期自动更新
- **WHEN** 用户操作一个已存在基金，但其 `last_sync_at` 已超过 30 天
- **THEN** 系统在执行主要逻辑前，静默重新抓取最新详情并更新数据库，同时刷新 `last_sync_at`

### Requirement: 手动强制同步
系统应当提供命令供用户手动刷新所有本地基金数据。

#### Scenario: 强制全局同步
- **WHEN** 用户执行 `fund sync --force`
- **THEN** 系统遍历本地 `fund` 表中的所有代码，逐一调用详情接口并强制更新

### Requirement: 详细元数据持久化
系统存入 `fund` 表的元数据必须包含完整的指定字段（详见设计方案）。

#### Scenario: 数据库持久化校验
- **WHEN** 系统完成同步逻辑
- **THEN** 数据库 `fund` 表中的 `fund_type`, `risk_level`, `manager`, `company`, `last_sync_at` 等列均包含非空值
