## ADDED Requirements

### Requirement: Automated Data Sync
系统必须能根据基金代码，从外部接口自动获取最新的单位净值、累计净值和申购费率。

#### Scenario: Sync on fund addition
- **WHEN** 用户执行 `fund add 000300`
- **THEN** 系统从网络抓取其最新信息，存入本地数据库，并记录今日的净值记录。

### Requirement: Historical Net Value Storage
系统必须按日期存储基金的净值数据，每天保留一条记录。

#### Scenario: Update existing NAV for same day
- **WHEN** 同一天内多次触发同步，且获取到的最新净值有更新
- **THEN** 系统更新该日期对应的净值记录，而不是创建新记录。
