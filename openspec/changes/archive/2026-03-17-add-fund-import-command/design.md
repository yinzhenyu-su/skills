## Context

目前 fund-manager 依赖 `transaction_log` 驱动持仓统计。引入 `import` 命令需要处理不同的对账场景：是简单的资金追加（Merge）还是彻底的资产重置（Override）。批量导入还需要处理名称歧义、数据合法性校验，并能够反馈详细的执行结果。

## Goals / Non-Goals

**Goals:**
- 提供 `import --file <PATH>` 和 `import <NAME> <MONEY>...` 两种输入方式。
- 实现 `--merge` 和 `--override` 两种冲突处理逻辑，且默认必须有明确的行为或在使用前报错保护。
- 批量处理时支持“非交互模式”，将歧义项记录到错误报告，而不是弹窗打断用户。

**Non-Goals:**
- 不支持历史交易流水的批量导入（本阶段仅支持当前持仓市值导入）。
- 不提供 CSV 模板导出功能。

## Decisions

### 1. 交易记录标记
- **选择**: 在 `transaction_log` 的 `type` 字段存储 `"import"`。
- **理由**: 语义更明确，方便后续统计区分自然买入和手动对账。查询 `get_holdings` 等接口需兼容 `'import'`，将其视为买入同等行为。

### 2. 模式冲突处理
- **Merge**: 默认行为，将导入金额作为新记录追加，效果类似于 `buy`。
- **Override**: 执行 `DELETE FROM transaction_log WHERE fund_code = ? AND wallet_id = ?`，然后插入这笔 `import` 记录。
- **理由**: 为保持导入过程符合用户的直觉，允许默认 `merge` 追加是最平滑的。如果用户想要推倒重来对账，必须显式附加 `--override`。

### 3. CSV 解析策略
- **选择**: 引入 `csv` crate。
- **理由**: 手动解析处理引号、逗号转义非常繁琐，使用标准库更稳健。

### 4. Resolver 交互降级
- **选择**: 修改 `resolve_fund` 签名，增加 `interactive: bool` 参数。
- **理由**: 在批量导入时（特别是文件导入），如果遇到一个名称对应多个基金，弹窗会让流程挂起。非交互模式下应直接返回 Error，计入最终的失败报告中。

## Risks / Trade-offs

- **[风险] 接口请求过快** → **[缓解]** 在批量循环中加入 200ms 的短暂休眠（Sleep）。
- **[风险] 成本计算误导** → **[缓解]** 在 `--override` 模式成功后，提示中明确告知用户“盈亏将从今日起算，历史盈亏已被重置”。
- **[风险] 现有统计逻辑失效** → **[缓解]** 确保所有计算份额、金额的 SQL 查询同时包含 `type = 'buy'` 和 `type = 'import'`。
