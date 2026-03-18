## Context

当前 `fund list` 直接查询 `fund` 表。新的需求需要结合 `nav_history` (获取最新净值) 和 `transaction_log` (针对特定钱包计算持仓)。为了避免 N+1 查询问题，需要设计一个高效的 SQL 联查方案。

## Goals / Non-Goals

**Goals:**
- 提供包含资产实战数据的基金列表。
- 支持自动识别当前活跃钱包并展示对应持仓。
- 保证大量基金（如 100+）时的列表加载性能。
- 兼容无活跃钱包的展示模式。

**Non-Goals:**
- 本次变更不涉及 `wallet list` 的修改。
- 不涉及除展示外的资产计算逻辑变更。

## Decisions

### 1. 数据库联查方案
**选择**: 在 `db.rs` 中新增 `get_funds_with_valuations(conn, wallet_id)` 函数，使用单条 SQL 联查。
**理由**: N+1 查询（先查基金，循环查净值，循环算份额）在 SQLite 中虽快，但代码逻辑复杂且扩展性差。SQL 联查能保证数据的一致性和查询效率。

**SQL 草案**:
```sql
SELECT 
    f.code, f.name, f.fund_type, f.risk_level, f.manager, f.last_sync_at,
    (SELECT nav FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav,
    (SELECT date FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav_date,
    SUM(CASE 
        WHEN t.wallet_id = ?1 AND t.status = 'settled' 
        THEN (CASE WHEN t.type IN ('buy', 'import') THEN CAST(t.shares AS REAL) ELSE -CAST(t.shares AS REAL) END)
        ELSE 0 
    END) as total_shares
FROM fund f
LEFT JOIN transaction_log t ON f.code = t.fund_code
GROUP BY f.code;
```

### 2. CLI 表格动态列
**选择**: 根据 `active_wallet_id` 的存在与否，动态构建 `comfy-table` 的表头和行。
**理由**: 保持表格整洁。如果没有持仓数据，强行显示全为 0 的列会造成视觉干扰。

### 3. 数据格式化
**选择**: 净值显示 4 位小数，份额显示 2 位，总价值显示 2 位并增加千分位。日期显示为 `(MM-DD)`。
**理由**: 符合金融数据展示习惯，节省空间。

## Risks / Trade-offs

- **[Risk] 屏幕宽度不足** → **Mitigation**: 使用 `comfy-table` 的约束，对“类型”、“经理”等列进行截断或根据窗口宽度自动调整。
- **[Trade-off] 实时性** → 列表显示的是数据库中的“最新同步”数据，而非实时抓取。这符合当前系统的离线优先设计，用户需手动运行 `sync` 更新。
