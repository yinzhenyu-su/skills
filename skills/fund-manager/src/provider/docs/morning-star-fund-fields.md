# 获取晨星基金的 performance 数据解析

## 获取晨星基金的 performance 数据

请求参数：

```bash
curl '<https://www.morningstar.cn/cn-api/v2/funds/000513/performance>' \
  -H 'accept: application/json, text/plain, */*' \
  -H 'accept-language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7' \
  -H 'referer: <https://www.morningstar.cn/>' \
  -H 'user-agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36'
```

本文档详细解析晨星（Morning Star）基金数据接口返回的 JSON 字段含义。

## 基金基础信息

| 字段 | 类型 | 说明 |
|------|------|------|
| `csdcc` | string | 基金代码（如 "000513"） |
| `secId` | string | 晨星证券ID（如 "F00000TGW4"） |
| `categoryId` | string | 基金分类ID（如 "CHCA000051"） |
| `categoryName` | string | 基金分类名称（如 "大盘成长股票"） |
| `benchmarkId` | string | 业绩比较基准ID |
| `benchmarkName` | string | 业绩比较基准名称 |

---

## 收益数据（Returns）

### 收益率周期说明

| 缩写 | 含义 |
|------|------|
| `D1` | 日收益率（1 Day） |
| `W1` | 周收益率（1 Week） |
| `M1` | 月收益率（1 Month） |
| `M3` | 3个月收益率 |
| `M6` | 6个月收益率 |
| `YTD` | 年初至今收益率（Year to Date） |
| `Y1` | 一年收益率 |
| `Y2` | 两年收益率 |
| `Y3` | 三年收益率 |
| `Y5` | 五年收益率 |
| `Y7` | 七年收益率 |
| `Y10` | 十年收益率 |

### 收益率数据结构

```json
{
  "returns": { ... },           // 基金自身收益率
  "categoryReturns": { ... },   // 同类基金平均收益率
  "benchmarkReturns": { ... },  // 业绩比较基准收益率
  "returnRanks": { ... },       // 收益率排名（百分比排名）
  "investmentsInCategory": { ... } // 同类基金数量
}
```

#### returnRanks（排名）说明

表示基金在同类基金中的排名百分位，数值越小表示排名越靠前。例如：

- 排名 8 表示该基金在同类基金中位于前 8%
- 排名 88 表示该基金位于前 88%（即后 12%）

---

## 评级数据（Rating）

| 字段 | 类型 | 说明 |
|------|------|------|
| `ratingDate` | string | 评级日期（格式：YYYY-MM） |
| `Y3` | string | 三年期综合评级（1-5星） |
| `Y5` | string | 五年期综合评级（1-5星） |
| `Y10` | string | 十年期综合评级（1-5星） |
| `Y3History` | array | 评级历史记录 |

### 评级历史记录格式

```json
{
  "k": "2017-06",   // 评级时间
  "v": "1"          // 星级（1-5）
}
```

---

## 风险指标（Risk）

风险数据按不同时间周期组织（Y1/Y3/Y5/Y10），每个周期包含以下结构：

```json
{
  "risk": { ... },           // 基金风险指标
  "categoryRisk": { ... },   // 同类基金平均风险指标
  "benchmarkRisk": { ... }, // 基准风险指标
  "benchmarkNames": [...]   // 基准名称列表
}
```

### 收益指标

| 字段 | 说明 | 计算方式 |
|------|------|----------|
| `return` | 收益率 | 区间年化收益率 |
| `returnRankOver` | 收益率排名百分位 | 在同类基金中的排名位置 |

### 波动率指标

| 字段 | 说明 | 解读 |
|------|------|------|
| `stdDev` | 标准差 | 衡量收益的波动程度，越高风险越大 |
| `stdDevRankOver` | 标准差排名百分位 | 数值越高表示波动越大 |
| `downsideDeviation` | 下行偏差 | 只考虑负收益的波动，更关注下行风险 |
| `downsideDeviationRankOver` | 下行偏差排名百分位 | - |
| `morningstarRisk` | 晨星风险系数 | 晨星特有的风险指标，越高风险越大 |
| `morningstarRiskRankOver` | 晨星风险排名百分位 | - |
| `maxDrawdown` | 最大回撤 | 从最高点到最低点的最大跌幅 |
| `maxDrawdownRankOver` | 最大回撤排名百分位 | 数值越高表示回撤越大 |

### 风险调整收益指标

| 字段 | 说明 | 解读 |
|------|------|------|
| `sharpeRatio` | 夏普比率 | (收益率-无风险利率)/标准差，越高越好 |
| `sharpeRatioRankOver` | 夏普比率排名百分位 | 数值越高表示风险调整收益越好 |
| `sortinoRatio` | 索提诺比率 | (收益率-目标收益)/下行偏差，越高越好 |
| `sortinoRatioRankOver` | 索提诺比率排名百分位 | - |
| `calmarRatio` | 卡玛比率 | 年化收益率/最大回撤，越高越好 |
| `calmarRatioRankOver` | 卡玛比率排名百分位 | - |

