## Context

在上一轮汉化中，我们处理了 90% 的用户界面字符串。本设计旨在覆盖剩下的 10% “深层”字符串，这些字符串通常出现在非正常路径（错误处理）或复杂的业务解释逻辑中。

## Goals / Non-Goals

**Goals:**
- 实现 100% 的中文覆盖率。
- 重点汉化 CSV 导入的报错细节。
- 重点汉化钱包切换相关的引导信息。

**Non-Goals:**
- 不涉及核心业务逻辑的修改。
- 不修改底层的 `expect` 消息（除非它们直接向用户展示）。

## Decisions

### 1. CSV 导入报错映射
- `CSV Line {}` -> `CSV 第 {} 行`
- `Amount must be positive` -> `金额必须为正数`
- `Invalid money format` -> `无效的金额格式`
- `Failed to open CSV` -> `无法打开 CSV 文件`

### 2. 钱包引导提示
- `Wallet '{}' not found.` -> `未找到名为 '{}' 的钱包。`
- `No active wallet selected. Use 'fund wallet use <name>' or specify --wallet.` -> `未选择活跃钱包。请使用 'fund wallet use <名称>' 或指定 --wallet 参数。`

### 3. 深度分析解释
- `and investors often lose money due to bad timing (buying high, selling low).` -> `且投资者常因追涨杀跌（择时错误）导致亏损。`

### 4. 同步状态
- `Sync completed.` -> `同步已完成。`
- `Could not fetch latest data` -> `无法获取最新数据`

## Risks / Trade-offs

- **[Risk] 测试断言失效**：大量字符串修改会导致现有的集成测试失效。
  - **Mitigation**：必须在 `tasks.md` 中包含“更新测试断言”的步骤，并确保所有测试通过。
