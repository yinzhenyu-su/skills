use clap::{Parser, Subcommand};
use rust_decimal::Decimal;

#[derive(Parser)]
#[command(name = "fund-manager")]
#[command(about = "一个用于管理基金投资数据的跨平台命令行工具。", long_about = None)]
pub struct Cli {
    #[command(subcommand)]
    pub command: Commands,

    /// 跳过交互式确认
    #[arg(short, long, global = true, default_value_t = false)]
    pub yes: bool,
}

#[derive(Subcommand)]
pub enum Commands {
    /// 钱包管理相关命令
    Wallet {
        #[command(subcommand)]
        command: WalletCommands,
    },
    /// 基金管理相关命令
    Fund {
        #[command(subcommand)]
        command: FundCommands,
    },
    /// 查看当前持仓盈亏状态
    #[command(
        long_about = "查看当前持仓状态，包括市值、成本和盈亏。\n\n示例：\n    fund status\n    fund status 000300\n    fund status --wallet 我的投资"
    )]
    Status {
        /// 基金代码或名称（可选，省略则显示全部）
        fund: Option<String>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
    },
    /// 查看交易历史
    #[command(
        long_about = "查看特定基金的交易历史记录。\n\n示例：\n    fund history 000300\n    fund history --wallet 我的投资\n    fund history --type buy --limit 10"
    )]
    History {
        /// 基金代码或名称（可选）
        fund: Option<String>,
        /// 筛选特定钱包
        #[arg(long)]
        wallet: Option<String>,
        /// 筛选交易类型 (buy, sell, dividend, reinvest, import)
        #[arg(long, name = "type")]
        t_type: Option<String>,
        /// 限制显示条数 (默认 0 为不限制)
        #[arg(long, default_value_t = 0)]
        limit: i64,
    },
    /// 买入基金
    #[command(
        long_about = "记录一笔买入交易。\n\n示例：\n    fund buy 000300 --money 1000\n    fund buy 000300 --money 1000 --date 2024-01-01\n    fund buy 000300 --shares 800 --nav 1.25 --date 2024-01-01"
    )]
    Buy {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 投入金额
        #[arg(long)]
        money: Option<Decimal>,
        /// 显式指定份额（如果不使用自动计算模式）
        #[arg(long)]
        shares: Option<Decimal>,
        /// 显式指定成交净值（如果不使用自动计算模式）
        #[arg(long)]
        nav: Option<Decimal>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
        /// 交易日期 (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// 卖出基金
    #[command(
        long_about = "记录一笔卖出交易。\n\n示例：\n    fund sell 000300 --shares 500\n    fund sell 000300 --money 1000 --date 2024-01-01\n    fund sell 000300 --shares 1/2 --nav 1.25 --date 2024-01-01"
    )]
    Sell {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 预期收回金额 (例如 "1000")
        #[arg(long)]
        money: Option<String>,
        /// 显式卖出份额 (例如 "500", "1/2", "all")
        #[arg(long)]
        shares: Option<String>,
        /// 显式成交净值 (例如 "1.23")
        #[arg(long)]
        nav: Option<String>,
        /// 赎回手续费 (例如 "5.0", "0.5%")
        #[arg(long)]
        fee: Option<String>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
        /// 交易日期 (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// 从其他平台导入基金持仓（CSV 格式：基金名称,持有金额,持有收益）
    #[command(
        name = "import-holding",
        long_about = "从 CSV 文件批量导入基金持仓数据。\n\n示例：\n    fund import-holding --file holdings.csv\n    fund import-holding --file holdings.csv --override\n    fund import-holding --file holdings.csv --wallet 我的钱包\n\nCSV 格式说明：\n    基金名称,持有金额,持有收益\n    中欧医疗健康混合A,11000,1000\n    注意：持有收益包含现金分红"
    )]
    ImportHolding {
        /// 要导入的 CSV 文件路径
        #[arg(long)]
        file: std::path::PathBuf,
        /// 与现有导入记录合并（默认行为，已存在则跳过）
        #[arg(long, conflicts_with = "override_flag")]
        merge: bool,
        /// 完全覆盖现有导入记录
        #[arg(long = "override")]
        override_flag: bool,
        /// 导入使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
    },
    /// 记录一笔分红（现金分红）
    #[command(
        long_about = "记录一笔基金分红（现金分红，不改变份额，仅降低成本）。\n\n示例：\n    fund dividend 000300 --money 100\n    fund dividend 000300 --money 100 --date 2024-03-15"
    )]
    Dividend {
        /// 基金代码或名称
        fund: Option<String>,
        /// 分红金额 (现金)
        #[arg(long)]
        money: Decimal,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
        /// 交易日期 (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// 记录一笔红利再投
    #[command(
        long_about = "记录一笔红利再投（不涉及现金流动，仅增加份额）。\n\n示例：\n    fund reinvest 000300 --shares 50 --nav 2.0\n    fund reinvest 000300 --shares 50 --date 2024-03-15"
    )]
    Reinvest {
        /// 基金代码或名称
        fund: Option<String>,
        /// 再投份额
        #[arg(long)]
        shares: Decimal,
        /// 成交净值（用于记录，不影响现金流）
        #[arg(long)]
        nav: Option<Decimal>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
        /// 交易日期 (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// 预览交易结果（不执行实际交易）
    #[command(
        long_about = "预览买入或卖出交易结果，不执行实际操作。\n\n示例：\n    fund-manager preview buy 000312 --money 5000\n    fund-manager preview sell 000312 --shares 500"
    )]
    Preview {
        #[command(subcommand)]
        command: PreviewCommands,
    },
    /// 查询主要市场行情（指数、外汇、大宗商品）
    #[command(
        name = "market",
        long_about = "查询主要市场指数、外汇及大宗商品的实时行情。\n\n示例：\n    fund market                 # 查询所有默认行情\n    fund market --fx            # 仅查询外汇行情\n    fund market --com           # 仅查询大宗商品\n    fund market --detail        # 查看包含 52 周区间的详细水位\n    fund market --trend         # 查看日内走势图\n    fund market 沪深300 黄金    # 查询多个指定的项"
    )]
    Market {
        /// 指定要查询的名称（可选，省略则显示全部默认行情）
        #[arg(num_args(0..))]
        names: Vec<String>,
        /// 仅显示外汇行情
        #[arg(long, default_value_t = false)]
        fx: bool,
        /// 仅显示大宗商品行情
        #[arg(long, default_value_t = false)]
        com: bool,
        /// 仅显示股市指数
        #[arg(long, default_value_t = false)]
        index: bool,
        /// 仅显示热门资产
        #[arg(long, default_value_t = false)]
        hot: bool,
        /// 显示详细水位（52 周区间）
        #[arg(short, long, default_value_t = false)]
        detail: bool,
        /// 显示趋势图（火花图）
        #[arg(short, long, default_value_t = false)]
        trend: bool,
    },
}

