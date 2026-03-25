---
name: fund-manager
description: 中国公募基金投资管理 CLI 工具。使用场景：(1) 用户询问基金持仓、基金盈亏；(2) 用户询问"我的基金"、"基金状态"；(3) 用户想要买入/卖出基金；(4) 用户想要同步基金净值、查看基金历史；(5) 用户想要管理钱包、导入持仓；(6) 用户想要查看市场指数行情；(7) 用户想要记录分红/红利再投。
metadata:
	openclaw:
		requires:
			bins: ["bash", "curl", "tar", "unzip"]
			env: ["FUND_MANAGER_GITLAB_PROJECT_ID", "FUND_MANAGER_GITLAB_TOKEN"]
---

# fund-manager

中国公募基金投资管理 CLI 工具，支持基金持仓追踪、盈亏计算、从东方财富和晨星同步净值数据。

## 项目位置

`skills/fund-manager/` - Rust CLI 项目

## 运行方式

优先使用包装脚本（推荐）：

```bash
cd skills/fund-manager
chmod +x scripts/bootstrap-fund-manager.sh scripts/fund-manager.sh
./scripts/fund-manager.sh --help
```

包装脚本行为：

- 首次运行：识别当前平台并下载 `fund-manager` 核心二进制
- 后续运行：复用本地缓存，避免重复下载
- 缓存损坏：自动重新下载

## 开发命令（源码模式）

```bash
cd skills/fund-manager && cargo build     # 构建
cd skills/fund-manager && cargo run -- [args]  # 运行
```

## 二进制下载配置

### 必选（未设置 `FUND_MANAGER_CORE_URL` 时）

- `FUND_MANAGER_GITLAB_PROJECT_ID`：GitLab 项目 ID
- `FUND_MANAGER_GITLAB_TOKEN`：私有仓库访问令牌（公开仓库可不填）

### 可选

- `FUND_MANAGER_VERSION`：下载版本，默认 `latest`
- `FUND_MANAGER_CORE_URL`：覆盖默认下载地址（用于回滚或自定义分发源）
- `FUND_MANAGER_CORE_SHA256`：归档校验值（设置后会校验下载完整性）
- `FUND_MANAGER_GITLAB_BASE_URL`：GitLab 基础地址，默认 `https://gitlab.com`
- `FUND_MANAGER_GITLAB_PACKAGE`：Package Registry 包名，默认 `fund-manager`
- `FUND_MANAGER_CACHE_DIR`：本地缓存目录

### 示例

