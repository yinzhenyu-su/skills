## Why

当前基金管理器在处理买入操作时，若当日官方净值尚未公布（常见于白天交易时段），用户无法获得准确的成交份额。此外，各数据提供者（Provider）硬编码了不同的 User-Agent，缺乏统一管理，且未处理东方财富等平台的防盗链（Referer）校验，导致数据抓取不够健壮。

## What Changes

- **统一 HTTP 客户端**: 将 User-Agent 提取到配置中，并提供全局统一的 `reqwest::Client` 构建逻辑，自动处理必要的 Header（如 Referer）。
- **异步结算机制 (Pending Buy)**: 允许在净值缺失时创建“预买入”记录。
- **自动对账子命令**: 增强 `fund sync` 命令，使其能够自动查询并核销处于 `pending` 状态的交易，补全份额和净值。
- **历史净值接口**: 引入东方财富 `lsjz` 接口，支持按日期精确获取官方净值。

## Capabilities

### New Capabilities
- `unified-http-client`: 提供统一的 HTTP 请求管理和反爬虫策略绕过。
- `asynchronous-settlement`: 支持 T+N 模式的交易记账与自动核销逻辑。

### Modified Capabilities
- `transaction-tracking`: 扩展交易记录状态，支持从 `pending` 到 `settled` 的状态流转。

## Impact

- `src/config.rs`: 增加默认 UA 配置。
- `src/db.rs`: 修改 `transaction_log` 表结构及相关的增删改查函数。
- `src/provider/`: 新增 `lsjz` 接口实现，重构现有 Provider 使用统一 Client。
- `src/main.rs`: 升级 `sync_funds` 逻辑，集成自动对账引擎。
