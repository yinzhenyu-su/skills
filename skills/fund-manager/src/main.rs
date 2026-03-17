mod cli;
mod config;
mod db;
mod finance;
mod provider;
mod resolver;
mod sync;

#[cfg(test)]
mod db_tests;

use clap::Parser;
use cli::{Cli, Commands, WalletCommands};
use comfy_table::Table;
use csv::ReaderBuilder;
use provider::eastmoney_lsjz::EastmoneyLsjzProvider;
use rusqlite::Connection;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::fs;
use std::path::PathBuf;
use std::str::FromStr;

use chrono::Local;

struct ImportItem {
    raw_input: String,
    money: Decimal,
    line_num: Option<usize>,
    date: String,
}

struct ImportResult {
    input: String,
    success: bool,
    reason: String,
}

fn get_today() -> String {
    Local::now().format("%Y-%m-%d").to_string()
}

fn parse_date(date_str: &str) -> Result<String, String> {
    use chrono::NaiveDate;
    NaiveDate::parse_from_str(date_str, "%Y-%m-%d")
        .map(|d| d.format("%Y-%m-%d").to_string())
        .map_err(|e| {
            format!(
                "Invalid date format '{}': {}. Expected YYYY-MM-DD",
                date_str, e
            )
        })
}

/// Smart NAV lookup: local DB -> API -> forward lookup
/// Returns (actual_date, nav) or None if not found
async fn smart_nav_lookup(
    conn: &Connection,
    code: &str,
    requested_date: &str,
) -> Result<Option<(String, Decimal)>, String> {
    // 1. Try local DB for exact date first
    if let Ok(Some(nav)) = db::get_nav_at_date(conn, code, requested_date) {
        return Ok(Some((requested_date.to_string(), nav)));
    }

    // 2. Local not found, try API
    let lsjz = EastmoneyLsjzProvider;
    match lsjz.fetch_by_date(code, requested_date).await {
        Ok(data) => {
            if let (Some(nav), Some(date)) = (data.nav, data.date) {
                // Save to local DB
                db::insert_nav_history_idempotent(conn, code, &date, &nav.to_string())
                    .map_err(|e| e.to_string())?;
                return Ok(Some((date, nav)));
            }
        }
        Err(e) => {
            println!(
                "Note: API fetch failed for {} on {}, trying local forward lookup: {}",
                code, requested_date, e
            );
        }
    }

    // 3. API also failed, fall back to forward lookup (up to 20 days)
    let nav_result =
        db::find_next_available_nav(conn, code, requested_date, 20).map_err(|e| e.to_string())?;

    Ok(nav_result)
}

fn resolve_wallet_id(conn: &Connection, wallet_name: Option<String>) -> i64 {
    if let Some(name) = wallet_name {
        db::get_wallet_id_by_name(conn, &name)
            .expect("DB error")
            .unwrap_or_else(|| {
                eprintln!("Error: Wallet '{}' not found.", name);
                std::process::exit(1);
            })
    } else {
        db::get_active_wallet_id(conn)
            .expect("DB error")
            .expect("No active wallet selected. Use 'fund wallet use <name>' or specify --wallet.")
    }
}

fn confirm_action(prompt: &str, force_yes: bool) -> bool {
    if force_yes {
        println!(
            "{} [y/N]: y (Skipping confirmation due to -y/--yes)",
            prompt
        );
        return true;
    }

    print!("{} [y/N]: ", prompt);
    use std::io::{self, Write};
    io::stdout().flush().unwrap();

    let mut input = String::new();
    io::stdin()
        .read_line(&mut input)
        .expect("Failed to read input");

    input.trim().to_lowercase() == "y"
}

async fn handle_inspect(conn: &Connection, identifier: &str, force: bool) -> Result<(), String> {
    let fund = resolver::resolve_fund(conn, identifier, true, false).await?;

    let analysis_opt = db::get_fund_analysis(conn, &fund.code).map_err(|e| e.to_string())?;

    let needs_update = if force {
        true
    } else if let Some(ref a) = analysis_opt {
        // Update if older than 30 days
        if let Ok(last) = chrono::NaiveDateTime::parse_from_str(&a.last_update, "%Y-%m-%d %H:%M:%S") {
            (chrono::Utc::now().naive_utc() - last).num_days() > 30
        } else {
            true
        }
    } else {
        true
    };

    let final_analysis = if needs_update {
        println!(
            "🔄 Fetching deep analysis for {} ({}) from Morningstar...",
            fund.name, fund.code
        );
        let updated_fund = resolver::sync_fund_details(conn, &fund.code).await?;
        db::get_fund_analysis(conn, &updated_fund.code).map_err(|e| e.to_string())?
    } else {
        analysis_opt
    };

    if let Some(a) = final_analysis {
        format_inspect_report(&fund, &a);
    } else {
        println!("❌ No analysis data available for this fund.");
    }

    Ok(())
}

