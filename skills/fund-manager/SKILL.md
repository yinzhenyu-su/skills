---
name: fund-manager
description: 中国公募基金投资管理 CLI 工具。使用场景：(1) 用户询问基金持仓、基金盈亏；(2) 用户询问"我的基金"、"基金状态"；(3) 用户想要买入/卖出基金；(4) 用户想要同步基金净值、查看基金历史；(5) 用户想要管理钱包、导入持仓；(6) 用户想要查看市场指数行情；(7) 用户想要记录分红/红利再投。
metadata:
  openclaw:
    requires:
      bins: ["bash", "curl", "tar", "unzip"]
      env: ["FUND_MANAGER_REPO"]
---

# fund-manager

中国公募基金投资管理 CLI 工具，支持基金持仓追踪、盈亏计算、从东方财富和晨星同步净值数据。

## 项目位置

`skills/fund-manager/` - Rust CLI 项目

## 运行方式

使用 bootstrap 脚本（推荐）：

```bash
cd skills/fund-manager
chmod +x scripts/bootstrap.sh
./scripts/bootstrap.sh --help
```

Bootstrap 脚本行为：

- 首次运行：识别当前平台并从 GitHub Releases 下载 `fund-manager` 核心二进制
- 后续运行：复用本地缓存（`~/.cache/fund-manager/bin/`），避免重复下载
- 缓存损坏：自动重新下载

## 开发命令（源码模式）

```bash
cd skills/fund-manager && cargo build     # 构建
cd skills/fund-manager && cargo run -- [args]  # 运行
```

## 二进制下载配置

### 必选（未设置 `FUND_MANAGER_CORE_URL` 时）

- `FUND_MANAGER_REPO`：GitHub 仓库路径 (例如 `YinZ-510/skills`)，默认 `YinZ-510/skills`
- `FUND_MANAGER_GITHUB_TOKEN`：GitHub 访问令牌（私有仓库或避免 API 限制时使用，也可直接使用 `GITHUB_TOKEN`）

### 可选

- `FUND_MANAGER_VERSION`：下载版本，默认 `latest`
- `FUND_MANAGER_CORE_URL`：覆盖默认下载地址
- `FUND_MANAGER_CORE_SHA256`：归档校验值
- `FUND_MANAGER_GITHUB_BASE_URL`：GitHub 基础地址，默认 `https://github.com`
- `FUND_MANAGER_CACHE_DIR`：本地缓存目录

### 示例

```bash
# 公开仓库（默认仓库）
./scripts/fund-manager.sh status

# 指定仓库
export FUND_MANAGER_REPO="yinzhenyu-su/skills"
./scripts/fund-manager.sh status

# 私有仓库
export FUND_MANAGER_REPO="owner/private-repo"
export FUND_MANAGER_GITHUB_TOKEN="<your-token>"
./scripts/fund-manager.sh fund list

# 固定版本
export FUND_MANAGER_VERSION="0.1.0"
./scripts/fund-manager.sh status
```

## CLI 概述

- **二进制名**：`fund-manager`
- **全局参数**：`-y/--yes` 跳过交互确认
- **退出码**：`0` 成功、`1` 错误、`3` 需要确认

## 核心命令

### 钱包管理

```bash
fund-manager wallet add <名称>           # 添加钱包
fund-manager wallet list                 # 列出所有钱包
fund-manager wallet use <名称>           # 设置活跃钱包
fund-manager wallet delete <名称>        # 删除钱包
fund-manager wallet rename <旧名> <新名> # 重命名钱包
```

### 基金追踪

```bash
fund-manager fund add <基金代码> [--fee <费率>]  # 添加基金
fund-manager fund delete <基金代码>               # 删除基金
fund-manager fund list [--wallet <钱包>]          # 列出追踪的基金
fund-manager fund inspect <基金代码> [--force]     # 查看晨星健康报告
fund-manager fund config <基金代码> --dividend-mode reinvest  # 配置基金（如分红方式）
```

### 净值同步

```bash
fund-manager fund sync <基金代码>                   # 同步单个基金
fund-manager fund sync --all                        # 同步所有基金
fund-manager fund sync --all --auto-fill            # 同步并补全净值空隙
fund-manager fund sync <基金代码> --start 2024-01-01 # 指定历史范围
```

### 持仓与交易

```bash
fund-manager status [基金代码] [--wallet <钱包>]      # 查看持仓
fund-manager buy <基金代码> --money <金额>             # 买入（按金额）
fund-manager buy <基金代码> --shares <份额> --nav <净值>  # 买入（按份额）
fund-manager sell <基金代码> --shares <份额>           # 卖出
fund-manager sell <基金代码> --shares all              # 全部卖出
fund-manager sell <基金代码> --shares 1/2              # 卖出一半
fund-manager history [基金代码] [--type buy|sell|dividend|reinvest|import] [--limit N]
fund-manager preview buy <基金代码> --money <金额>     # 预览买入
fund-manager preview sell <基金代码> --shares <份额>   # 预览卖出
```

### 分红记录

```bash
fund-manager dividend <基金代码> --money <金额>  # 记录现金分红
fund-manager reinvest <基金代码> --shares <份额> # 记录红利再投
```

### 持仓导入

```bash
fund-manager import-holding --file <CSV文件>          # 从其他平台导入（格式：名称,持有金额,持有收益）
```

### 市场指数

```bash
fund-manager market                         # 查询默认行情
fund-manager market 沪深300 纳斯达克        # 查询指定指数
fund-manager market --fx                    # 仅外汇
fund-manager market --com                   # 仅大宗商品
fund-manager market --detail                 # 包含 52 周区间
```

## 数据存储

- SQLite 数据库：`~/.config/fund-manager/fund.db`
- 可通过 `FUND_MANAGER_APP_DIR` 环境变量覆盖

## GitHub Actions 产物规范

支持以下 6 种平台/架构组合：

- Linux: `fund-manager-v<version>-x86_64-unknown-linux-gnu.tar.gz`
- Linux ARM: `fund-manager-v<version>-aarch64-unknown-linux-gnu.tar.gz`
- macOS: `fund-manager-v<version>-x86_64-apple-darwin.tar.gz`
- macOS Apple Silicon: `fund-manager-v<version>-aarch64-apple-darwin.tar.gz`
- Windows: `fund-manager-v<version>-x86_64-pc-windows-gnu.zip`
- Windows ARM: `fund-manager-v<version>-aarch64-pc-windows-gnullvm.zip`

默认下载路径（GitHub Release）：

```text
{GITHUB_BASE_URL}/{REPO}/releases/download/v{VERSION}/{ASSET_NAME}
```

## 常见问题

- `401 Unauthorized`：检查 `FUND_MANAGER_GITHUB_TOKEN` 是否有效。
- `404 Not Found`：检查 `FUND_MANAGER_VERSION` 与平台产物名是否存在。
- 下载后执行失败：删除缓存目录后重试。

## 注意事项

- 所有金额使用 `rust_decimal::Decimal` 精度，无浮点数误差
- 日期格式统一为 `YYYY-MM-DD`
- 基金代码为 6 位数字
- 买入时若净值不可用，交易状态为 `pending`，净值同步后自动结算
- 卖出时使用前一交易日净值（若当日净值未发布）
