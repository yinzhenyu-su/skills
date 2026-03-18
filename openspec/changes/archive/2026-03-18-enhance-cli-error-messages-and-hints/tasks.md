## 1. 结构化错误与建议引擎实现

- [x] 1.1 在 `resolver.rs` 中定义 `ResolveError` 枚举和 `ResolveResult` 类型。
- [x] 1.2 重构 `resolve_fund` 以返回结构化错误。
- [x] 1.3 实现 `AdviceEngine` 核心逻辑（参数反转检测函数）。
- [x] 1.4 在 `db.rs` 中添加支持 Levenshtein 或简单相似度匹配的查询接口。
- [x] 1.5 实现 `AdviceEngine` 的拼写建议函数。

## 2. 命令层集成 (Buy/Sell/Import)

- [x] 2.1 在 `handle_buy` 中捕获 `ResolveError` 并应用建议提示。
- [x] 2.2 在 `handle_sell` 中捕获 `ResolveError` 并应用建议提示。
- [x] 2.3 在 `handle_import` 的批量循环中集成建议引擎。
- [x] 2.4 在 `handle_sell` 中添加持有量预检逻辑（Sell Transaction Pre-check）。

## 3. UI/UX 增强与全局提示

- [x] 3.1 在 `handle_sync` 中添加无参数时的全量同步引导（Sync Without Arguments Guide）。
- [x] 3.2 实现全局错误包装器，在报错信息中注入当前活跃钱包上下文。
- [x] 3.3 统一提示信息的视觉风格（使用 Emoji 如 💡, ❓, ❌）。

## 4. 验证与测试

- [x] 4.1 编写单元测试验证参数反转检测算法。
- [x] 4.2 编写集成测试模拟 `buy 1000 520570` 场景。
- [x] 4.3 验证 `sell` 超额时的拦截提示。
- [x] 4.4 验证 `fund fund sync` 的引导输出。