### Alpha 与 Beta

| 字段 | 说明 | 解读 |
|------|------|------|
| `alphaWI` | 相对万得全指的超额收益 | >0 表示跑赢基准 |
| `betaWI` | 相对万得全指的波动系数 | >1 表示波动大于基准 |
| `rSquaredWI` | R²（决定系数） | 表示基金波动能被基准解释的比例 |
| `alphaCAI` | 相对指数的超额收益 | 针对特定基准 |
| `betaCAI` | 相对指数的波动系数 | - |
| `rSquaredCAI` | R² | - |
| `alphaCA` | 相对同类平均的超额收益 | >0 表示跑赢同类平均 |
| `betaCA` | 相对同类平均的波动系数 | - |
| `rSquaredCA` | R² | - |
| `alphaPB` | 相对业绩比较基准的超额收益 | - |
| `betaPB` | 相对业绩比较基准的波动系数 | - |
| `rSquaredPB` | R² | - |

### 捕获比率

| 字段 | 说明 | 解读 |
|------|------|------|
| `upsideCaptureRatioWI` | 上行捕获比率（万得全指） | >100% 表示在上涨时跑赢基准 |
| `downsideCaptureRatioWI` | 下行捕获比率（万得全指） | <100% 表示在下跌时更抗跌 |
| `upsideCaptureRatioCAI` | 上行捕获比率（指数） | - |
| `downsideCaptureRatioCAI` | 下行捕获比率（指数） | - |
| `upsideCaptureRatioCA` | 上行捕获比率（同类平均） | - |
| `downsideCaptureRatioCA` | 下行捕获比率（同类平均） | - |
| `upsideCaptureRatioPB` | 上行捕获比率（业绩基准） | - |
| `downsideCaptureRatioPB` | 下行捕获比率（业绩基准） | - |

### 其他风险指标

| 字段 | 说明 |
|------|------|
| `battingAverageWI` | 胜率（万得全指）- 基金跑赢基准的月份比例 |
| `battingAverageCAI` | 胜率（指数） |
| `battingAverageCA` | 胜率（同类平均） |
| `battingAveragePB` | 胜率（业绩基准） |
| `excessPB` | 相对业绩比较基准的超额收益 |
| `trackPB` | 相对业绩比较基准的跟踪误差 |
| `infoPB` | 信息比率 |
| `excessCA` | 相对同类平均的超额收益 |
| `trackCA` | 相对同类平均的跟踪误差 |
| `infoCA` | 信息比率 |
| `excessCAI` | 相对指数的超额收益 |
| `trackCAI` | 相对指数的跟踪误差 |
| `infoCAI` | 信息比率 |
| `excessWI` | 相对万得全指的超额收益 |
| `trackWI` | 相对万得全指的跟踪误差 |
| `infoWI` | 信息比率 |

### 基准名称说明

```json
[
  { "k": "WI", "v": "沪深300全收益" },
  { "k": "CAI", "v": "沪深300相对全收益成长指数" },
  { "k": "CA", "v": "同类平均" },
  { "k": "PB", "v": "业绩比较基准" }
]
```

---

## 投资者回报率（Investor Return）

| 字段 | 说明 |
|------|------|
| `returnDate` | 统计截止日期 |
| `investorReturn` | 投资者实际回报率（考虑资金进出时机） |
| `return` | 基金净值收益率（时间加权收益率） |

### 投资者回报与基金回报的区别

- **基金回报（return）**：基金净值的变化率，不考虑投资者买入卖出时机
- **投资者回报（investorReturn）**：考虑投资者实际资金进出时机的真实收益，通常低于基金回报

### 资金流向（Cash Flows）

```json
{
  "k": "2016-03-31",  // 季度末日期
  "v": 1.5152718068E8 // 净资金流入（正=净流入，负=净流出）
}
```

单位：元（人民币）

### 季度投资者回报

```json
{
  "k": "2016-03-31",  // 季度末日期
  "v": -22.27051      // 该季度投资者回报率(%)
}
```

---

## 数据时间周期说明

| 类型 | 数据频率 |
|------|----------|
| `dayEnd` | 日频（包含日、周、月、YTD、年等周期） |
| `monthEnd` | 月频（包含月、季、半年、年等周期） |
| `quarterly` | 季频（自2016年起） |
| `annual` | 年频（包含历年年度数据） |

---

## 字段命名规律总结

1. **无后缀**：基金自身的指标
2. **Category/CA 后缀**：同类基金平均值
3. **Benchmark/WI/CAI/PB 后缀**：对应基准的指标
4. **RankOver 后缀**：在同类基金中的排名百分位

---

## 参考资料

- [晨星中国官网](https://cn.morningstar.com/)
- 晨星风险指标计算方法基于全球通用的基金评价标准
