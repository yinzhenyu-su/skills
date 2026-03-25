好，我来把5类数据维度整理成一份 PRD 字段清单，按"买入判断"和"卖出判断"分别标注每个字段的必要性。

---

## 基金买入/卖出判断 — 产品数据字段清单

---

### 一、交易可执行性数据

| 字段名 | 数据示例 | 买入 | 卖出 | 当前系统状态 |
|--------|---------|:----:|:----:|:----------:|
| `subscription_status` | 开放/暂停/限大额/仅定投 | 必须 | - | ❌ 缺失 |
| `redemption_status` | 开放/暂停/限额 | - | 必须 | ❌ 缺失 |
| `subscription_limit_single` | 单笔限购上限，如 100,000 元 | 必须 | - | ❌ 缺失 |
| `subscription_limit_daily` | 单日限购上限 | 必须 | - | ❌ 缺失 |
| `min_subscription_amount` | 最低买入额，如 100 元 | 必须 | - | ❌ 缺失 |
| `min_additional_amount` | 最低追加额 | 推荐 | - | ❌ 缺失 |
| `min_redemption_shares` | 最低赎回份额 | - | 必须 | ❌ 缺失 |
| `available_shares` | 可用份额（含冻结情况） | - | 必须 | ✅ 已有 |
| `pending_shares` | 待确认（在途）份额 | 推荐 | 必须 | ✅ 已有 |
| `locked_shares` | 冻结份额（定期持有期内） | - | 必须 | ❌ 缺失 |
| `cutoff_time` | 当日申请截止时间，如 15:00 | 推荐 | 推荐 | ❌ 缺失 |
| `nav_effective_date_rule` | T日/T+1日净值生效规则 | 推荐 | - | ❌ 缺失 |
| `settlement_days` | 资金到账时间，如 T+2 | 推荐 | 必须 | ❌ 缺失 |

---

### 二、价格与成本数据

| 字段名 | 数据示例 | 买入 | 卖出 | 当前系统状态 |
|--------|---------|:----:|:----:|:----------:|
| `latest_nav` | 单位净值，如 2.3456 | 必须 | 必须 | ✅ 已有 |
| `nav_date` | 净值日期，如 2026-03-24 | 必须 | 必须 | ✅ 已有 |
| `estimated_nav` | 盘中估算净值 | 推荐 | 推荐 | ❌ 缺失 |
| `acc_nav` | 累计净值 | 推荐 | 推荐 | ✅ 已有 |
| `subscription_fee_rate` | 申购费率，阶梯式 | 必须 | - | ✅ 已有（单档） |
| `subscription_fee_tiers` | 阶梯费率表（按金额分档） | 推荐 | - | ❌ 缺失 |
| `redemption_fee_rate` | 赎回费率（按持有天数分档） | - | 必须 | ❌ 缺失 |
| `redemption_fee_tiers` | 赎回费率阶梯表 | - | 必须 | ❌ 缺失 |
| `mgmt_fee` | 管理费率（年化），如 1.5% | 参考 | - | ✅ 已有 |
| `trust_fee` | 托管费率（年化） | 参考 | - | ✅ 已有 |
| `estimated_input_amount` | 买入预估投入总金额 | 必须 | - | ✅ 已有（preview） |
| `estimated_shares` | 买入预估获得份额 | 必须 | - | ✅ 已有（preview） |
| `estimated_subscription_fee` | 买入预估手续费 | 必须 | - | ✅ 已有（preview） |
| `estimated_redemption_fee` | 卖出预估手续费 | - | 必须 | ✅ 已有（preview） |
| `estimated_proceeds` | 卖出预估到账金额 | - | 必须 | ✅ 已有（preview） |

---

### 三、基金质量与风险数据

