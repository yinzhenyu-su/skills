# Asynchronous Settlement

## Requirements

1.  **Pending 状态记录**: 在执行 `buy` 且当日官方净值不可用时（20 天内都不可用），必须能够存储一笔 `pending` 状态的交易。
2.  **交易字段支持**: `transaction_log` 的 `shares` 和 `nav` 字段应支持存储 `NULL`。
3.  **自动核销机制**: `fund sync` 应定期扫描所有 `pending` 记录，并尝试通过 `lsjz` 接口查询买入日当天的官方净值。
4.  **核销幂等性**: 对单笔记录的核销不应因重复运行而产生错误。
5.  **查询支持**: `get_holdings` 等统计函数必须正确处理 `pending` 记录（通常忽略份额，直到正式成交）。

## Interface

### Database
- `transaction_log` 字段变更:
    - `shares`: `TEXT` (Nullable)
    - `nav`: `TEXT` (Nullable)
    - `status`: `TEXT` (Default: 'settled')

### Provider
- `LsjzProvider`: 获取特定基金在特定日期的 `DWJZ`。
