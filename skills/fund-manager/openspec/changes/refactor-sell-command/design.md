## Context

目前 `sell` 命令的实现较为单一，仅支持具体的数值输入且缺乏交互引导。用户在赎回时需要频繁计算比例或查看余额，导致体验割裂。

## Goals / Non-Goals

**Goals:**
- 提供多维度的卖出输入方式（数值、分数、全量）。
- 支持灵活的手续费录入（定额或比例）。
- 建立“预览-确认”交互流，防止误操作。
- 确保所有推导结果的精度符合财务规范。

**Non-Goals:**
- 不支持跨日期的补录赎回（当前仅支持按最新或指定单价实时结算）。
- 不实现复杂的阶梯赎回费率自动查询（用户需手动输入）。

## Decisions

### 1. 份额解析策略 (Fractional & Keyword Parsing)
- **Decision**: 使用自定义解析器处理 `shares` 字符串。
- **Logic**:
    - 如果是 `all` -> 查库获取 `total_shares`。
    - 如果包含 `/` -> 拆分 `numerator` / `denominator`，乘以 `total_shares`。
    - 否则 -> 尝试 `Decimal::from_str`。
- **Rationale**: 允许用户直接表达“卖出一半”这种直觉想法。

### 2. 费率与金额混合解析 (Fee Parsing)
- **Decision**: 检查 `fee` 字符串结尾。
- **Logic**:
    - 如果以 `%` 结尾 -> `fee = (shares * nav) * rate / 100`。
    - 否则 -> 视为固定金额。
- **Rationale**: 模拟真实赎回场景中常见的百分比扣费模式。

### 3. 交互式确认实现
- **Decision**: 在执行 DB 写入前，阻塞线程并等待 `stdin` 输入。
- **Rationale**: 对于资产减损类操作，显式的二次确认是 CLI 工具的安全底线。

## Risks / Trade-offs

- **[Risk] 参数混淆** → **Mitigation**: 在 `clap` 定义中使用互斥或清晰的帮助文案说明 `--shares`, `--money` 的优先级（通常 `shares` 优先）。
- **[Risk] 浮点运算误差** → **Mitigation**: 所有中间计算均使用 `rust_decimal`，禁止直接使用 `f64`。
