# 设计：统一 CLI 错误信息格式

## 错误信息规范

### 格式模板

```
❌ <问题描述>
   <可用选项或提示信息>
   用法示例：<具体命令>
```

### 规则

1. **前缀**：统一使用 `❌ `，不加"错误："字样（问题描述本身已说明问题）
2. **引号**：统一使用单引号 `'{}'`
3. **缩进**：后续行统一 3 空格缩进 `   `
4. **无括号注释**：不使用 `"（执行过程失败）"` 等括号注释
5. **用法示例**：每个错误尽量给出用法示例

### 各场景错误信息

**缺少 fund 参数：**
```
❌ 缺少基金参数：请提供基金代码或名称
   当前追踪的基金：
   - 基金A (000001)
   - 基金B (000002)
   用法示例：fund-manager history 000001
```

**缺少 --money/--shares 参数：**
```
❌ 缺少参数：请提供 --money 或 --shares 之一
   --money <金额>   按投入金额买入，例如：--money 5000
   --shares <份额>  按指定份额买入，例如：--shares 4538.65
```

**无效的数值格式：**
```
❌ 无效的净值格式 'abc'：请输入有效数字
   用法示例：fund-manager buy 000300 --money 5000 --nav 1.25
```

**净值数据不可用：**
```
❌ 未找到基金 '000300' 的净值数据
   请使用 --nav 手动指定净值
   用法示例：fund-manager sell 000300 --shares 500 --nav 1.25
```

**数据库操作失败：**
```
❌ 数据库操作失败：<具体原因>
```

**基金不存在于钱包：**
```
❌ 基金 '000300' 在当前钱包中不存在或无持仓
   请先用 buy 命令买入该基金
```

## clap 参数与交互式选择改动

**核心决策：全面废除 "参数缺失时进入交互模式"（如 `dialoguer` 等方案）的想法，统一采用 "友好报错 + 上下文提示 + 退出代码 1" 的模式。** 对于 CLI 工具，直接给出有用的提示比阻断自动化流程更好。

1. 清理 `src/cli.rs` 中所有类似 `（省略则交互式选择）` 的注释，改为 `（省略将列出可用基金并提示）`。
2. 将以下必填参数改为 `Option`：

| 命令 | 参数 | 改动 |
|------|------|------|
| `buy` | `<FUND>` | → `Option<String>` |
| `sell` | `<FUND>` | → `Option<String>` |
| `history` | `<FUND>` | → `Option<String>` |
| `fund delete` | `<FUND>` | → `Option<String>` |
| `fund inspect` | `<FUND>` | → `Option<String>` |
| `buy` | `--money` | → `Option<Decimal>` |

handler 入口处统一使用 `require_fund_or_exit` 检查，缺失时输出友好错误并 exit(1)。

### 扩展 `require_fund_or_exit` 设计

`require_fund_or_exit` 应根据不同的子命令提供更智能的上下文提示：
- 对于 `sell`：提示的列表应只包含**当前有持仓**的基金。
- 对于 `buy`/`history`/`fund inspect`：可以列出**当前钱包中所有追踪**的基金。

## panic 替换清单

| 位置 | 原始 | 替换为 |
|------|------|--------|
| sell handler L1632 | `.expect("未找到该基金的净值数据...")` | `eprintln!` + `exit(1)` |
| sell handler L1614 | `.expect("无效的 --nav 参数")` | `eprintln!` + `exit(1)` |
| sell handler L1619 | `.expect("数据库错误")` | `eprintln!` + `exit(1)` |
| sell handler L1633 | `.expect("数据库中的净值数据无效")` | `eprintln!` + `exit(1)` |
| sell handler L1641 | `.expect("无效的 --shares 参数")` | `eprintln!` + `exit(1)` |
| sell handler L1643 | `.expect("无效的 --money 参数")` | `eprintln!` + `exit(1)` |
| sell handler L1666 | `.expect("无效的 --fee 参数")` | `eprintln!` + `exit(1)` |
| sell handler L1704 | `.expect("记录交易失败")` | `eprintln!` + `exit(1)` |
| buy handler L1572 | `.expect("记录交易失败")` | `eprintln!` + `exit(1)` |
| buy handler L1558 | `.expect("手动买入模式必须提供 --shares")` | `eprintln!` + `exit(1)` |
| buy handler L1559 | `.expect("手动买入模式必须提供 --nav")` | `eprintln!` + `exit(1)` |
| buy handler L1485 | `.expect("无法查询净值")` | `eprintln!` + `exit(1)` |
| buy handler L1515 | `.expect("记录交易失败")` | `eprintln!` + `exit(1)` |
| buy handler L1548 | `.expect("记录交易失败")` | `eprintln!` + `exit(1)` |
