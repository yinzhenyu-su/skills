## Context

`status` 和 `list` 命令需要支持 `--wallet` 参数，并与 `buy`、`sell`、`import` 命令保持一致的钱包处理行为。现有的 `resolve_wallet_id()` 函数已实现了通用的钱包解析逻辑。

## Goals / Non-Goals

**Goals:**
- 给 `status` 和 `list` 命令添加 `--wallet` 参数
- 修改 `resolve_wallet_id()` 钱包解析逻辑：
  - **情况 A**：无指定钱包参数 + 无活跃钱包 + 没有任何钱包 → 自动创建"默认钱包"
  - **情况 B**：无指定钱包参数 + 无活跃钱包 + 有其他钱包 → 交互式询问用户选择
  - **情况 C**：有指定钱包参数 或 有活跃钱包 → 直接使用
- 使用 `inquire::Select` 实现交互式选择

**Non-Goals:**
- 不修改钱包的核心数据模型
- 不修改其他命令的钱包处理逻辑

## Decisions

### 修改 `resolve_wallet_id()` 函数行为

**选择**：扩展现有 `resolve_wallet_id()` 函数，添加交互式询问逻辑

**原因**：
- 保持与 `buy`、`sell`、`import` 命令调用方式一致
- 统一的钱包解析逻辑便于维护
- `inquire::Select` 交互模式与 resolver 中基金选择一致

**新增逻辑**：
```
无指定钱包参数
    │
    ├─ 有活跃钱包 → 直接返回
    │
    ├─ 无活跃钱包，有其他钱包 → 询问用户选择 ← 新增
    │
    └─ 没有任何钱包 → 自动创建"默认钱包"
```

**替代方案**：在各命令中内联实现询问逻辑
- 缺点：代码重复

### CLI 参数命名

**选择**：`--wallet`

**原因**：
- 与 `buy`、`sell`、`import` 命令保持一致
- 用户已熟悉该参数

## Risks / Trade-offs

| 风险 |  Mitigation |
|------|-------------|
| 无明显风险 | - |