fn format_inspect_report(fund: &db::Fund, analysis: &db::FundAnalysis) {
    let mut table = Table::new();
    table.set_header(vec!["Metric", "Value", "Description"]);

    table.add_row(vec![
        "Fund".to_string(),
        format!("{} ({})", fund.name, fund.code),
        "Official name and code".to_string(),
    ]);

    table.add_row(vec![
        "Morningstar Category".to_string(),
        fund.fund_type.clone().unwrap_or_else(|| "N/A".to_string()),
        "Investment style".to_string(),
    ]);

    let rating_3y = analysis
        .rating_3y
        .map(|r| "★".repeat(r as usize))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Rating (3Y)".to_string(),
        rating_3y,
        "Morningstar 3-year comprehensive rating".to_string(),
    ]);

    let rank = analysis
        .rank_pct_3y
        .map(|r| format!("{:.2}%", r))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Category Rank (3Y)".to_string(),
        rank,
        "Percentage rank in category (lower is better)".to_string(),
    ]);

    let sharpe = analysis
        .sharpe_3y
        .map(|s| format!("{:.2}", s))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Sharpe Ratio (3Y)".to_string(),
        sharpe,
        "Risk-adjusted return (higher is better)".to_string(),
    ]);

    let calmar = analysis
        .calmar_3y
        .map(|c| format!("{:.2}", c))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Calmar Ratio (3Y)".to_string(),
        calmar,
        "Return over maximum drawdown (higher is better)".to_string(),
    ]);

    let mdd = analysis
        .max_drawdown_3y
        .map(|m| format!("{:.2}%", m))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Max Drawdown (3Y)".to_string(),
        mdd,
        "Worst historical peak-to-trough decline".to_string(),
    ]);

    let gap = analysis
        .investor_gap_3y
        .map(|g| format!("{:.2}%", g))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "Investor Gap (3Y)".to_string(),
        gap,
        "Diff between fund return and avg investor return".to_string(),
    ]);

    println!("\n📊 Fund Health Report (Morningstar Data)");
    println!("{table}");

    if let Some(g) = analysis.investor_gap_3y {
        if g > 5.0 {
            println!(
                "💡 Insight: High investor gap (>{:.1}%) suggests this fund is highly volatile,",
                g
            );
            println!("   and investors often lose money due to bad timing (buying high, selling low).");
        }
    }
}

