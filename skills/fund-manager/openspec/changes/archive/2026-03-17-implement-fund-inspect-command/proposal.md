## Why

当前 `fund-manager` 仅提供基础的净值追踪和收益计算，缺乏对基金“赚钱质量”和“风险水平”的深度评估。投资者难以判断一个基金的夏普比率、同类排名以及最大回撤等专业指标。接入晨星（Morningstar）的深度数据可以为用户提供专业级的投资参考，帮助其做出更理性的持有或赎回决策。

## What Changes

- **新增 `fund inspect <FUND>` 命令**: 展示指定基金的深度体检报告，包括晨星评级、同类排名、夏普比率、卡玛比率、最大回撤等。
- **扩展 Provider 系统**: 新增 `MorningstarProvider`，专门用于抓取和解析晨星的深度分析数据。
- **数据库增强**: 新增 `fund_analysis` 表，缓存基金的深度评估指标，支持月度/季度更新。
- **展示优化**: 使用 ASCII 表格（`comfy-table`）优雅地展示多维度的专业评估数据。

## Capabilities

### New Capabilities
- `deep-fund-analysis`: 定义如何获取、存储和展示基金的深度风险与收益指标（如夏普比率、评级、排名等）。
- `investor-behavior-insights`: 利用晨星数据分析“基金回报”与“投资者回报”之间的缺口，提供持有建议。

### Modified Capabilities
- `metadata-sync`: 扩展同步功能，支持在同步时可选地拉取深度分析元数据。

## Impact

- **Affected Code**: `src/cli.rs`, `src/main.rs`, `src/db.rs`, `src/provider/`.
- **New Dependencies**: 无（继续使用现有的 `reqwest` 和 `serde_json`）。
- **Database**: 新增 `fund_analysis` 表。
