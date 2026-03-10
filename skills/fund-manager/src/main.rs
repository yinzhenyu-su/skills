mod config;
mod db;
mod cli;
mod finance;
mod provider;

#[cfg(test)]
mod db_tests;

use clap::Parser;
use cli::{Cli, Commands, WalletCommands};
use rusqlite::Connection;
use std::fs;
use std::env;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::str::FromStr;
use comfy_table::Table;
use provider::aggregator::Aggregator;
use provider::eastmoney_js::EastmoneyJsProvider;
use provider::eastmoney_html::EastmoneyHtmlProvider;

async fn sync_funds(conn: &Connection) -> Result<(), String> {
    if env::var("FORCE_SYNC_FAILURE").is_ok() {
        return Err("Network unreachable".to_string());
    }

    let mut aggregator = Aggregator::new();
    aggregator.add_provider(Box::new(EastmoneyJsProvider));
    aggregator.add_provider(Box::new(EastmoneyHtmlProvider));

    let mut stmt = conn.prepare("SELECT code FROM fund").map_err(|e| e.to_string())?;
    let codes: Vec<String> = stmt.query_map([], |row| row.get(0)).map_err(|e| e.to_string())?
        .map(|r| r.unwrap()).collect();

    for code in codes {
        if let Ok(data) = aggregator.fetch_all(&code).await {
            if let Some(nav) = data.nav {
                let date = data.date.unwrap_or_else(|| "2026-03-09".to_string());
                db::insert_nav_history_idempotent(conn, &code, &date, &nav.to_string()).map_err(|e| e.to_string())?;
            }
            if let Some(fee) = data.fee_rate {
                conn.execute("UPDATE fund SET current_fee_rate = ?1 WHERE code = ?2", [fee.to_string(), code]).map_err(|e| e.to_string())?;
            }
        }
    }

    Ok(())
}

fn resolve_wallet_id(conn: &Connection, wallet_name: Option<String>) -> i64 {
    if let Some(name) = wallet_name {
        db::get_wallet_id_by_name(conn, &name).expect("DB error")
            .unwrap_or_else(|| {
                eprintln!("Error: Wallet '{}' not found.", name);
                std::process::exit(1);
            })
    } else {
        db::get_active_wallet_id(conn).expect("DB error")
            .expect("No active wallet selected. Use 'fund wallet use <name>' or specify --wallet.")
    }
}

fn confirm_action(prompt: &str, force_yes: bool) -> bool {
    if force_yes {
        println!("{} [y/N]: y (Skipping confirmation due to -y/--yes)", prompt);
        return true;
    }

    print!("{} [y/N]: ", prompt);
    use std::io::{self, Write};
    io::stdout().flush().unwrap();
    
    let mut input = String::new();
    io::stdin().read_line(&mut input).expect("Failed to read input");
    
    input.trim().to_lowercase() == "y"
}

fn is_fund_code(input: &str) -> bool {
    input.len() == 6 && input.chars().all(|c| c.is_ascii_digit())
}