```bash
# 公开仓库：只需要项目 ID
export FUND_MANAGER_GITLAB_PROJECT_ID="12345678"
./scripts/fund-manager.sh status

# 私有仓库：增加 token
export FUND_MANAGER_GITLAB_PROJECT_ID="12345678"
export FUND_MANAGER_GITLAB_TOKEN="<your-token>"
./scripts/fund-manager.sh fund list

# 固定版本
export FUND_MANAGER_VERSION="0.1.0"
./scripts/fund-manager.sh status

# 强制覆盖下载源
export FUND_MANAGER_CORE_URL="https://gitlab.example.com/api/v4/projects/123/packages/generic/fund-manager/0.1.0/fund-manager-v0.1.0-x86_64-unknown-linux-gnu.tar.gz"
./scripts/fund-manager.sh --help
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
fund-manager index                    # 查询所有默认指数
fund-manager index 沪深300 纳斯达克   # 查询指定指数
```

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
fund-manager fund sync --all           # 同步所有基金
fund-manager fund sync <基金代码>       # 同步单个基金
```

**查看晨星健康报告：**
```
fund-manager fund inspect <基金代码>
```

**查询市场指数：**
```
fund-manager index
fund-manager index 沪深300 标普500
```

**记录分红：**
```
fund-manager dividend <基金代码> --money 100
```

## 数据源架构

数据从多个 Provider 获取，通过 `aggregator` 合并：

| Provider | 数据来源 | 用途 |
|----------|---------|------|
| `eastmoney_js` | fundgz.1234567.com.cn | 实时净值、名称 |
| `eastmoney_html` | fund.eastmoney.com | 申购费率 |
| `eastmoney_details` | fundmobapi.eastmoney.com | 基金元数据（类型、经理等）|
| `eastmoney_lsjz` | api.fund.eastmoney.com | 历史净值（分页） |
| `morningstar` | morningstar.cn | 晨星分析（评级、夏普比率等）|
| `morningstar_search` | morningstar.cn | 基金名称/代码搜索 |
| `morningstar_market` | morningstar.cn | 市场指数行情 |

## 核心模块

| 模块 | 职责 |
|------|------|
| `src/db.rs` | SQLite 数据库 CRUD、WAL 模式、外键约束 |
| `src/finance.rs` | 金融计算：申购份额、赎回费、费率解析 |
| `src/resolver.rs` | 基金解析：本地搜索、远程搜索、模糊匹配、纠错建议 |
| `src/sync.rs` | 净值同步、待确认交易自动结算 |
| `src/config.rs` | 应用目录路径、环境变量配置 |

## 数据存储

- SQLite 数据库：`~/.config/fund-manager/fund.db`
- 可通过 `FUND_MANAGER_APP_DIR` 环境变量覆盖

## 环境变量

| 变量 | 说明 |
|------|------|
| `FUND_MANAGER_GITLAB_PROJECT_ID` | GitLab 项目 ID（默认下载源必需） |
| `FUND_MANAGER_GITLAB_TOKEN` | GitLab 私有仓库访问 token |
| `FUND_MANAGER_VERSION` | 二进制版本（默认 latest） |
| `FUND_MANAGER_CORE_URL` | 强制覆盖下载地址 |
| `FUND_MANAGER_CORE_SHA256` | 下载归档 SHA256（可选） |
| `FUND_MANAGER_GITLAB_BASE_URL` | GitLab 地址（默认 https://gitlab.com） |
| `FUND_MANAGER_GITLAB_PACKAGE` | Package Registry 包名 |
| `FUND_MANAGER_CACHE_DIR` | 二进制缓存目录 |
| `FUND_MANAGER_APP_DIR` | 应用数据目录 |
| `FUND_MANAGER_UA` | 自定义 User-Agent |
| `SKIP_SYNC` | 跳过净值同步 |
| `FORCE_SYNC_FAILURE` | 强制同步失败（测试用）|

## GitLab CI 产物规范

- Linux: `fund-manager-v<version>-x86_64-unknown-linux-gnu.tar.gz`
- macOS: `fund-manager-v<version>-aarch64-apple-darwin.tar.gz`
- Windows: `fund-manager-v<version>-x86_64-pc-windows-msvc.zip`（内含 `fund-manager.exe`）

默认下载路径（Package Registry）：

```text
{GITLAB_BASE_URL}/api/v4/projects/{PROJECT_ID}/packages/generic/fund-manager/{VERSION}/{ASSET_NAME}
```

## 常见问题

- `401 Unauthorized`：检查 `FUND_MANAGER_GITLAB_TOKEN` 是否有效、是否有读取 package 权限。
- `403 Forbidden`：检查项目可见性、token 作用域与项目 ID 是否正确。
- `404 Not Found`：检查 `FUND_MANAGER_VERSION` 与平台产物名是否存在。
- 下载后执行失败：删除缓存目录后重试，或设置 `FUND_MANAGER_CORE_SHA256` 开启校验。

## 注意事项

- 所有金额使用 `rust_decimal::Decimal` 精度，无浮点数误差
- 日期格式统一为 `YYYY-MM-DD`
- 基金代码为 6 位数字
- 买入时若净值不可用，交易状态为 `pending`，净值同步后自动结算
- 卖出时使用前一交易日净值（若当日净值未发布）
