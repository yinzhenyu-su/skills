## Context

目前项目处于从零到一的阶段。用户需要一个高效、精确且易于扩展的命令行工具来管理基金资产。由于涉及财务数据，计算精度和数据一致性至关重要。

## Goals / Non-Goals

**Goals:**
- 实现基于 SQLite 的持久化存储，支持多钱包隔离。
- 提供自动化的基金数据同步（净值、费率）。
- 实现精确的财务计算（使用 `rust_decimal`）。
- 建立清晰的 CLI 交互规范。

**Non-Goals:**
- 不实现图形用户界面 (GUI)。
- 不实现多用户登录/同步功能（仅限本地单用户使用）。
- 不支持除了基金以外的其他资产类型（如股票、虚拟货币）。

## Decisions

### 1. 数据库选型：SQLite + `rusqlite`
- **Rationale**: CLI 工具不需要复杂的数据库服务器。SQLite 是单文件存储，便于迁移和备份。`rusqlite` 提供了简单且性能优异的 Rust 绑定。
- **Alternatives**: `sqlx` (异步支持更好但配置较复杂), `diesel` (ORM 较重，初期灵活性较低)。

### 2. 财务计算：`rust_decimal`
- **Rationale**: 浮点数 (`f64`) 在处理份额和金额时会产生精度误差，这在财务软件中是不可接受的。`rust_decimal` 提供定点小数运算，确保 `0.1 + 0.2 == 0.3`。
- **Alternatives**: `f64` (不安全), `bigdecimal` (较重)。

### 3. 数据抓取策略：基于代码的触发式更新
- **Rationale**: 每次操作（如买入、查看状态）时，检查本地数据是否为最新日期。如果不是，则自动发起 HTTP 请求同步。这能保证数据的实时性。
- **Alternatives**: 独立的 `sync` 命令（用户体验较差）, 后台常驻进程（过于复杂）。

### 4. 钱包切换机制：Context-based
- **Rationale**: 模仿 `git checkout`。在数据库中存储一个 `active_wallet_id`。所有的 `buy/sell/status` 命令默认作用于该 ID，减少用户输入负担。

## Risks / Trade-offs

- **[Risk] 网络环境不稳定** → **Mitigation**: 网络请求超时处理，并在请求失败时使用本地缓存的最后一次净值，同时给出警告。
- **[Risk] 外部 API 结构变更** → **Mitigation**: 将解析逻辑封装在独立的 `provider` 模块，方便在接口失效时快速替换。
- **[Risk] 数据库版本升级** → **Mitigation**: 引入简单的 Migration 机制，在 `init` 时检查版本并执行升级脚本。
