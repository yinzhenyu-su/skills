## Context

目前 `fund inspect` 等命令依赖 `resolve_fund` 函数进行基金定位。当前的 `resolve_fund` 逻辑对名称匹配非常严格，且 `inspect` 调用时禁用了交互模式。对于本地库中已有的基金，由于不支持 `LIKE` 搜索，用户必须输入完整名称或 6 位代码才能定位。

## Goals / Non-Goals

**Goals:**
- 实现本地数据库的模糊搜索逻辑。
- 优化 `resolve_fund` 算法，使其在多级查找中更具鲁棒性。
- 提升 `inspect` 命令在处理歧义时的交互体验。
- 统一各子命令（`sync`, `delete` 等）的基金解析入口。

**Non-Goals:**
- 不改变基金数据的存储结构。
- 不引入新的远程数据提供者。

## Decisions

### 1. 扩展 `resolve_fund` 查找链路
解析算法将调整为以下顺序：
1. **精确匹配**: 调用 `db::get_fund_by_code_or_name`。
2. **代码直查**: 若输入符合 6 位数字格式，调用 `sync_fund_details` 远程同步。
3. **本地模糊匹配 (新增)**: 调用 `db::search_funds_locally`（使用 `LIKE` 语法）。
4. **远程模糊搜索**: 调用远程搜索接口。

**Rationale**: 优先尝试本地数据（包括模糊匹配）可以显著减少网络请求，提升响应速度。

### 2. 交互模式处理
- 在 `handle_inspect` 中启用 `interactive: true`。
- `resolve_fund` 在步骤 3 和 4 若发现多个结果：
  - 若 `interactive=true`: 使用 `inquire::Select` 弹出选择菜单。
  - 若 `interactive=false`: 将所有匹配结果格式化为字符串并作为错误消息返回，告知用户存在歧义。

### 3. `delete` 命令的特殊处理 (`local_only`)
- 修改 `resolve_fund` 或添加变体，支持 `local_only` 标志。
- 对于 `delete` 命令，只允许执行步骤 1 和 3（本地精确和本地模糊）。
- **Rationale**: 用户不可能删除一个本地甚至都不存在的基金，因此无需发起远程搜索。

## Risks / Trade-offs

- **[Risk] 名称重叠冲突** → **Mitigation**: 优先显示匹配结果的基金代码，帮助用户区分同一基金的不同份额（如 A/C 类）。
- **[Trade-off] 本地数据陈旧** → **Mitigation**: 命中本地模糊匹配后，仍会经过 `maybe_sync_fund` 逻辑，确保数据时效性。
