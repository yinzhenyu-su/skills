## Context

当前 `fund-manager` 的钱包管理存在以下问题：

1. **外键约束失效**：`db::init_db()` 中设置了 `PRAGMA foreign_keys = ON`，但 `main.rs` 随后又用 `Connection::open()` 打开了一个新连接，该连接的 `PRAGMA foreign_keys` 默认为 `OFF`。导致 `wallet` 表的 `ON DELETE CASCADE` 约束未生效。

2. **钱包不存在时体验差**：调用 `resolve_wallet_id()` 时，如果既没有指定 `--wallet` 参数，也没有活跃钱包，程序直接 `expect()` 崩溃。

3. **缺少重命名**：CLI 没有提供钱包重命名功能。

## Goals / Non-Goals

**Goals:**
- 修复外键约束在运行时未生效的问题
- 实现无钱包时的自动创建逻辑
- 添加钱包重命名命令

**Non-Goals:**
- 不改变数据库 schema
- 不修改钱包的其他属性（只改 name）

## Decisions

### Decision 1: 统一在 `Connection::open()` 后设置 `PRAGMA foreign_keys = ON`

**Option A**: 在 `db.rs` 创建 `open_db()` 封装函数，统一处理
- **Pros**: 所有打开数据库的地方自动获得外键约束
- **Cons**: 需要修改 `init_db` 和所有 `Connection::open` 调用点

**Option B**: 在 `main.rs` 打开连接后执行 `PRAGMA`
- **Pros**: 改动最小
- **Cons**: 如果其他地方也打开连接，容易遗漏

**Selected**: Option A - 创建 `open_conn(path: P) -> Connection` 函数，确保所有连接统一设置。

### Decision 2: 自动创建钱包的触发时机

**Option A**: 在 `resolve_wallet_id()` 中自动创建
- **Pros**: 所有需要钱包的命令自动受益
- **Cons**: 用户可能不知道钱包的概念

**Option B**: 只在首次运行时创建
- **Pros**: 更可控
- **Cons**: 实现更复杂

**Selected**: Option A - 在 `resolve_wallet_id()` 中检查并自动创建。

### Decision 3: 重命名的实现方式

**Option A**: 添加 `rename_wallet(conn, old_name, new_name)` 函数
- **Pros**: 与现有 `add_wallet`, `delete_wallet` 风格一致
- **Cons**: 无

**Selected**: Option A。

## Risks / Trade-offs

- [Risk] 自动创建钱包可能导致用户意外创建多个钱包 → [Mitigation] 只在没有活跃钱包时才创建，有活跃钱包但用户想用其他名时仍需手动指定 `--wallet`
- [Risk] 重命名可能与已有钱包名冲突 → [Mitigation] 在 `rename_wallet` 中检查 `new_name` 是否已存在，返回错误

## Open Questions

- 无
