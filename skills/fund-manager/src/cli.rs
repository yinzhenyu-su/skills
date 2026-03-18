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
        long_about = "查看当前持仓状态，包括市值、成本和盈亏。\n\n示例：\n    fund status\n    fund status 000300"
    )]
    Status {
        /// 基金代码或名称（可选，省略则显示全部）
        fund: Option<String>,
    },
    /// 查看交易历史
    #[command(
        long_about = "查看特定基金的交易历史记录。\n\n示例：\n    fund history 000300"
    )]
    History {
        /// 基金代码或名称
        fund: String,
    },
    /// 买入基金
    #[command(
        long_about = "记录一笔买入交易。\n\n示例：\n    fund buy 000300 --money 1000\n    fund buy 000300 --money 1000 --date 2024-01-01\n    fund buy 000300 --shares 800 --nav 1.25 --date 2024-01-01"
    )]
    Buy {
        /// 基金代码或名称
        fund: String,
        /// 投入金额
        #[arg(long)]
        money: Decimal,
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
        /// 基金代码或名称
        fund: String,
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
    /// 从 CSV 文件或参数批量导入基金持仓
    #[command(
        long_about = "批量导入基金持仓记录。\n\n示例：\n    fund import --file data.csv\n    fund import 000300 5000 000001 2000 --date 2024-01-01\n\nCSV 格式说明：\n    名称,金额[,日期]\n    沪深300,5000,2024-01-01"
    )]
    Import {
        /// 要导入的 CSV 文件路径
        #[arg(long)]
        file: Option<std::path::PathBuf>,

        /// 基金名称和金额的成对参数 [名称 金额]...
        #[arg(num_args(0..))]
        pairs: Vec<String>,

        /// 与现有持仓合并（默认行为）
        #[arg(long, conflicts_with = "override_flag")]
        merge: bool,

        /// 完全覆盖现有持仓记录
        #[arg(long = "override")]
        override_flag: bool,

        /// 导入交易使用的特定钱包
        #[arg(long)]
        wallet: Option<String>,

        /// 此次导入的全局交易日期 (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
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
}

#[derive(Subcommand)]
pub enum FundCommands {
    /// 手动添加一个基金到追踪列表
    #[command(
        long_about = "手动将基金添加到本地追踪列表。\n\n示例：\n    fund fund add 000300 沪深300 --fee 0.0015"
    )]
    Add {
        /// 基金代码
        code: String,
        /// 基金名称
        name: String,
        /// 初始申购费率（例如 0.0015 表示 0.15%）
        #[arg(long, default_value = "0.00")]
        fee: String,
    },
    /// 删除基金及其所有数据
    #[command(
        long_about = "从本地数据库中移除基金及其所有的交易历史记录。\n\n示例：\n    fund fund delete 000300"
    )]
    Delete {
        /// 基金代码或名称
        fund: String,
    },
    /// 列出所有追踪中的基金
    List,
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
        /// 基金代码或名称
        fund: String,
        /// 强制刷新分析数据
        #[arg(short, long, default_value_t = false)]
        force: bool,
    },
}