#[derive(Subcommand)]
pub enum PreviewCommands {
    /// 预览买入结果（不执行实际买入）
    #[command(
        long_about = "预览买入交易结果，不执行实际买入。\n\n示例：\n    fund-manager preview buy 000312 --money 5000\n    fund-manager preview buy 000312 --shares 4538.65\n    fund-manager preview buy 000312 --money 5000 --nav 1.05\n    fund-manager preview buy 000312 --money 5000 --date 2024-01-01"
    )]
    Buy {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 投入金额
        #[arg(long)]
        money: Option<Decimal>,
        /// 显式指定份额
        #[arg(long)]
        shares: Option<Decimal>,
        /// 显式指定成交净值
        #[arg(long)]
        nav: Option<Decimal>,
        /// 指定日期（查询该日期之前的最近净值）
        #[arg(long)]
        date: Option<String>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
    },
    /// 预览卖出结果（不执行实际卖出）
    #[command(
        long_about = "预览卖出交易结果，不执行实际卖出。\n\n示例：\n    fund-manager preview sell 000312 --shares 500\n    fund-manager preview sell 000312 --money 5500\n    fund-manager preview sell 000312 --shares 500 --nav 1.05\n    fund-manager preview sell 000312 --shares 500 --date 2024-01-01"
    )]
    Sell {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 预期收回金额 (例如 "1000")
        #[arg(long)]
        money: Option<String>,
        /// 显式卖出份额 (例如 "500", "1/2", "all")
        #[arg(long)]
        shares: Option<String>,
        /// 显式指定成交净值 (例如 "1.23")
        #[arg(long)]
        nav: Option<String>,
        /// 指定日期（查询该日期之前的最近净值）
        #[arg(long)]
        date: Option<String>,
        /// 手动指定赎回费率（自动从持有天数推算，指定后跳过自动查询）
        #[arg(long)]
        fee: Option<String>,
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
    },
}

