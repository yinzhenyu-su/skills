# 提案：统一 CLI 错误信息格式并消除 panic

## 问题

当前 fund-manager CLI 存在三个层面的问题：

### 1. 两种错误风格并存

**已优化（中文，友好）：**
```
$ fund-manager preview buy
❌ 缺少参数 <FUND>：请提供基金代码或名称
   当前追踪的基金：
   - 德邦德利货币A (000300)
   用法示例：fund-manager preview buy 000300 --money 5000
```

**未优化（英文，clap 自动生成）：**
```
$ fund-manager history
error: the following required arguments were not provided:
  <FUND>

Usage: fund-manager history <FUND>

For more information, try '--help'.
```

### 2. 交互式选择预期与实际体验不符

目前某些命令注释了“省略则交互式选择”，但这种做法对于 CLI 工具存在缺点：会阻断自动化脚本，并且增加用户的心智负担。相反，“预测意图并直接给出有用的上下文提示”往往体验更好。

### 3. panic 而非优雅退出

`sell` 命令中的多处 `.expect()` 会在运行时产生 Rust panic 堆栈：
```
thread 'main' panicked at src/main.rs:1632:26:
未找到该基金的净值数据。请使用 --nav 手动指定。
```

### 4. 错误格式不一致

- 前缀：`"❌ 错误："` vs `"❌ {}"` vs `"❌ 缺少参数："`
- 引号：`'{}'` vs `「{}」`
- 括号注释：`"❌ 错误（执行过程失败）："` 有时有有时没有
- 帮助文本：有些错误有后续提示和用法示例，有些没有

## 方案

1. **废弃交互式选择模式** — 将“省略则交互式选择”改为“省略则输出友好报错并退出”，不引入 `dialoguer` 等阻断性交互输入工具。
2. **将 clap 必填参数改为 `Option`** — `history`、`sell`、`fund delete`、`fund inspect`、`buy` 的 fund 参数和 `buy` 的 `--money` 改为可选，在 handler 中统一验证，提供智能的上下文（如有持仓基金列表）和具体命令示例。
3. **消灭所有 panic** — `.expect()`/`.unwrap()` 替换为 `eprintln!` + `exit(1)`
4. **统一错误格式** — 所有错误走自定义路径，格式为 `❌ 问题描述` → `   可用选项` → `   用法示例`

## 影响范围

- `src/cli.rs` — 参数定义改动
- `src/main.rs` — handler 中错误处理改动
- `tests/cli_tests.rs` — 可能需要更新测试断言

## 成功标准

- `cargo test` 全部通过
- 所有命令在缺少参数时输出中文友好错误信息，提供上下文选项
- 无 `.expect()` panic（测试代码除外）
