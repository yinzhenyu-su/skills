---
name: fund-manager
description: 中国公募基金投资管理 CLI 工具。使用场景：(1) 用户询问基金持仓、基金盈亏；(2) 用户询问"我的基金"、"基金状态"；(3) 用户想要买入/卖出基金；(4) 用户想要同步基金净值、查看基金历史；(5) 用户想要管理钱包、导入持仓。
---

# fund-manager

中国公募基金投资管理 CLI 工具，支持基金持仓追踪、盈亏计算、从东方财富和晨星同步净值数据。

## 项目位置

`skills/fund-manager/` - Rust CLI 项目

## 常用命令

```bash
cd skills/fund-manager && cargo build     # 构建
cd skills/fund-manager && cargo run -- [args]  # 运行
```

## 核心功能

| 功能 | 命令示例 |
|------|---------|
| 钱包管理 | `fund-manager wallet {add, list, use, delete, rename}` |
| 添加基金 | `fund-manager fund add <基金代码>` |
| 查看持仓 | `fund-manager status [fund] [--wallet]` |
| 买入基金 | `fund-manager buy <基金> --money <金额>` |
| 卖出基金 | `fund-manager sell <基金> --shares <份额>` |
| 交易历史 | `fund-manager history <基金>` |
| 同步净值 | `fund-manager fund sync [fund]` |
| 预览交易 | `fund-manager preview {buy, sell} <基金> [--money\|--shares]` |
| 导入持仓 | `fund-manager import-holding --file <文件>` |

## 使用场景

**查看所有基金状态：**
```
fund-manager status
```

**查看单个基金详情：**
```
fund-manager status <基金代码>
fund-manager status <基金代码> --wallet <钱包名>
```

**买入基金（按金额）：**
```
fund-manager buy <基金代码> --money 10000 --wallet <钱包>
```

**卖出基金（按份额）：**
```
fund-manager sell <基金代码> --shares 100 --wallet <钱包>
```

**同步基金净值：**
```
fund-manager fund sync           # 同步所有基金
fund-manager fund sync <基金代码>  # 同步单个基金
```

## 数据存储

- SQLite 数据库：`~/.config/fund-manager/fund.db`
- 可通过 `FUND_MANAGER_APP_DIR` 环境变量覆盖

## 环境变量

| 变量 | 说明 |
|------|------|
| `FUND_MANAGER_APP_DIR` | 应用数据目录 |
| `FUND_MANAGER_UA` | 自定义 User-Agent |
| `SKIP_SYNC` | 跳过净值同步 |
| `FORCE_SYNC_FAILURE` | 强制同步失败（测试用）|

## 注意事项

- 所有金额使用 Decimal 精度，无浮点数误差
- 日期格式统一为 `YYYY-MM-DD`
- 基金代码为 6 位数字
- 买入时若净值不可用，交易状态为 `pending`，净值同步后自动结算
