use clap::{Parser, Subcommand};
use rust_decimal::Decimal;

#[derive(Parser)]
#[command(name = "fund-manager")]
#[command(about = "A cross-platform CLI tool to manage your fund data.", long_about = None)]
pub struct Cli {
    #[command(subcommand)]
    pub command: Commands,

    /// Skip interactive confirmation
    #[arg(short, long, global = true, default_value_t = false)]
    pub yes: bool,
}

#[derive(Subcommand)]
pub enum Commands {
    /// Wallet management commands
    Wallet {
        #[command(subcommand)]
        command: WalletCommands,
    },
    /// Fund management commands
    Fund {
        #[command(subcommand)]
        command: FundCommands,
    },
    /// Show current portfolio status
    #[command(
        long_about = "Show current portfolio status including valuation, cost, and P&L.\n\nEXAMPLES:\n    fund status\n    fund status 000300"
    )]
    Status {
        /// Fund code or name (optional, shows all if omitted)
        fund: Option<String>,
    },
    /// Show transaction history
    #[command(
        long_about = "Show transaction history for a specific fund.\n\nEXAMPLES:\n    fund history 000300"
    )]
    History {
        /// Fund code or name
        fund: String,
    },
    /// Buy a fund
    #[command(
        long_about = "Record a buy transaction.\n\nEXAMPLES:\n    fund buy 000300 --money 1000\n    fund buy 000300 --money 1000 --date 2024-01-01\n    fund buy 000300 --shares 800 --nav 1.25 --date 2024-01-01"
    )]
    Buy {
        /// Fund code or name
        fund: String,
        /// Money amount to spend
        #[arg(long)]
        money: Decimal,
        /// Explicit shares (required if not using auto mode)
        #[arg(long)]
        shares: Option<Decimal>,
        /// Explicit NAV (required if not using auto mode)
        #[arg(long)]
        nav: Option<Decimal>,
        /// Specific wallet to use for this transaction
        #[arg(long)]
        wallet: Option<String>,
        /// Transaction date (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// Sell a fund
    #[command(
        long_about = "Record a sell transaction.\n\nEXAMPLES:\n    fund sell 000300 --shares 500\n    fund sell 000300 --money 1000 --date 2024-01-01\n    fund sell 000300 --shares 1/2 --nav 1.25 --date 2024-01-01"
    )]
    Sell {
        /// Fund code or name
        fund: String,
        /// Money amount to receive (e.g. "1000")
        #[arg(long)]
        money: Option<String>,
        /// Explicit shares to sell (e.g. "500", "1/2", "all")
        #[arg(long)]
        shares: Option<String>,
        /// Explicit NAV (e.g. "1.23")
        #[arg(long)]
        nav: Option<String>,
        /// Redemption fee (e.g. "5.0", "0.5%")
        #[arg(long)]
        fee: Option<String>,
        /// Specific wallet to use for this transaction
        #[arg(long)]
        wallet: Option<String>,
        /// Transaction date (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
    /// Import funds from CSV or arguments
    #[command(
        long_about = "Bulk import fund holdings.\n\nEXAMPLES:\n    fund import --file data.csv\n    fund import 000300 5000 000001 2000 --date 2024-01-01\n\nCSV FORMAT:\n    name,money[,date]\n    沪深300,5000,2024-01-01"
    )]
    Import {
        /// CSV file path to import from
        #[arg(long)]
        file: Option<std::path::PathBuf>,

        /// Positional pairs of [NAME MONEY]...
        #[arg(num_args(0..))]
        pairs: Vec<String>,

        /// Merge with existing holdings (default behavior)
        #[arg(long, conflicts_with = "override_flag")]
        merge: bool,

        /// Override existing holdings completely
        #[arg(long = "override")]
        override_flag: bool,

        /// Specific wallet to use for this transaction
        #[arg(long)]
        wallet: Option<String>,

        /// Global transaction date for this import (YYYY-MM-DD)
        #[arg(long)]
        date: Option<String>,
    },
}

#[derive(Subcommand)]
pub enum WalletCommands {
    /// Add a new wallet
    #[command(
        long_about = "Add a new wallet to manage multiple portfolios.\n\nEXAMPLES:\n    fund wallet add MyInvestments"
    )]
    Add {
        /// Name of the wallet
        name: String,
    },
    /// List all wallets
    List,
    /// Use a specific wallet as active
    #[command(
        long_about = "Set a wallet as the active one for future commands.\n\nEXAMPLES:\n    fund wallet use MyInvestments"
    )]
    Use {
        /// Name of the wallet to use
        name: String,
    },
}

#[derive(Subcommand)]
pub enum FundCommands {
    /// Add a new fund to track
    #[command(
        long_about = "Manually add a fund to the tracking list.\n\nEXAMPLES:\n    fund fund add 000300 沪深300 --fee 0.0015"
    )]
    Add {
        /// Fund code
        code: String,
        /// Fund name
        name: String,
        /// Initial fee rate (e.g. 0.0015 for 0.15%)
        #[arg(long, default_value = "0.00")]
        fee: String,
    },
    /// Delete a fund and all its data
    #[command(
        long_about = "Remove a fund and all its transaction history from local database.\n\nEXAMPLES:\n    fund fund delete 000300"
    )]
    Delete {
        /// Fund code or name
        fund: String,
    },
    /// List all tracked funds
    List,
    /// Sync fund metadata and history from remote
    #[command(
        long_about = "Sync fund metadata and net value history.\n\nEXAMPLES:\n    fund fund sync\n    fund fund sync 000300 --start 2024-01-01\n    fund fund sync --auto-fill"
    )]
    Sync {
        /// Force sync metadata even if not expired
        #[arg(long, default_value_t = false)]
        force: bool,
        /// Start date for history backfill (YYYY-MM-DD)
        #[arg(long)]
        start: Option<String>,
        /// End date for history backfill (YYYY-MM-DD)
        #[arg(long)]
        end: Option<String>,
        /// Automatically detect and fill gaps in history
        #[arg(long, default_value_t = false)]
        auto_fill: bool,
        /// Sync all funds: metadata + last 30 days NAV history
        #[arg(long, short, default_value_t = false)]
        all: bool,
        /// Specific fund to sync (required if not using --all)
        fund: Option<String>,
    },
    /// Inspect a fund's health and performance (Morningstar)
    #[command(
        long_about = "Show detailed health report for a fund from Morningstar analysis.\n\nEXAMPLES:\n    fund fund inspect 000513\n    fund fund inspect \"汇添富全球医疗\""
    )]
    Inspect {
        /// Fund code or name
        fund: String,
        /// Force refresh analysis data
        #[arg(short, long, default_value_t = false)]
        force: bool,
    },
}