#[derive(Subcommand)]
pub enum WalletCommands {
    /// 添加一个新钱包
    #[command(
        long_about = "添加一个新钱包以管理多个投资组合。\n\n示例：\n    fund wallet add 我的投资"
    )]
    Add {
        /// 钱包名称
        name: String,
    },
    /// 列出所有钱包
    List,
    /// 设置特定钱包为当前活跃钱包
    #[command(
        long_about = "将特定钱包设置为后续命令的默认操作钱包。\n\n示例：\n    fund wallet use 我的投资"
    )]
    Use {
        /// 要使用的钱包名称
        name: String,
    },
    /// 删除一个钱包及其所有数据
    #[command(
        alias = "del",
        long_about = "从本地数据库中移除钱包及其所有的交易历史记录。\n\n示例：\n    fund wallet delete 我的投资"
    )]
    Delete {
        /// 要删除的钱包名称
        name: String,
    },
    /// 重命名钱包
    #[command(
        long_about = "将钱包重命名为新名称。\n\n示例：\n    fund wallet rename 我的投资 投资组合"
    )]
    Rename {
        /// 钱包当前名称
        old_name: String,
        /// 钱包新名称
        new_name: String,
    },
}

#[derive(Subcommand)]
pub enum FundCommands {
    /// 手动添加一个基金到追踪列表
    #[command(
        long_about = "手动将基金添加到本地追踪列表。\n\n示例：\n    fund fund add 000300\n    fund fund add 000300 --fee 0.0015"
    )]
    Add {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 初始申购费率（例如 0.0015 表示 0.15%）
        #[arg(long)]
        fee: Option<String>,
    },
    /// 删除基金及其所有数据
    #[command(
        long_about = "从本地数据库中移除基金及其所有的交易历史记录。\n\n示例：\n    fund fund delete 000300"
    )]
    Delete {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
    },
    /// 列出所有追踪中的基金
    List {
        /// 交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,
    },
    /// 从远程同步基金元数据和历史净值
    #[command(
        long_about = "同步基金元数据及历史净值数据。\n\n示例：\n    fund fund sync\n    fund fund sync 000300 --start 2024-01-01\n    fund fund sync --auto-fill"
    )]
    Sync {
        /// 强制同步元数据（即使未过期）
        #[arg(long, default_value_t = false)]
        force: bool,
        /// 历史补全的开始日期 (YYYY-MM-DD)
        #[arg(long)]
        start: Option<String>,
        /// 历史补全的结束日期 (YYYY-MM-DD)
        #[arg(long)]
        end: Option<String>,
        /// 自动检测并补全历史净值空隙
        #[arg(long, default_value_t = false)]
        auto_fill: bool,
        /// 同步所有基金：元数据 + 最近30天的净值历史
        #[arg(long, short, default_value_t = false)]
        all: bool,
        /// 要同步的特定基金（如果不使用 --all 则必填）
        fund: Option<String>,
    },
    /// 查看基金的健康度及表现 (Morningstar)
    #[command(
        long_about = "显示基于晨星分析的基金深度健康报告。\n\n示例：\n    fund fund inspect 000513\n    fund fund inspect \"汇添富全球医疗\""
    )]
    Inspect {
        /// 基金代码或名称（省略将列出可用基金并提示）
        fund: Option<String>,
        /// 强制刷新分析数据
        #[arg(short, long, default_value_t = false)]
        force: bool,
    },
    /// 配置基金属性（例如分红方式）
    #[command(
        long_about = "更新基金的配置属性，如分红方式（现金分红 vs 红利再投）。\n\n示例：\n    fund fund config 000300 --dividend-mode reinvest"
    )]
    Config {
        /// 基金代码或名称
        fund: String,
        /// 设置分红方式 (cash: 现金分红, reinvest: 红利再投)
        #[arg(long, value_parser = ["cash", "reinvest"])]
        dividend_mode: String,
    },
    /// 重置所有数据（清除所有钱包、基金、交易历史）
    #[command(
        long_about = "永久删除所有个人数据，包括钱包、基金、交易历史和配置。此操作不可恢复！\n\n示例：\n    fund reset\n    fund reset -y"
    )]
    Reset,
}
