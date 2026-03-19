# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

fund-manager 是一个 Rust CLI 工具，用于管理中国公募基金投资。支持基金持仓追踪、盈亏计算、从东方财富和晨星同步净值数据。

## 常用命令

```bash
cargo build                    # 构建
cargo build --release          # 发布构建
cargo run -- [args]            # 运行
cargo test                     # 运行所有测试
cargo test <test_name>         # 运行指定测试
cargo test --test integration_tests  # 仅集成测试
cargo test --test cli_tests          # 仅 CLI 测试
cargo test --test db_tests           # 仅数据库测试
cargo test --test cli_wallet_tests   # 仅钱包测试
```

## 架构

### 核心模块

- `src/main.rs` — 命令分发入口（~2000 行），所有 CLI 子命令的实现
- `src/cli.rs` — clap derive 定义所有 CLI 参数和子命令结构
- `src/db.rs` — SQLite 数据库：schema、CRUD、WAL 模式、外键约束
- `src/finance.rs` — 金融计算：申购、份额解析、费率解析
- `src/resolver.rs` — 基金解析：本地搜索、远程搜索、模糊匹配
- `src/sync.rs` — 净值同步、待确认交易结算
- `src/config.rs` — 应用目录路径、UA、环境变量覆盖
- `src/provider/` — 数据提供者抽象层

### Provider 模块

`Provider` trait 定义在 `src/provider/mod.rs`，各实现：

| Provider | 数据来源 | 提供数据 |
|----------|---------|---------|
| `eastmoney_js` | fundgz.1234567.com.cn | 实时净值、名称 |
| `eastmoney_html` | fund.eastmoney.com HTML | 申购费率 |
| `eastmoney_details` | fundmobapi.eastmoney.com | 基金元数据 |
| `eastmoney_lsjz` | api.fund.eastmoney.com | 历史净值（分页） |
| `morningstar` | morningstar.cn | 分析数据（评级等） |
| `morningstar_search` | morningstar.cn 搜索 | 基金名称/代码搜索 |
| `aggregator` | 合并多数据源 | 综合 FundData（10s 超时，允许部分成功） |

### CLI 命令结构

```
fund-manager
  wallet {add, list, use, delete, rename}
  fund {add, delete, list, sync, inspect}
  status [fund] [--wallet]
  history <fund>
  buy <fund> --money [--shares|--nav] [--wallet] [--date]
  sell <fund> --shares|--money [--nav] [--fee] [--wallet] [--date]
  import [--file|--pairs] [--merge|--override] [--wallet] [--date]
  import-holding --file [--merge|--override] [--wallet]
  preview {buy, sell} [fund] [--money|--shares] [--nav] [--date] [--wallet]
```

### 数据库表

SQLite，路径 `~/.config/fund-manager/fund.db`（可通过 `FUND_MANAGER_APP_DIR` 覆盖）。

- `wallet` — 钱包
- `fund` — 基金信息（code 主键）
- `nav_history` — 历史净值（复合主键 fund_code + date）
- `transaction_log` — 交易记录（外键关联 wallet 和 fund）
- `fund_analysis` — 基金分析数据
- `wallet_config` — 钱包配置

开启 `PRAGMA foreign_keys = ON`，使用 WAL 日志模式。

## 关键约定

- **金额精度**：所有金额使用 `rust_decimal::Decimal`，不使用浮点数
- **日期格式**：统一 `YYYY-MM-DD` 字符串
- **基金代码**：6 位数字
- **待确认交易**：买入时若净值不可用，交易状态为 `pending`，净值同步后自动结算
- **用户界面**：所有面向用户的输出使用中文
- **全局 `-y` 参数**：跳过交互确认
- **环境变量**：`FUND_MANAGER_APP_DIR`、`FUND_MANAGER_UA`、`SKIP_SYNC`、`FORCE_SYNC_FAILURE`

## 测试模式

- 单元测试：各源文件内 `#[cfg(test)] mod tests`
- 集成测试：`tests/` 目录，`TestContext` 辅助类（`tests/common/context.rs`）
- CLI 测试：`assert_cmd` + `predicates`，通过 `.write_stdin("y\n")` 模拟交互确认
- Mock：`mockall` crate mock `Provider` trait
- 数据库测试：`db::setup_test_db()` 创建内存 SQLite
