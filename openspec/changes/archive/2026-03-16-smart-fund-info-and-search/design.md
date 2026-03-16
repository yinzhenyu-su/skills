## Context

目前基金管理器主要依赖实时净值抓取。数据库 `fund` 表仅包含 `code` 和 `name`，导致用户必须手动输入名称，且缺乏风险等级、基金经理等决策参考信息。

## Goals / Non-Goals

**Goals:**
- 实现根据基金代码或名称自动补全完整元数据。
- 支持基于同花顺接口的模糊搜索（中文/拼音）。
- 引入本地缓存同步机制（`last_sync_at`），减少不必要的网络请求。
- 提供交互式 CLI 界面供用户在多个搜索结果中进行选择。

**Non-Goals:**
- 不实现复杂的本地全文本搜索引擎。
- 不处理非基金类（如股票、债券直投）的搜索建议。

## Decisions

### 1. 数据库 Schema 扩展
在 `fund` 表中新增以下字段，以持久化详情数据：
- `fund_type`: 基金类型（如：指数型-海外股票）
- `risk_level`: 风险等级（1-5）
- `manager`: 基金经理
- `company`: 基金公司
- `establish_date`: 成立日期
- `management_fee`, `trust_fee`, `sales_fee`: 费率信息
- `last_sync_at`: 最后同步时间戳

**Rationale**: 将元数据与基础表合并，简化查询逻辑，适合 CLI 工具的单机存储场景。

### 2. 增强型 Provider 架构
扩展 `Provider` trait 或新增专用模块：
- **`DetailProvider`**: 专门对接东方财富详情 API (`FundMNDetailInformation`)。
- **`SearchProvider`**: 对接同花顺搜索建议 API。

**Rationale**: 保持模块化，方便未来更换数据源。

### 3. 智能解析与拦截流 (Smart Resolver)
在执行业务逻辑（如 `Buy`）之前，引入一个 `resolve_fund` 逻辑：
1. **本地精准匹配**: 检查 `fund` 表中是否存在对应的 `code` 或 `name`。
2. **精确代码降级 (Fallback)**: 如果输入是 6 位纯数字且本地未命中，跳过模糊搜索，直接调用 `DetailProvider` 抓取详情。如果能抓到，则视为有效代码。
3. **远程模糊搜索**: 若输入不是 6 位代码，调用 `SearchProvider`。若结果为空则报错。
4. **交互选择**: 若搜索结果 > 1，使用 `inquire::Select` 弹出菜单。
5. **详情同步**: 确定代码后，检查 `last_sync_at`。若过期（30天）或本地无详情，调用 `DetailProvider` 补全并存入 DB。

### 4. 统一的 HTTP User-Agent
为防止 API 被防爬虫机制（如 Nginx 403 Forbidden）拦截，`Provider` 中使用的所有 `reqwest` 客户端应统一使用标准的浏览器 User-Agent，或者抽取一个公共的 Client 构建方法。

### 5. 依赖库引入
引入 `inquire` 库处理 CLI 交互。

## Risks / Trade-offs

- **[接口稳定性]**: 东方财富和同花顺的非公开 API 可能随时变动。
    - *Mitigation*: 在解析层使用鲁棒的正则匹配，并提供友好的错误提示。
- **[网络延迟]**: 自动补全可能增加命令执行时间。
    - *Mitigation*: 设置合理的超时（如 5s），并实现本地缓存 TTL 逻辑。
