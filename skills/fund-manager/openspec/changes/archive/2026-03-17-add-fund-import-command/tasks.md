## 1. 基础设施与依赖准备

- [x] 1.1 在 `Cargo.toml` 中添加 `csv` crate 依赖。
- [x] 1.2 在 `src/cli.rs` 中新增 `Import` 子命令，支持 `--file`, `--merge`, `--override` 参数，以及 `pairs` 变长参数。
- [x] 1.3 在 `src/db.rs` 中修改持仓统计查询（如 `get_holdings`, `get_fund_shares`），使 `type = 'import'` 的逻辑与 `'buy'` 一致。

## 2. 解析器 (Resolver) 增强

- [x] 2.1 扩展 `src/resolver.rs` 的 `resolve_fund` 函数，增加 `interactive: bool` 参数。
- [x] 2.2 当 `interactive` 为 `false` 且匹配到多个结果时，抛出明确的歧义错误。
- [x] 2.3 修复所有现有调用 `resolve_fund` 的地方，默认传入 `true`。

## 3. CSV 与批量解析逻辑

- [x] 3.1 编写 CSV 文件解析逻辑，提取（名称/代码，金额）对，并在解析失败时记录错误行号。
- [x] 3.2 编写变长参数对解析逻辑，验证参数对齐。
- [x] 3.3 构建批量处理的主循环（Pipeline），逐一对提取的数据进行名称搜索和合法性校验。

## 4. 冲突策略与写入执行

- [x] 4.1 在循环内实现系统状态校验：如果已存在持仓，根据策略选择是 `--merge` 追加、`--override` 重置（删除后插入），还是在无参数时直接报错跳过。
- [x] 4.2 根据当天是否公布净值，决定将 `import` 记录存储为 `settled` 或 `pending` 状态（复用之前的结算逻辑）。
- [x] 4.3 执行数据库写入。

## 5. 报告展示与命令串联

- [x] 5.1 在 `src/main.rs` 的分发逻辑中调用编写好的导入函数。
- [x] 5.2 导入结束后，使用 `comfy-table` 打印汇总表，包含成功数量和失败原因详情。
- [x] 5.3 运行 `cargo check` 和 `cargo test` 确保无代码退化，并进行手动测试验证 merge 和 override 行为。