| 字段名 | 数据示例 | 买入 | 卖出 | 当前系统状态 |
|--------|---------|:----:|:----:|:----------:|
| `fund_type` | 偏股混合、纯债、QDII | 必须 | 参考 | ✅ 已有 |
| `risk_level` | R1~R5 | 必须 | - | ✅ 已有 |
| `fund_size` | 规模，如 12.3 亿 | 推荐 | - | ❌ 缺失 |
| `establish_date` | 成立日期 | 推荐 | - | ✅ 已有 |
| `manager_name` | 基金经理姓名 | 推荐 | - | ✅ 已有 |
| `manager_tenure` | 现任经理任职年限 | 推荐 | - | ❌ 缺失 |
| `company_name` | 基金公司 | 参考 | - | ✅ 已有 |
| `morningstar_rating_3y` | 晨星3年评级 1~5 星 | 推荐 | - | ✅ 已有 |
| `morningstar_rating_5y` | 晨星5年评级 | 推荐 | - | ✅ 已有 |
| `rank_pct_3y` | 同类3年排名百分位（越低越好）| 推荐 | 推荐 | ✅ 已有 |
| `return_1y` | 近1年收益率 | 推荐 | 推荐 | ❌ 缺失 |
| `return_3y` | 近3年收益率 | 推荐 | 推荐 | ❌ 缺失 |
| `sharpe_3y` | 夏普比率 3年 | 推荐 | - | ✅ 已有 |
| `calmar_3y` | 卡玛比率 3年 | 推荐 | - | ✅ 已有 |
| `max_drawdown_3y` | 最大回撤 3年 | 推荐 | 推荐 | ✅ 已有 |
| `investor_gap_3y` | 基民获得感（收益差） | 推荐 | 推荐 | ✅ 已有 |

---

### 四、组合适配数据（持仓层）

| 字段名 | 数据示例 | 买入 | 卖出 | 当前系统状态 |
|--------|---------|:----:|:----:|:----------:|
| `current_shares` | 当前持有份额 | 推荐 | 必须 | ✅ 已有 |
| `avg_cost_per_share` | 平均持仓成本 | 推荐 | 推荐 | ✅ 已有 |
| `total_cost` | 总成本（净投入） | 推荐 | 推荐 | ✅ 已有 |
| `current_value` | 当前总市值 | 推荐 | 推荐 | ✅ 已有 |
| `unrealized_pnl` | 浮动盈亏金额 | 推荐 | 必须 | ✅ 已有 |
| `unrealized_pnl_pct` | 浮动盈亏百分比 | 推荐 | 必须 | ✅ 已有 |
| `holding_days` | 持有天数 | - | 必须 | ❌ 缺失（需计算） |
| `first_buy_date` | 首次买入日期 | - | 推荐 | ✅ 可从 history 推算 |
| `wallet_allocation_pct` | 该基金在钱包中的资产占比 | 推荐 | 推荐 | ❌ 缺失 |
| `total_dividend_received` | 累计分红收入 | 参考 | 参考 | ✅ 已有（dividend 记录） |

---

### 五、市场环境与时机数据

| 字段名 | 数据示例 | 买入 | 卖出 | 当前系统状态 |
|--------|---------|:----:|:----:|:----------:|
| `benchmark_index` | 关联基准指数名称 | 推荐 | 推荐 | ❌ 缺失 |
| `benchmark_index_change_1d` | 基准指数今日涨跌 | 推荐 | - | ✅ 部分（index 命令） |
| `nav_change_1d` | 净值日涨跌幅 | 推荐 | - | ❌ 缺失 |
| `nav_change_1m` | 近1月净值涨跌幅 | 参考 | 参考 | ❌ 缺失 |
| `pe_percentile` | 指数估值历史分位（适用宽基）| 推荐 | 推荐 | ❌ 缺失 |
| `recent_announcements` | 近期公告（限购/分红/调整）| 推荐 | 推荐 | ❌ 缺失 |

---

**字段优先级汇总（覆盖情况）**

```text
✅ 已覆盖    约 25 个字段
❌ 待补充    约 20 个字段

关键缺口（影响最基本判断的）：
  - subscription_status     ← 能不能买
  - redemption_status       ← 能不能卖
  - redemption_fee_tiers    ← 卖出成本核心
  - available_shares (确认冻结) ← 卖出前置条件
  - holding_days            ← 影响赎回费档位
  - settlement_days         ← 用户最关心的到账时间
```

如果要做"赎回费分档"这个能力，我们有一个比较自然的 OpenSpec 提案方向：**fund-sell-cost-tiers**。你要我现在起草成提案，还是继续往别的方向探？

<task_complete>
<summary>Organized the 5 categories of data dimensions for fund buy/sell decision-making into a structured PRD field checklist, covering 45+ fields across: (1) trade executability, (2) price & cost, (3) fund quality & risk, (4) portfolio fit, and (5) market timing. Each field is marked with buy/sell relevance and current system coverage status, highlighting ~20 critical gaps including subscription_status, redemption_fee_tiers, holding_days, and settlement_days.</summary>
</task_complete>已整理完成。
