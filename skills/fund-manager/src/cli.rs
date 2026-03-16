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
    Status {
        /// Fund code or name (optional, shows all if omitted)
        fund: Option<String>,
    },
    /// Show transaction history
    History {
        /// Fund code or name
        fund: String,
    },
    /// Buy a fund
    Buy {
        /// Fund code or name
        fund: String,
        /// Money amount to spend
        #[arg(long)]
        money: Decimal,
        /// Automatically calculate shares based on latest NAV and fee
        #[arg(long, default_value_t = false)]
        auto: bool,
        /// Explicit shares (if not using --auto)
        #[arg(long)]
        shares: Option<Decimal>,
        /// Explicit NAV (if not using --auto)
        #[arg(long)]
        nav: Option<Decimal>,
        /// Specific wallet to use for this transaction
        #[arg(long)]
        wallet: Option<String>,
    },
    /// Sell a fund
    Sell {
        /// Fund code or name
        fund: String,
        /// Money amount to receive (e.g. "1000")
        #[arg(long)]
        money: Option<String>,
        /// Automatically calculate money based on shares and latest NAV
        #[arg(long, default_value_t = false)]
        auto: bool,
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
    },
}

#[derive(Subcommand)]
pub enum WalletCommands {
    /// Add a new wallet
    Add {
        /// Name of the wallet
        name: String,
    },
    /// List all wallets
    List,
    /// Use a specific wallet as active
    Use {
        /// Name of the wallet to use
        name: String,
    },
}

#[derive(Subcommand)]
pub enum FundCommands {
    /// Add a new fund to track
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
    Delete {
        /// Fund code or name
        fund: String,
    },
    /// List all tracked funds
    List,
    /// Sync fund metadata from remote
    Sync {
        /// Force sync even if not expired
        #[arg(long, default_value_t = false)]
        force: bool,
        /// Specific fund to sync (syncs all if omitted)
        fund: Option<String>,
    },
}