async fn handle_import(
    conn: &Connection,
    wallet_id: i64,
    file: Option<PathBuf>,
    pairs: Vec<String>,
    merge: bool,
    override_flag: bool,
    global_date: Option<String>,
) -> Result<(), String> {
    let mut items = Vec::new();
    let mut results = Vec::new();

    // 1. Parse Input
    if let Some(path) = file {
        let mut rdr = ReaderBuilder::new()
            .has_headers(true)
            .from_path(path)
            .map_err(|e| format!("Failed to open CSV: {}", e))?;

        for (i, result) in rdr.records().enumerate() {
            let line_num = i + 2;
            match result {
                Ok(record) => {
                    if record.len() < 2 {
                        results.push(ImportResult {
                            input: format!("CSV Line {}", line_num),
                            success: false,
                            reason: "Row has fewer than 2 columns".to_string(),
                        });
                        continue;
                    }
                    let raw_input = record.get(0).unwrap().to_string();
                    let money_str = record.get(1).unwrap();

                    // Priority: CSV column > Global flag > Today
                    let item_date = if let Some(d_str) = record.get(2) {
                        match parse_date(d_str) {
                            Ok(d) => d,
                            Err(e) => {
                                results.push(ImportResult {
                                    input: raw_input,
                                    success: false,
                                    reason: e,
                                });
                                continue;
                            }
                        }
                    } else if let Some(ref gd) = global_date {
                        gd.clone()
                    } else {
                        get_today()
                    };

                    match Decimal::from_str(money_str) {
                        Ok(money) => {
                            if money <= Decimal::ZERO {
                                results.push(ImportResult {
                                    input: raw_input,
                                    success: false,
                                    reason: format!("Amount must be positive: {}", money_str),
                                });
                            } else {
                                items.push(ImportItem {
                                    raw_input,
                                    money,
                                    line_num: Some(line_num),
                                    date: item_date,
                                });
                            }
                        }
                        Err(e) => {
                            results.push(ImportResult {
                                input: raw_input,
                                success: false,
                                reason: format!("Invalid money format: {} ({})", money_str, e),
                            });
                        }
                    }
                }
                Err(e) => {
                    results.push(ImportResult {
                        input: format!("CSV Line {}", line_num),
                        success: false,
                        reason: format!("CSV error: {}", e),
                    });
                }
            }
        }
    } else if !pairs.is_empty() {
        if pairs.len() % 2 != 0 {
            return Err("Positional arguments must be in pairs of [NAME MONEY]".to_string());
        }
        let date = if let Some(ref gd) = global_date {
            gd.clone()
        } else {
            get_today()
        };
        for chunk in pairs.chunks(2) {
            let raw_input = chunk[0].clone();
            let money_str = &chunk[1];
            match Decimal::from_str(money_str) {
                Ok(money) => {
                    if money <= Decimal::ZERO {
                        results.push(ImportResult {
                            input: raw_input,
                            success: false,
                            reason: format!("Amount must be positive: {}", money_str),
                        });
                    } else {
                        items.push(ImportItem {
                            raw_input,
                            money,
                            line_num: None,
                            date: date.clone(),
                        });
                    }
                }
                Err(e) => {
                    results.push(ImportResult {
                        input: raw_input,
                        success: false,
                        reason: format!("Invalid money format: {} ({})", money_str, e),
                    });
                }
            }
        }
    } else {
        return Err("Please provide either --file or positional name/money pairs".to_string());
    }

    if items.is_empty() && results.is_empty() {
        println!("No items found to import.");
        return Ok(());
    }

    // 2. Main Loop
    for item in items {
        let mut result = ImportResult {
            input: item.raw_input.clone(),
            success: false,
            reason: String::new(),
        };

        // Delay to avoid rate limiting
        tokio::time::sleep(tokio::time::Duration::from_millis(200)).await;

        // Resolve fund (non-interactive)
        match resolver::resolve_fund(conn, &item.raw_input, false, false).await {
            Ok(fund_obj) => {
                // Check existing holdings
                let current_shares =
                    db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
                let already_exists = !current_shares.is_zero();

                if already_exists && !merge && !override_flag {
                    result.reason =
                        "Fund already exists in wallet. Use --merge or --override.".to_string();
                } else {
                    if override_flag && already_exists {
                        conn.execute(
                            "DELETE FROM transaction_log WHERE wallet_id = ?1 AND fund_code = ?2",
                            rusqlite::params![wallet_id, fund_obj.code],
                        )
                        .map_err(|e| e.to_string())?;
                    }

                    // Handle settlement logic with smart NAV lookup (local DB -> API -> forward lookup)
                    let nav_result = smart_nav_lookup(conn, &fund_obj.code, &item.date)
                        .await
                        .map_err(|e| e.to_string())?;

                    if let Some((actual_date, nav)) = nav_result {
                        let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                            if !sf.trim().is_empty() && sf != "0.00%" {
                                finance::parse_percentage_rate(sf)
                            } else {
                                dec!(0.0015)
                            }
                        } else {
                            dec!(0.0015)
                        };

                        let res = finance::calculate_purchase(item.money, nav, fee_rate_dec);
                        db::add_transaction(
                            conn,
                            wallet_id,
                            &fund_obj.code,
                            "import",
                            &item.money.to_string(),
                            Some(&res.shares.to_string()),
                            Some(&nav.to_string()),
                            &res.fee.to_string(),
                            &actual_date,
                            "settled",
                        )
                        .map_err(|e| e.to_string())?;
                    } else {
                        // No NAV available within 20 days, create pending transaction
                        let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                            finance::parse_percentage_rate(sf)
                        } else {
                            dec!(0.0015)
                        };
                        let fee =
                            (item.money * fee_rate_dec / (dec!(1) + fee_rate_dec)).round_dp(2);

                        db::add_transaction(
                            conn,
                            wallet_id,
                            &fund_obj.code,
                            "import",
                            &item.money.to_string(),
                            None,
                            None,
                            &fee.to_string(),
                            &item.date,
                            "pending",
                        )
                        .map_err(|e| e.to_string())?;
                    }
                    result.success = true;
                }
            }
            Err(e) => {
                result.reason = e;
            }
        }
        results.push(result);
    }

    // 3. Print Report
    let success_count = results.iter().filter(|r| r.success).count();
    let fail_count = results.len() - success_count;

    println!("\n🚀 Import process finished!");
    println!("✅ Success: {}", success_count);
    println!("❌ Failed:  {}", fail_count);

    if fail_count > 0 {
        let mut table = Table::new();
        table.set_header(vec!["Input", "Status", "Reason"]);
        for r in results.iter().filter(|r| !r.success) {
            table.add_row(vec![
                r.input.clone(),
                "FAILED".to_string(),
                r.reason.clone(),
            ]);
        }
        println!("\nFailure Details:");
        println!("{table}");
    }

    Ok(())
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
            WalletCommands::Add { name } => match db::add_wallet(&conn, &name) {
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
            },
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
                        let nav = h
                            .latest_nav
                            .as_deref()
                            .and_then(|s| Decimal::from_str(s).ok())
                            .unwrap_or_default();

                        total_valuation += (shares * nav).round_dp(2);
                        total_cost += cost;
                    }

                    let pl = (total_valuation - total_cost).round_dp(2);
                    let pl_pct = if total_cost.is_zero() {
                        dec!(0.00)
                    } else {
                        ((pl / total_cost) * dec!(100)).round_dp(2)
                    };

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
            WalletCommands::Use { name } => match db::get_wallet_id_by_name(&conn, &name) {
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
            },
        },
        Commands::Fund { command } => match command {
            cli::FundCommands::Add { code, name, fee } => {
                db::add_fund(
                    &conn,
                    &code,
                    &name,
                    None,
                    None,
                    None,
                    None,
                    None,
                    Some(&fee),
                    None,
                    None,
                    None,
                )
                .expect("Failed to add fund");
                println!("Successfully added fund: {} ({})", name, code);
            }
            cli::FundCommands::Delete { fund } => {
                let fund_obj = resolver::resolve_fund(&conn, &fund, true, true)
                    .await
                    .unwrap_or_else(|e| {
                        eprintln!("Error resolving fund: {}", e);
                        std::process::exit(1);
                    });

                let prompt = format!(
                    "Are you sure you want to delete fund {} and ALL its transaction history?",
                    fund_obj.code
                );
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
                table.set_header(vec!["Code", "Name", "Type", "Risk", "Manager", "Last Sync"]);
                for f in funds {
                    table.add_row(vec![
                        f.code,
                        f.name,
                        f.fund_type.unwrap_or_else(|| "N/A".to_string()),
                        f.risk_level.unwrap_or_else(|| "-".to_string()),
                        f.manager.unwrap_or_else(|| "-".to_string()),
                        f.last_sync_at.unwrap_or_else(|| "Never".to_string()),
                    ]);
                }
                println!("{table}");
            }
            cli::FundCommands::Sync {
                force: _,
                start,
                end,
                auto_fill,
                all,
                fund,
            } => {
                if !all && fund.is_none() {
                    eprintln!("Error: Please specify a fund identifier or use --all for full sync.");
                    std::process::exit(1);
                }

                let fund_code = if let Some(identifier) = fund {
                    match resolver::resolve_fund(&conn, &identifier, true, false).await {
                        Ok(f) => Some(f.code),
                        Err(e) => {
                            eprintln!("Error resolving fund: {}", e);
                            std::process::exit(1);
                        }
                    }
                } else {
                    None
                };

                if let Err(e) = sync::sync_funds(&conn, fund_code, start, end, auto_fill).await {
                    eprintln!("Error during sync: {}", e);
                    std::process::exit(1);
                }
                println!("✅ Sync completed.");
            }
            cli::FundCommands::Inspect { fund, force } => {
                if let Err(e) = handle_inspect(&conn, &fund, force).await {
                    eprintln!("Error: {}", e);
                    std::process::exit(1);
                }
            }
        },
        Commands::Status { fund: _ } => {
            if let Err(e) = sync::sync_funds(&conn, None, None, None, true).await {
                eprintln!("Warning: Could not fetch latest data: {}", e);
                eprintln!("Showing cached data from local database.");
            }

            let wallet_id = db::get_active_wallet_id(&conn)
                .expect("DB error")
                .expect("No active wallet selected.");

            let holdings = db::get_holdings(&conn, wallet_id).expect("DB error");

            let mut table = Table::new();
            table.set_header(vec![
                "Fund",
                "Shares",
                "Cost",
                "NAV",
                "Valuation",
                "P&L",
                "P&L %",
            ]);

            for h in holdings {
                let shares = Decimal::from_str(&h.total_shares).unwrap_or_default();
                let cost = Decimal::from_str(&h.net_cost).unwrap_or_default();
                let nav = h
                    .latest_nav
                    .as_deref()
                    .and_then(|s| Decimal::from_str(s).ok())
                    .unwrap_or_default();

                let valuation = (shares * nav).round_dp(2);
                let pl = (valuation - cost).round_dp(2);
                let pl_pct = if cost.is_zero() {
                    dec!(0.00)
                } else {
                    ((pl / cost) * dec!(100)).round_dp(2)
                };

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
        Commands::Buy {
            fund,
            money,
            shares,
            nav,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            let fund_obj = resolver::resolve_fund(&conn, &fund, true, false)
                .await
                .unwrap_or_else(|e| {
                    eprintln!("Error resolving fund: {}", e);
                    std::process::exit(1);
                });

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("Error: {}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            // Auto mode: use smart NAV lookup (local DB -> API -> forward lookup)
            if shares.is_none() && nav.is_none() {
                let nav_result = smart_nav_lookup(&conn, &fund_obj.code, &tx_date)
                    .await
                    .expect("Failed to lookup NAV");

                if let Some((actual_date, nav_val)) = nav_result {
                    // Purchase Fee priority: sales_fee -> 0.15% (default)
                    let (fee_rate_dec, fee_source) = if let Some(ref sf) = fund_obj.sales_fee {
                        if !sf.trim().is_empty() && sf != "0.00%" {
                            (finance::parse_percentage_rate(sf), format!("sales_fee: {}", sf))
                        } else {
                            (dec!(0.0015), "default: 0.15%".to_string())
                        }
                    } else {
                        (dec!(0.0015), "default: 0.15%".to_string())
                    };

                    let res = finance::calculate_purchase(money, nav_val, fee_rate_dec);
                    db::add_transaction(
                        &conn,
                        wallet_id,
                        &fund_obj.code,
                        "buy",
                        &money.to_string(),
                        Some(&res.shares.to_string()),
                        Some(&nav_val.to_string()),
                        &res.fee.to_string(),
                        &actual_date,
                        "settled",
                    )
                    .expect("Failed to record transaction");

                    if actual_date != tx_date {
                        println!(
                            "Note: NAV for {} not available, used {}",
                            tx_date, actual_date
                        );
                    }
                    println!(
                        "Bought {}: {} shares (NAV: {}, Fee: {} [{}]) on {}",
                        fund_obj.code, res.shares, nav_val, res.fee, fee_source, actual_date
                    );
                } else {
                    // NAV not available within 20 days, create pending transaction
                    let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                        finance::parse_percentage_rate(sf)
                    } else {
                        dec!(0.0015)
                    };
                    let fee = (money * fee_rate_dec / (dec!(1) + fee_rate_dec)).round_dp(2);

                    db::add_transaction(
                        &conn,
                        wallet_id,
                        &fund_obj.code,
                        "buy",
                        &money.to_string(),
                        None,
                        None,
                        &fee.to_string(),
                        &tx_date,
                        "pending",
                    )
                    .expect("Failed to record transaction");

                    println!(
                        "NAV not available within 20 days for {} on {}. Created a pending transaction.",
                        fund_obj.code, tx_date
                    );
                    println!(
                        "It will be automatically settled when you run 'fund sync' after the official NAV is published."
                    );
                }
            } else {
                // Manual mode: user provides shares and nav
                let s = shares.expect("Must provide --shares in manual mode");
                let n = nav.expect("Must provide --nav in manual mode");
                db::add_transaction(
                    &conn,
                    wallet_id,
                    &fund_obj.code,
                    "buy",
                    &money.to_string(),
                    Some(&s.to_string()),
                    Some(&n.to_string()),
                    "0",
                    &tx_date,
                    "settled",
                )
                .expect("Failed to record transaction");

                println!(
                    "Bought {}: {} shares (NAV: {}) on {}",
                    fund_obj.code, s, n, tx_date
                );
            }
        }
        Commands::Sell {
            fund,
            money,
            shares,
            nav,
            fee,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);

            let fund_obj = resolver::resolve_fund(&conn, &fund, true, false)
                .await
                .unwrap_or_else(|e| {
                    eprintln!("Error resolving fund: {}", e);
                    std::process::exit(1);
                });

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("Error: {}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            let current_shares =
                db::get_fund_shares(&conn, wallet_id, &fund_obj.code).expect("DB error");

            // 1. Resolve NAV (use find_prev_available_nav for sell - need previous day's NAV)
            let (final_nav, actual_date) = if let Some(n_str) = nav {
                let n = Decimal::from_str(&n_str).expect("Invalid --nav");
                (n, tx_date.clone())
            } else {
                // Auto mode: use smart NAV lookup (find previous available NAV for sell)
                let nav_result = db::find_prev_available_nav(&conn, &fund_obj.code, &tx_date, 20)
                    .expect("DB error");
                if let Some((actual_date, n)) = nav_result {
                    if actual_date != tx_date {
                        println!(
                            "Note: NAV for {} not available, used {}",
                            tx_date, actual_date
                        );
                    }
                    (n, actual_date)
                } else {
                    // Fall back to latest NAV
                    let latest_nav_str = db::get_latest_nav(&conn, &fund_obj.code)
                        .expect("DB error")
                        .expect("No NAV data found for this fund. Please provide --nav.");
                    let n = Decimal::from_str(&latest_nav_str).expect("Invalid NAV in DB");
                    println!("Note: No historical NAV available, used latest NAV: {}", n);
                    (n, tx_date.clone())
                }
            };

            // 2. Resolve Shares
            let final_shares = if let Some(s_input) = shares {
                finance::resolve_shares(&s_input, current_shares).expect("Invalid --shares")
            } else if let Some(m_str) = money {
                let m = Decimal::from_str(&m_str).expect("Invalid --money");
                (m / final_nav).round_dp(2)
            } else {
                panic!("Please provide --shares or --money for sell command.");
            };

            if final_shares > current_shares {
                eprintln!(
                    "Error: Insufficient shares. Current: {}, Requested: {}",
                    current_shares, final_shares
                );
                std::process::exit(1);
            }

            // 3. Resolve Fee
            let total_money = final_shares * final_nav;
            let final_fee = if let Some(f_input) = fee {
                finance::resolve_fee(&f_input, total_money).expect("Invalid --fee")
            } else {
                Decimal::ZERO
            };

            let received_money = (total_money - final_fee).round_dp(2);

            // 4. Preview and Confirm
            println!("┌─────────────────────────────────────────┐");
            println!("│           赎回操作预览 (PREVIEW)          │");
            println!("├─────────────────────────────────────────┤");
            println!("│ 基金: {} ({})", fund_obj.name, fund_obj.code);
            println!(
                "│ 份额: {} 份 (剩余: {})",
                final_shares,
                current_shares - final_shares
            );
            println!("│ 净值: {} ", final_nav);
            println!("│ 日期: {} ", tx_date);
            println!("├─────────────────────────────────────────┤");
            println!("│ 预计金额: ￥{}", total_money);
            println!("│ 赎回费用: ￥{}", final_fee);
            println!("│ 实际到账: ￥{}", received_money);
            println!("└─────────────────────────────────────────┘");

            if confirm_action("确认记录此笔交易？", cli.yes) {
                db::add_transaction(
                    &conn,
                    wallet_id,
                    &fund_obj.code,
                    "sell",
                    &received_money.to_string(),
                    Some(&final_shares.to_string()),
                    Some(&final_nav.to_string()),
                    &final_fee.to_string(),
                    &tx_date,
                    "settled",
                )
                .expect("Failed to record transaction");

                println!(
                    "Sold {}: {} shares on {}",
                    fund_obj.code, final_shares, tx_date
                );
            } else {
                println!("Transaction cancelled.");
            }
        }
        Commands::Import {
            file,
            pairs,
            merge,
            override_flag,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            let validated_date = if let Some(d) = date {
                Some(parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("Error: {}", e);
                    std::process::exit(1);
                }))
            } else {
                None
            };
            if let Err(e) = handle_import(
                &conn,
                wallet_id,
                file,
                pairs,
                merge,
                override_flag,
                validated_date,
            )
            .await
            {
                eprintln!("Error during import: {}", e);
                std::process::exit(1);
            }
        }
    }
}
