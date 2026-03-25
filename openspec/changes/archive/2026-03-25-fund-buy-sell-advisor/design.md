## Context

fund-manager 当前具备净值同步、交易记录、持仓盈亏展示和 Morningstar 分析评级等能力，但缺少支撑买卖决策的关键信息层：

1. **可交易性数据缺失**：不知道基金今天是否允许申购/赎回，有无限购门槛
2. **赎回成本不透明**：`sell` 和 `preview sell` 使用固定费率，无法反映按持有天数分档的真实赎回费
3. **持有时间不可见**：用户和系统都无法快速得知"这只基金我拿了多久"
4. **仓位占比缺失**：`status` 只显示绝对金额，不展示每只基金在整体资产中的权重

数据来源探索结论如下：

1. **可稳定获取**：

- 申购/赎回状态：Eastmoney `lsjz` 返回 `SGZT/SHZT`
- 分段费率：Morningstar `fees` 返回 `frontLoadFee/deferLoadFee/redemptionFee`

2. **可获取但口径待定**：

- 最低买入额：`fees.minInvestment` 与 `shareClassFees[].minInvestment` 可能不一致
- 单笔限购：`purchaseAndRedeem.applyingMaxIII/IV/VII/VIII` 语义待映射

3. **当前无稳定来源**：

- 资金到账天数（T+N）

## Goals / Non-Goals

**Goals:**

- 展示基金申购/赎回状态（开放、暂停、限大额），让用户在下单前明确知道当前是否可操作
- 实现赎回费率分档存储与查询，`sell`/`preview sell` 按实际持有天数计算赎回成本
- 在 `status` 命令中展示每只基金的持有天数和当前适用赎回费率档位
- 在 `status` 命令中展示每只基金占钱包总资产的仓位百分比
- 明确字段可用性分级（A/B/C）并对不可用字段提供一致降级行为

**Non-Goals:**

- 自动化投资建议或 AI 评分系统（"应该买/卖"结论由用户自行判断）
- 行情预警或价格提醒推送
- 用户风险偏好建模
- 组合优化推荐
- QDII/债基的特殊清算规则差异化处理（一期统一为 T+N 天）
- 在一期内保证“资金到账天数”字段可用（该字段允许为未知）

## Decisions

### 决策 1：采用多源策略而非单源策略

**选择**：

- 可交易状态：`EastmoneyLsjzProvider` 提供 `SGZT/SHZT`
- 分段费率和申赎限额：`MorningstarProvider` 提供 `redemptionFee/frontLoadFee/deferLoadFee/purchaseAndRedeem`

**理由**：线上与样本验证显示单一接口无法覆盖全部字段，多源组合可显著提升可用率。

**放弃的方案**：继续依赖 `eastmoney_details` 承载状态与分段费率——字段证据不足且稳定性差。

### 决策 2：赎回费率存储为独立表

**选择**：新建 `redemption_fee_tiers` 表，字段为 `(fund_code, min_days, max_days, fee_rate)`，允许一只基金有多个费率档位记录。

**理由**：每只基金的费率档位数量不定（通常 2~4 档），用独立表而非 JSON 字段存储，支持直接 SQL 查询"适用档位"，避免应用层循环解析。

**放弃的方案**：在 `fund` 表中增加 JSON 字段——不利于按持有天数直接查询，且 SQLite 的 JSON 函数可读性差。

### 决策 2.1：分段费率单位标准化

**选择**：将外部费率档位统一标准化为内部结构：`(min_days, max_days, fee_rate_decimal, fee_kind)`。

- `feeUnit = 2.0` 视为百分比，转换为小数（如 `1.5` -> `0.015`）
- `feeUnit = 1.0` 视为固定金额（元）
- `floorUnit` 用于阈值单位映射（金额/天/月）

**理由**：不同基金存在百分比档与固定金额档混合，标准化后便于预览与交易命令一致计算。

### 决策 3：持有天数计算策略——使用首次买入日期

**选择**：以该基金在当前钱包中**最早的 `buy` 或 `import` 类型的已结算交易日期**作为持有起点，计算距今天数。

**理由**：持有天数用于确定赎回费档位，基金赎回时通常从**最早持有的部分**起算（先进先出）。使用首次买入日期是合理的保守估计，且实现最简单。不做逐笔追踪（lot-level），避免过度设计。

**放弃的方案**：加权平均持有天数——计算复杂，且在费率判断中意义不大（费率关注的是"有没有满足某个天数门槛"）。

### 决策 4：`sync` 时顺带同步可交易性数据

**选择**：在 `fund sync` 命令的 aggregator 流程中，把可交易性数据和赎回费率分档一并写入数据库，不新增独立的同步命令。

**理由**：保持命令界面简洁；可交易性数据的变化频率与净值数据相近，同步周期对齐合理。

**放弃的方案**：在 `fund inspect` 时实时抓取——inspect 强依赖网络，离线使用时该命令会失效。

### 决策 5：字段分级与降级策略

**选择**：

- A 级（可直接实现）：`SGZT/SHZT`、`redemptionFee/frontLoadFee`
- B 级（规则后实现）：`minInvestment`、`applyingMax*`
- C 级（无稳定来源）：`settlement_days`

**理由**：先保证核心买卖判断闭环，避免被低确定性字段阻塞。

**降级规则**：C 级字段允许 `NULL`，CLI 展示 `未知`，不中断交易。

## Risks / Trade-offs

- **[Risk]** Morningstar `purchaseAndRedeem.applyingMax*` 语义不透明，可能映射错误
  → **Mitigation**：先以“待确认限购信息”口径展示；在探针任务中完成字段语义标注后再启用硬约束

- **[Risk]** `settlement_days` 当前无稳定来源
  → **Mitigation**：定义为可空字段，统一显示为 `-` / `未知`，并在输出文案标注“以基金公司公告为准”

- **[Risk]** 首次买入日期策略在"卖出后重新买入"的场景下会高估持有时间（赎回费可能被低估）
  → **Mitigation**：在 `preview sell` 输出中注明"持有天数按首次买入估算，实际以基金公司为准"

- **[Risk]** 赎回费率分档数据来源可能不完整（某些基金 API 不返回费率）
  → **Mitigation**：缺失时默认显示"未知"并提示用户去基金平台确认，不阻断 sell 命令执行

- **[Trade-off]** `status` 命令行宽度增加（新增"持有天数"和"仓位占比"两列），在小终端可能换行
  → 接受，进入 MVP 阶段后可按反馈添加 `--compact` 标志

## Migration Plan

1. 数据库 migration：在 `db::setup` 中，使用 `CREATE TABLE IF NOT EXISTS` 新增 `fund_tradability` 和 `redemption_fee_tiers` 表；无需迁移现有数据
2. provider 扩展：`eastmoney_lsjz` 解析 `SGZT/SHZT`，`morningstar` 解析 `redemptionFee/frontLoadFee/deferLoadFee/purchaseAndRedeem` 并写入数据库
3. 命令扩展：`status`、`sell`、`preview sell`、`inspect` 命令读取新表数据时，若无数据则优雅降级
4. 无破坏性变更（BREAKING）：所有新增列/表均有默认值，不影响已有 CLI 接口