async fn find_or_create_fund(conn: &Connection, fund_input: &str) -> db::Fund {
    if let Some(f) = db::get_fund_by_code_or_name(conn, fund_input).expect("DB error") {
        return f;
    }

    if is_fund_code(fund_input) {
        println!("✨ Fund '{}' not found locally. Attempting to fetch from Eastmoney...", fund_input);
        let mut aggregator = Aggregator::new();
        aggregator.add_provider(Box::new(EastmoneyJsProvider));
        aggregator.add_provider(Box::new(EastmoneyHtmlProvider));

        match aggregator.fetch_all(fund_input).await {
            Ok(data) => {
                let name = data.name.unwrap_or_else(|| "Unknown Fund".to_string());
                let fee_rate = data.fee_rate.map(|f| f.to_string()).unwrap_or_else(|| "0.00".to_string());
                db::add_fund(conn, &data.code, &name, &fee_rate).expect("Failed to add fund");
                if let Some(nav) = data.nav {
                    let date = data.date.unwrap_or_else(|| "2026-03-09".to_string());
                    db::insert_nav_history_idempotent(conn, &data.code, &date, &nav.to_string()).expect("DB error");
                }
                println!("Successfully created new fund: {} ({})", name, data.code);
                db::get_fund_by_code_or_name(conn, &data.code).expect("DB error").unwrap()
            }
            Err(e) => {
                eprintln!("Error: Could not fetch fund info for '{}': {}", fund_input, e);
                std::process::exit(1);
            }
        }
    } else {
        eprintln!("Error: Fund '{}' not found and is not a valid 6-digit code.", fund_input);
        std::process::exit(1);
    }
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    let app_dir = config::get_app_dir();
    fs::create_dir_all(&app_dir).expect("Failed to create app directory");
    
    let db_path = config::get_db_path();
    db::init_db(&db_path).expect("Failed to initialize database");
    
    let conn = Connection::open(&db_path).expect("Failed to open database");

    match cli.command {
        Commands::Wallet { command } => match command {
            WalletCommands::Add { name } => {
                match db::add_wallet(&conn, &name) {
                    Ok(_) => println!("Successfully added wallet: {}", name),
                    Err(e) => {
                        if e.to_string().contains("UNIQUE constraint failed") {
                            eprintln!("Error: Wallet '{}' already exists.", name);
                            std::process::exit(1);
                        } else {
                            eprintln!("Error adding wallet: {}", e);
                            std::process::exit(1);
                        }
                    }
                }
            }
            WalletCommands::List => {
                let wallets = db::get_all_wallets(&conn).expect("DB error");
                let active_id = db::get_active_wallet_id(&conn).expect("DB error");
                
                let mut table = Table::new();
                table.set_header(vec!["", "Name", "Valuation", "Cost", "Total P&L", "P&L %"]);

                for w in wallets {
                    let holdings = db::get_holdings(&conn, w.id).expect("DB error");
                    let mut total_valuation = Decimal::ZERO;
                    let mut total_cost = Decimal::ZERO;

                    for h in holdings {
                        let shares = Decimal::from_str(&h.total_shares).unwrap_or_default();
                        let cost = Decimal::from_str(&h.net_cost).unwrap_or_default();
                        let nav = h.latest_nav.as_deref().and_then(|s| Decimal::from_str(s).ok()).unwrap_or_default();
                        
                        total_valuation += (shares * nav).round_dp(2);
                        total_cost += cost;
                    }

                    let pl = (total_valuation - total_cost).round_dp(2);
                    let pl_pct = if total_cost.is_zero() { dec!(0.00) } else { ((pl / total_cost) * dec!(100)).round_dp(2) };

                    let active_mark = if Some(w.id) == active_id { "*" } else { "" };

                    table.add_row(vec![
                        active_mark.to_string(),
                        w.name,
                        total_valuation.to_string(),
                        total_cost.to_string(),
                        pl.to_string(),
                        format!("{:.2}%", pl_pct),
                    ]);
                }
                println!("{table}");
            }
            WalletCommands::Use { name } => {
                match db::get_wallet_id_by_name(&conn, &name) {
                    Ok(Some(id)) => {
                        db::set_active_wallet(&conn, id).expect("Failed to set active wallet");
                        println!("Now using wallet: {}", name);
                    }
                    Ok(None) => {
                        eprintln!("Error: Wallet '{}' does not exist.", name);
                        std::process::exit(1);
                    }
                    Err(e) => {
                        eprintln!("Error finding wallet: {}", e);
                        std::process::exit(1);
                    }
                }
            }
        },
        Commands::Fund { command } => match command {
            cli::FundCommands::Add { code, name, fee } => {
                db::add_fund(&conn, &code, &name, &fee).expect("Failed to add fund");
                println!("Successfully added fund: {} ({})", name, code);
            }
            cli::FundCommands::Delete { fund } => {
                let fund_obj = db::get_fund_by_code_or_name(&conn, &fund).expect("DB error")
                    .unwrap_or_else(|| {
                        eprintln!("Error: Fund '{}' not found.", fund);
                        std::process::exit(1);
                    });
                
                let prompt = format!("Are you sure you want to delete fund {} and ALL its transaction history?", fund_obj.code);
                if confirm_action(&prompt, cli.yes) {
                    db::delete_fund(&conn, &fund_obj.code).expect("Failed to delete fund");
                    println!("Successfully deleted fund: {}", fund_obj.code);
                } else {
                    println!("Deletion cancelled.");
                }
            }
            cli::FundCommands::List => {
                let funds = db::get_all_funds(&conn).expect("DB error");
                let mut table = Table::new();
                table.set_header(vec!["Code", "Name", "Fee Rate", "Last Sync"]);
                for f in funds {
                    table.add_row(vec![
                        f.code,
                        f.name,
                        f.current_fee_rate.unwrap_or_else(|| "N/A".to_string()),
                        f.last_sync_at.unwrap_or_else(|| "Never".to_string()),
                    ]);
                }
                println!("{table}");
            }
        },
        Commands::Status { fund: _ } => {
            if let Err(e) = sync_funds(&conn).await {
                eprintln!("Warning: Could not fetch latest data: {}", e);
                eprintln!("Showing cached data from local database.");
            }
            
            let wallet_id = db::get_active_wallet_id(&conn).expect("DB error")
                .expect("No active wallet selected.");
            
            let holdings = db::get_holdings(&conn, wallet_id).expect("DB error");
            
            let mut table = Table::new();
            table.set_header(vec!["Fund", "Shares", "Cost", "NAV", "Valuation", "P&L", "P&L %"]);

            for h in holdings {
                let shares = Decimal::from_str(&h.total_shares).unwrap_or_default();
                let cost = Decimal::from_str(&h.net_cost).unwrap_or_default();
                let nav = h.latest_nav.as_deref().and_then(|s| Decimal::from_str(s).ok()).unwrap_or_default();
                
                let valuation = (shares * nav).round_dp(2);
                let pl = (valuation - cost).round_dp(2);
                let pl_pct = if cost.is_zero() { dec!(0.00) } else { ((pl / cost) * dec!(100)).round_dp(2) };

                table.add_row(vec![
                    format!("{} ({})", h.fund_name, h.fund_code),
                    shares.to_string(),
                    cost.to_string(),
                    nav.to_string(),
                    valuation.to_string(),
                    pl.to_string(),
                    format!("{:.2}%", pl_pct),
                ]);
            }
            println!("{table}");
        }
        Commands::History { fund: _ } => {
            println!("Transaction History (to be implemented)");
        }
        Commands::Buy { fund, money, auto, shares, nav, wallet } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            let fund_obj = find_or_create_fund(&conn, &fund).await;

            let (final_shares, final_nav, final_fee) = if auto {
                let latest_nav_str = db::get_latest_nav(&conn, &fund_obj.code).expect("DB error")
                    .expect("No NAV data found for this fund. Cannot use --auto.");
                let latest_nav = Decimal::from_str(&latest_nav_str).expect("Invalid NAV in DB");
                let fee_rate = fund_obj.current_fee_rate.as_deref().unwrap_or("0.00");
                let fee_rate_dec = Decimal::from_str(fee_rate).unwrap_or_default();
                
                let res = finance::calculate_purchase(money, latest_nav, fee_rate_dec);
                (res.shares, latest_nav, res.fee)
            } else {
                let s = shares.expect("Must provide --shares if not using --auto");
                let n = nav.expect("Must provide --nav if not using --auto");
                (s, n, Decimal::ZERO)
            };

            let today = "2026-03-09"; // Dummy date for now
            db::add_transaction(
                &conn, 
                wallet_id, 
                &fund_obj.code, 
                "buy", 
                &money.to_string(), 
                &final_shares.to_string(), 
                &final_nav.to_string(), 
                &final_fee.to_string(), 
                today
            ).expect("Failed to record transaction");

            println!("Bought {}: {} shares (NAV: {}, Fee: {})", fund_obj.code, final_shares, final_nav, final_fee);
        }
        Commands::Sell { fund, money, auto, shares, nav, fee, wallet } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            
            let fund_obj = db::get_fund_by_code_or_name(&conn, &fund).expect("DB error")
                .unwrap_or_else(|| {
                    eprintln!("Error: Fund '{}' not found in database. You cannot sell a fund you've never owned or tracked.", fund);
                    std::process::exit(1);
                });

            let current_shares = db::get_fund_shares(&conn, wallet_id, &fund_obj.code).expect("DB error");

            // 1. Resolve NAV
            let final_nav = if let Some(n_str) = nav {
                Decimal::from_str(&n_str).expect("Invalid --nav")
            } else {
                let latest_nav_str = db::get_latest_nav(&conn, &fund_obj.code).expect("DB error")
                    .expect("No NAV data found for this fund. Please provide --nav.");
                Decimal::from_str(&latest_nav_str).expect("Invalid NAV in DB")
            };

            // 2. Resolve Shares
            let final_shares = if let Some(s_input) = shares {
                finance::resolve_shares(&s_input, current_shares).expect("Invalid --shares")
            } else if let Some(m_str) = money {
                let m = Decimal::from_str(&m_str).expect("Invalid --money");
                (m / final_nav).round_dp(2)
            } else if auto {
                // If auto is set but no money/shares, we might assume money is needed? 
                // Let's assume --auto means use latest NAV if --nav is missing (handled).
                panic!("Please provide --shares or --money for sell command.");
            } else {
                panic!("Please provide --shares or --money for sell command.");
            };

            if final_shares > current_shares {
                eprintln!("Error: Insufficient shares. Current: {}, Requested: {}", current_shares, final_shares);
                std::process::exit(1);
            }

            // 3. Resolve Fee
            let total_money = final_shares * final_nav;
            let final_fee = if let Some(f_input) = fee {
                finance::resolve_fee(&f_input, total_money).expect("Invalid --fee")
            } else {
                Decimal::ZERO
            };

            let received_money = total_money - final_fee;

            // 4. Preview and Confirm
            println!("┌─────────────────────────────────────────┐");
            println!("│           赎回操作预览 (PREVIEW)          │");
            println!("├─────────────────────────────────────────┤");
            println!("│ 基金: {} ({})", fund_obj.name, fund_obj.code);
            println!("│ 份额: {} 份 (剩余: {})", final_shares, current_shares - final_shares);
            println!("│ 净值: {} ", final_nav);
            println!("├─────────────────────────────────────────┤");
            println!("│ 预计金额: ￥{}", total_money);
            println!("│ 赎回费用: ￥{}", final_fee);
            println!("│ 实际到账: ￥{}", received_money);
            println!("└─────────────────────────────────────────┘");
            
            if confirm_action("确认记录此笔交易？", cli.yes) {
                let today = "2026-03-09"; // Dummy date
                db::add_transaction(
                    &conn, 
                    wallet_id, 
                    &fund_obj.code, 
                    "sell", 
                    &received_money.to_string(), 
                    &final_shares.to_string(), 
                    &final_nav.to_string(), 
                    &final_fee.to_string(), 
                    today
                ).expect("Failed to record transaction");

                println!("Sold {}: {} shares", fund_obj.code, final_shares);
            } else {
                println!("Transaction cancelled.");
            }

        }
    }
}
