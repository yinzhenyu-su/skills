## Context

当前系统仅支持抓取东方财富和同花顺的基础净值信息。晨星（Morningstar）提供的深度分析数据（如夏普比率、卡玛比率、晨星评级、投资者回报 gap 等）是提高 `fund-manager` 专业性的关键。

## Goals / Non-Goals

**Goals:**
- 实现 `MorningstarProvider` 以解析复杂的晨星深度分析 JSON。
- 扩展数据库 Schema 以持久化这些随时间变化的深度指标。
- 提供 `fund inspect <CODE>` 命令，以直观的 ASCII 表格展示“基金体检报告”。
- 实现“投资者损耗（Investor Return Gap）”的计算与展示。

**Non-Goals:**
- 不支持晨星的历史净值数据同步（目前由东方财富处理）。
- 不支持组合（Portfolio）层面的深度分析，仅针对单个基金。

## Decisions

### 1. 新增 `MorningstarProvider` 
- **方案**: 实现 `Provider` trait，但其 `fetch` 方法将返回包含更多 Option 字段的 `FundData`。
- **理由**: 保持与现有 `Aggregator` 的兼容性，同时允许扩展新的分析字段。

### 2. 数据库扩展：`fund_analysis` 表
- **设计**:
  ```sql
  CREATE TABLE fund_analysis (
      fund_code TEXT PRIMARY KEY,
      snapshot_date TEXT,
      rating_3y INTEGER,
      rating_5y INTEGER,
      rank_pct_3y REAL,
      sharpe_3y REAL,
      calmar_3y REAL,
      max_drawdown_3y REAL,
      investor_gap_3y REAL,
      last_update DATETIME DEFAULT CURRENT_TIMESTAMP
  );
  ```
- **理由**: 将分析指标与基础信息分离。这些指标通常月度更新，使用 `PRIMARY KEY (fund_code)` 存储最新快照。

### 3. 交互设计：`fund inspect`
- **UI**: 使用 `comfy_table` 构建一个带边框的体检报告。
- **逻辑**: 如果本地数据过期（如超过 30 天），则在 `inspect` 时自动触发更新。

## Risks / Trade-offs

- **[Risk] 晨星 API 变动** → **Mitigation**: 封装解析逻辑，并在测试中覆盖该 JSON 结构。
- **[Trade-off] 数据不一致** → **Mitigation**: 晨星的分类（Category）可能与东方财富不同，在 `inspect` 时优先显示晨星的分类以保证专业性。
