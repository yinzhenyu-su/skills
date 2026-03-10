## Context

目前系统已经实现了买入功能，但缺乏卖出功能和数据清理机制。`sell` 命令需要能够准确处理持仓份额的减少，而 `delete` 功能需要确保彻底移除所有关联数据。

## Goals / Non-Goals

**Goals:**
- 实现 `sell` 命令，支持按份额卖出。
- 实现 `sell --auto` 命令，根据金额、当前净值及（可能的）赎回费率计算卖出份额。
- 实现 `fund delete <code>` 命令，彻底从数据库中移除该基金的所有足迹。
- 确保卖出后的总份额非负。

**Non-Goals:**
- 不实现部分卖出时的复杂税务计算（如 FIFO/LIFO），仅记录流水并计算实时总份额。
- `fund delete` 不支持批量删除（仅限单代码删除）。

## Decisions

### 1. 卖出份额校验
- **Decision**: 在执行 `sell` 操作前，系统必须查询当前活跃钱包下该基金的总份额。如果卖出份额大于当前持仓，则报错。
- **Rationale**: 防止产生“负持仓”，保证数据逻辑合理。

### 2. 赎回金额计算 (`sell --auto`)
- **Decision**: 类似于买入，卖出也可以通过输入金额自动反算份额。公式：`份额 = 卖出金额 / (单位净值 * (1 - 赎回费率))`。
- **Rationale**: 方便用户根据想“变现”的金额来操作。

### 3. 数据删除策略：级联删除 vs 手动清理
- **Decision**: 在 SQLite 中使用 `PRAGMA foreign_keys = ON` 并配置 `ON DELETE CASCADE`。
- **Rationale**: 让数据库自动处理 `fund` 被删除时 `transaction_log` 和 `nav_history` 的清理，减少 Rust 层的代码复杂度并保证一致性。

## Risks / Trade-offs

- **[Risk] 误删数据** → **Mitigation**: 在执行 `fund delete` 前增加交互式确认提示。
- **[Risk] 赎回费率变动** → **Mitigation**: 赎回费率通常随持有时间变化（阶梯费率）。初期版本支持用户手动输入费率，或默认使用 0。
