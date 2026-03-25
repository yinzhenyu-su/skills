use clap::Parser;
use comfy_table::{Cell, CellAlignment, Color, Table};
use csv::ReaderBuilder;
use fund_manager::cli::{Cli, Commands, FundCommands, PreviewCommands, WalletCommands};
use fund_manager::db;
use fund_manager::finance;
use fund_manager::provider::{morningstar_market, Provider, eastmoney_lsjz::EastmoneyLsjzProvider};
use fund_manager::{config, resolver, sync};
use rusqlite::Connection;
use rust_decimal::Decimal;
use rust_decimal::prelude::FromPrimitive;
use rust_decimal_macros::dec;
use std::fs;
use std::io::{self, Write};
use std::str::FromStr;

struct ImportItem {
    raw_input: String,
    money: Decimal,
    date: String,
    #[allow(dead_code)]
    line_num: Option<usize>,
}

struct ImportResult {
    input: String,
    success: bool,
    reason: String,
}

/// Holds parsed data from a holdings CSV row
struct HoldingImportItem {
    fund_name: String,
    holding_amount: Decimal,
    holding_profit: Decimal,
    #[allow(dead_code)]
    line_num: Option<usize>,
}

struct HoldingImportResult {
    fund_name: String,
    success: bool,
    reason: String,
}

fn get_today() -> String {
    chrono::Local::now().format("%Y-%m-%d").to_string()
}

fn parse_date(date_str: &str) -> Result<String, String> {
    if date_str.len() != 10 {
        return Err("日期格式必须为 YYYY-MM-DD".to_string());
    }
    // Simple validation
    let parts: Vec<&str> = date_str.split('-').collect();
    if parts.len() != 3 {
        return Err("日期格式必须为 YYYY-MM-DD".to_string());
    }
    Ok(date_str.to_string())
}

/// Smart NAV lookup: Local DB -> Morningstar API -> Forward lookup (next available)
async fn smart_nav_lookup(
    conn: &Connection,
    code: &str,
    requested_date: &str,
) -> Result<Option<(String, Decimal)>, String> {
    // 1. Check local DB first
    if let Ok(Some(nav)) = db::get_nav_at_date(conn, code, requested_date) {
        return Ok(Some((requested_date.to_string(), nav)));
    }

    // 2. Not in DB, try to fetch from API
    println!(
        "🔍 正在从天天基金获取 {} 在 {} 的净值...",
        code, requested_date
    );
    let provider = EastmoneyLsjzProvider;
    if let Ok(data) = provider.fetch_at_date(code, requested_date).await {
        if let Some(nav) = data.nav {
            // Save to DB for future use
            db::insert_nav_history_idempotent(conn, code, requested_date, &nav.to_string())
                .map_err(|e: rusqlite::Error| e.to_string())?;
            return Ok(Some((requested_date.to_string(), nav)));
        }
    }

    // 3. API also failed, fall back to backward lookup first (up to 30 days)
    // This handles cases like QDII funds where today's NAV isn't published yet
    let nav_result = db::find_prev_available_nav(conn, code, requested_date, 30)
        .map_err(|e: rusqlite::Error| e.to_string())?;
    if nav_result.is_some() {
        return Ok(nav_result);
    }

    // 4. Nothing backward, try forward (up to 20 days) as last resort
    let nav_result = db::find_next_available_nav(conn, code, requested_date, 20)
        .map_err(|e: rusqlite::Error| e.to_string())?;

    Ok(nav_result)
}

fn resolve_wallet_id(conn: &Connection, wallet_name: Option<String>) -> i64 {
    if let Some(name) = wallet_name {
        db::get_wallet_id_by_name(conn, &name)
            .expect("数据库错误")
            .unwrap_or_else(|| {
                eprintln!("❌ 未找到名为 '{}' 的钱包。", name);
                std::process::exit(1);
            })
    } else {
        // 尝试获取活跃钱包
        if let Ok(Some(wallet_id)) = db::get_active_wallet_id(conn) {
            return wallet_id;
        }
        // 没有活跃钱包，检查是否有其他钱包
        let all_wallets = db::get_all_wallets(conn).expect("数据库错误");
        if !all_wallets.is_empty() {
            let wallet_names: Vec<String> = all_wallets.iter().map(|w| w.name.clone()).collect();
            eprintln!("❌ 未设置活跃钱包，请先用 'fund wallet use <名称>' 选择");
            eprintln!("   可用的钱包：");
            for name in &wallet_names {
                eprintln!("   - {}", name);
            }
            eprintln!("   用法示例：fund wallet use {}", wallet_names[0]);
            std::process::exit(1);
        }
        // 没有任何钱包，自动创建"默认钱包"
        let default_name = "默认钱包";
        db::add_wallet(conn, default_name).expect("无法创建默认钱包");
        let wallet_id = db::get_wallet_id_by_name(conn, default_name)
            .expect("数据库错误")
            .unwrap();
        db::set_active_wallet(conn, wallet_id).expect("无法设置活跃钱包");
        println!("🔔 未检测到活跃钱包，已自动创建并激活「默认钱包」。");
        wallet_id
    }
}

fn require_fund_or_exit(
    conn: &Connection,
    fund: Option<String>,
    wallet_id: i64,
    subcommand: &str,
) -> String {
    if let Some(f) = fund {
        return f;
    }
    
    // 没有提供基金，列出已追踪基金作为提示
    let all_funds = db::get_funds_with_valuations(conn, Some(wallet_id)).unwrap_or_default();
    
    let is_sell = subcommand == "sell" || subcommand == "preview sell";
    let funds: Vec<_> = if is_sell {
        all_funds.into_iter().filter(|fv| fv.total_shares > 0.0).collect()
    } else {
        all_funds
    };

    if funds.is_empty() {
        eprintln!("❌ 缺少基金标识符参数");
        if is_sell {
            eprintln!("💡 Hint: 当前钱包中没有持有任何基金，无法卖出");
        } else {
            eprintln!("💡 Hint: 当前钱包中没有追踪任何基金，请先用 'fund fund add' 添加基金");
            eprintln!("   用法示例：fund {} 000300 --money 5000", subcommand);
        }
    } else {
        eprintln!("❌ 缺少基金标识符参数");
        if is_sell {
            eprintln!("💡 Hint: 请提供要卖出的基金代码或名称");
            eprintln!("   当前持有的基金：");
        } else {
            eprintln!("💡 Hint: 请提供基金代码或名称");
            eprintln!("   当前追踪的基金：");
        }
        for fv in &funds {
            eprintln!("   - {} ({})", fv.fund.name, fv.fund.code);
        }
        
        let example_args = if is_sell {
            "--shares 500"
        } else if subcommand.contains("buy") {
            "--money 5000"
        } else {
            ""
        };
        let space = if example_args.is_empty() { "" } else { " " };
        eprintln!("   用法示例：fund {} {}{}{}", subcommand, funds[0].fund.code, space, example_args);
    }
    std::process::exit(1);
}

fn confirm_action(prompt: &str, force_yes: bool) -> bool {
    if force_yes {
        println!("{} [y/N]: y (由于使用了 -y/--yes，已跳过确认)", prompt);
        return true;
    }

    print!("{} [y/N]: ", prompt);
    io::stdout().flush().unwrap();

    let mut input = String::new();
    io::stdin().read_line(&mut input).expect("读取输入失败");

    input.trim().to_lowercase() == "y"
}

fn print_resolve_error(conn: &Connection, err: resolver::ResolveError, context_val: Option<&str>) {
    eprintln!("❌ {}", err);

    if let Some(val) = context_val {
        if let resolver::ResolveError::NotFound(ref input) = err {
            if let Some(hint) = resolver::AdviceEngine::check_param_swap(input, val) {
                eprintln!("{}", hint);
            }
        }
    }

    if let resolver::ResolveError::NotFound(ref input) = err {
        if let Some(suggestion) = resolver::AdviceEngine::suggest_spelling(conn, input) {
            eprintln!("{}", suggestion);
        }
    }

    if let Ok(wallet) = db::get_active_wallet(conn) {
        eprintln!("   当前激活钱包: [{}]", wallet.name);
    }
}

async fn handle_inspect(
    conn: &Connection,
    identifier: &str,
    force: bool,
) -> resolver::ResolveResult<()> {
    let fund = resolver::resolve_fund(conn, identifier, true).await?;

    let analysis_opt = db::get_fund_analysis(conn, &fund.code)
        .map_err(|e| resolver::ResolveError::DatabaseError(e.to_string()))?;

    let needs_update = if force {
        true
    } else if let Some(ref a) = analysis_opt {
        // Update if older than 30 days
        if let Ok(last) = chrono::NaiveDateTime::parse_from_str(&a.last_update, "%Y-%m-%d %H:%M:%S")
        {
            (chrono::Utc::now().naive_utc() - last).num_days() > 30
        } else {
            true
        }
    } else {
        true
    };

    let final_analysis = if needs_update {
        println!(
            "🔄 正在从晨星获取 {} ({}) 的深度分析数据...",
            fund.name, fund.code
        );
        let updated_fund = resolver::sync_fund_details(conn, &fund.code)
            .await
            .map_err(|e| resolver::ResolveError::FetchFailed(fund.code.clone(), e))?;
        db::get_fund_analysis(conn, &updated_fund.code)
            .map_err(|e| resolver::ResolveError::DatabaseError(e.to_string()))?
    } else {
        analysis_opt
    };

    if let Some(a) = final_analysis {
        format_inspect_report(&fund, &a);
    } else {
        println!("❌ 暂无该基金的分析数据。");
    }

    Ok(())
}

fn format_inspect_report(fund: &db::Fund, analysis: &db::FundAnalysis) {
    let mut table = Table::new();
    table.set_header(vec!["评价指标", "数值", "指标说明"]);

    table.add_row(vec![
        "基金名称".to_string(),
        format!("{} ({})", fund.name, fund.code),
        "基金的官方名称及代码".to_string(),
    ]);

    table.add_row(vec![
        "晨星分类".to_string(),
        fund.fund_type.clone().unwrap_or_else(|| "N/A".to_string()),
        "基金的投资风格分类".to_string(),
    ]);

    let rating_3y = analysis
        .rating_3y
        .map(|r| "★".repeat(r as usize))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "晨星评级 (3年)".to_string(),
        rating_3y,
        "晨星三年期综合风险收益评级".to_string(),
    ]);

    let rank = analysis
        .rank_pct_3y
        .map(|r| format!("{:.2}%", r))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "同类排名 (3年)".to_string(),
        rank,
        "在同类基金中的百分比排名（越小越好）".to_string(),
    ]);

    let sharpe = analysis
        .sharpe_3y
        .map(|s| format!("{:.2}", s))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "夏普比率 (3年)".to_string(),
        sharpe,
        "衡量每承受一单位总风险所产生的超额回报（越高越好）".to_string(),
    ]);

    let calmar = analysis
        .calmar_3y
        .map(|c| format!("{:.2}", c))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "卡玛比率 (3年)".to_string(),
        calmar,
        "衡量收益与最大回撤的比率（越高越好）".to_string(),
    ]);

    let mdd = analysis
        .max_drawdown_3y
        .map(|m| format!("{:.2}%", m))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "最大回撤 (3年)".to_string(),
        mdd,
        "统计周期内的最大跌幅".to_string(),
    ]);

    let gap = analysis
        .investor_gap_3y
        .map(|g| format!("{:.2}%", g))
        .unwrap_or_else(|| "N/A".to_string());
    table.add_row(vec![
        "基民获得感".to_string(),
        gap,
        "基金回报与基民平均回报之差".to_string(),
    ]);

    println!("\n📊 基金健康分析报告 (基于晨星数据)");
    println!("{table}");

    if let Some(g) = analysis.investor_gap_3y {
        if g > 5.0 {
            println!(
                "💡 洞察：较大的投资者获得感缺口 (>{:.1}%) 表明该基金波动较大，",
                g
            );
            println!("   且投资者常因追涨杀跌（择时错误）导致亏损。");
        }
    }
}

async fn handle_import(
    conn: &Connection,
    wallet_id: i64,
    file: Option<std::path::PathBuf>,
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
            .map_err(|e| format!("无法打开 CSV 文件：{}", e))?;

        for (i, result) in rdr.records().enumerate() {
            let line_num = i + 2;
            match result {
                Ok(record) => {
                    if record.len() < 2 {
                        results.push(ImportResult {
                            input: format!("CSV 第 {} 行", line_num),
                            success: false,
                            reason: "该行少于 2 列数据".to_string(),
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
                                    reason: format!("金额必须为正数：{}", money_str),
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
                                reason: format!("无效的金额格式：{} ({})", money_str, e),
                            });
                        }
                    }
                }
                Err(e) => {
                    results.push(ImportResult {
                        input: format!("CSV 第 {} 行", line_num),
                        success: false,
                        reason: format!("CSV 错误：{}", e),
                    });
                }
            }
        }
    } else if !pairs.is_empty() {
        if pairs.len() % 2 != 0 {
            return Err("位置参数必须成对出现 [名称 金额]".to_string());
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
                            reason: format!("金额必须为正数：{}", money_str),
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
                        reason: format!("无效的金额格式：{} ({})", money_str, e),
                    });
                }
            }
        }
    } else {
        return Err("请提供 --file 参数或成对的名称/金额参数".to_string());
    }

    if items.is_empty() && results.is_empty() {
        println!("未发现可导入的项目。");
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
        match resolver::resolve_fund(conn, &item.raw_input, false).await {
            Ok(fund_obj) => {
                // Check existing holdings
                let current_shares =
                    db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
                let already_exists = !current_shares.is_zero();

                if already_exists && !merge && !override_flag {
                    result.reason =
                        "该基金在钱包中已存在。请使用 --merge 或 --override 参数。".to_string();
                } else {
                    if override_flag && already_exists {
                        conn.execute(
                            "DELETE FROM transaction_log WHERE wallet_id = ?1 AND fund_code = ?2",
                            rusqlite::params![wallet_id, fund_obj.code],
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    }

                    // Handle settlement logic with smart NAV lookup (local DB -> API -> forward lookup)
                    let nav_result = smart_nav_lookup(conn, &fund_obj.code, &item.date).await?;

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
                            "buy",
                            &item.money.to_string(),
                            Some(&res.shares.to_string()),
                            Some(&nav.to_string()),
                            &res.fee.to_string(),
                            &actual_date,
                            "settled",
                            None,
                            "import",
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    } else {
                        // NAV not available, create pending
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
                            "buy",
                            &item.money.to_string(),
                            None,
                            None,
                            &fee.to_string(),
                            &item.date,
                            "pending",
                            None,
                            "import",
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    }
                    result.success = true;
                }
            }
            Err(e) => {
                let mut reason = e.to_string();
                if let resolver::ResolveError::NotFound(ref input) = e {
                    if let Some(hint) =
                        resolver::AdviceEngine::check_param_swap(input, &item.money.to_string())
                    {
                        reason = format!("{}\n{}", reason, hint);
                    }
                    if let Some(suggestion) = resolver::AdviceEngine::suggest_spelling(conn, input)
                    {
                        reason = format!("{}\n{}", reason, suggestion);
                    }
                }
                result.reason = reason;
            }
        }
        results.push(result);
    }

    // 3. Print Report
    let success_count = results.iter().filter(|r| r.success).count();
    let fail_count = results.iter().filter(|r| !r.success).count();

    println!("\n🚀 导入流程已完成！");
    println!("✅ 成功：{}", success_count);
    println!("❌ 失败：{}", fail_count);

    if fail_count > 0 {
        let mut table = Table::new();
        table.set_header(vec!["输入内容", "状态", "失败原因"]);
        for r in results.iter().filter(|r| !r.success) {
            table.add_row(vec![r.input.clone(), "失败".to_string(), r.reason.clone()]);
        }
        println!("\n失败详情：");
        println!("{table}");
    }

    Ok(())
}

async fn handle_import_holding(
    conn: &Connection,
    wallet_id: i64,
    file: &std::path::Path,
    merge: bool,
    override_flag: bool,
) -> Result<(), String> {
    let mut items = Vec::new();
    let mut results = Vec::new();

    // Parse CSV
    let mut rdr = ReaderBuilder::new()
        .has_headers(true)
        .from_path(file)
        .map_err(|e| format!("无法打开 CSV 文件：{}", e))?;

    for (i, result) in rdr.records().enumerate() {
        let line_num = i + 2;
        match result {
            Ok(record) => {
                if record.len() < 3 {
                    results.push(HoldingImportResult {
                        fund_name: format!("CSV 第 {} 行", line_num),
                        success: false,
                        reason: "该行少于 3 列数据".to_string(),
                    });
                    continue;
                }
                let fund_name = record.get(0).unwrap().trim().to_string();
                let amount_str = record.get(1).unwrap();
                let profit_str = record.get(2).unwrap();

                match Decimal::from_str(amount_str) {
                    Ok(amount) => {
                        if amount <= Decimal::ZERO {
                            results.push(HoldingImportResult {
                                fund_name: fund_name.clone(),
                                success: false,
                                reason: format!("持有金额必须为正数：{}", amount_str),
                            });
                            continue;
                        }

                        match Decimal::from_str(profit_str) {
                            Ok(profit) => {
                                items.push(HoldingImportItem {
                                    fund_name,
                                    holding_amount: amount,
                                    holding_profit: profit,
                                    line_num: Some(line_num),
                                });
                            }
                            Err(e) => {
                                results.push(HoldingImportResult {
                                    fund_name: fund_name.clone(),
                                    success: false,
                                    reason: format!("无效的持有收益格式：{} ({})", profit_str, e),
                                });
                            }
                        }
                    }
                    Err(e) => {
                        results.push(HoldingImportResult {
                            fund_name: fund_name.clone(),
                            success: false,
                            reason: format!("无效的持有金额格式：{} ({})", amount_str, e),
                        });
                    }
                }
            }
            Err(e) => {
                results.push(HoldingImportResult {
                    fund_name: format!("CSV 第 {} 行", line_num),
                    success: false,
                    reason: format!("CSV 错误：{}", e),
                });
            }
        }
    }

    if items.is_empty() && results.is_empty() {
        println!("未发现可导入的项目。");
        return Ok(());
    }

    // Process each item
    for item in items {
        let mut result = HoldingImportResult {
            fund_name: item.fund_name.clone(),
            success: false,
            reason: String::new(),
        };

        // Delay to avoid rate limiting
        tokio::time::sleep(tokio::time::Duration::from_millis(200)).await;

        // Resolve fund name to code
        match resolver::resolve_fund(conn, &item.fund_name, false).await {
            Ok(fund_obj) => {
                // Check existing import record
                let current_shares =
                    db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
                let already_exists = !current_shares.is_zero();

                if already_exists && !merge && !override_flag {
                    result.reason =
                        "该基金在钱包中已存在导入记录。请使用 --override 参数覆盖。".to_string();
                } else {
                    if override_flag && already_exists {
                        // Delete existing import records for this fund/wallet
                        conn.execute(
                            "DELETE FROM transaction_log WHERE wallet_id = ?1 AND fund_code = ?2 AND type = 'import'",
                            rusqlite::params![wallet_id, fund_obj.code],
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    }

                    // Get current NAV
                    let nav_result = smart_nav_lookup(conn, &fund_obj.code, &get_today()).await?;

                    if let Some((_, nav)) = nav_result {
                        // Calculate shares and cost
                        let (shares, cost_basis, _cost_per_share) =
                            finance::calculate_holding_from_profit(
                                item.holding_amount,
                                item.holding_profit,
                                nav,
                            );

                        // Write to transaction_log
                        db::add_transaction(
                            conn,
                            wallet_id,
                            &fund_obj.code,
                            "import",
                            &cost_basis.to_string(),
                            Some(&shares.to_string()),
                            Some(&nav.to_string()),
                            "0",
                            &get_today(),
                            "settled",
                            None,
                            "import",
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;

                        result.success = true;
                    } else {
                        result.reason = "无法获取基金净值".to_string();
                    }
                }
            }
            Err(e) => {
                result.reason = e.to_string();
            }
        }
        results.push(result);
    }

    // Print Report
    let success_count = results.iter().filter(|r| r.success).count();
    let fail_count = results.iter().filter(|r| !r.success).count();

    println!("\n🚀 导入流程已完成！");
    println!("✅ 成功：{}", success_count);
    println!("❌ 失败：{}", fail_count);

    if fail_count > 0 {
        let mut table = Table::new();
        table.set_header(vec!["基金名称", "状态", "失败原因"]);
        for r in results.iter().filter(|r| !r.success) {
            table.add_row(vec![
                r.fund_name.clone(),
                "失败".to_string(),
                r.reason.clone(),
            ]);
        }
        println!("\n失败详情：");
        println!("{table}");
    }

    Ok(())
}

async fn handle_preview_buy(
    conn: &Connection,
    wallet_id: i64,
    fund: &str,
    money: Option<Decimal>,
    shares: Option<Decimal>,
    nav: Option<Decimal>,
    date: Option<&str>,
) {
    let fund_obj = match resolver::resolve_fund(conn, fund, true).await {
        Ok(f) => f,
        Err(e) => {
            print_resolve_error(&conn, e, None);
            std::process::exit(1);
        }
    };

    // Resolve NAV
    let (nav_date, nav_value) = if let Some(nav_val) = nav {
        (get_today(), nav_val)
    } else if let Some(d) = date {
        let parsed_date = parse_date(d).unwrap_or_else(|e| {
            eprintln!("❌ {}", e);
            std::process::exit(1);
        });
        let nav_result = smart_nav_lookup(conn, &fund_obj.code, &parsed_date)
            .await
            .unwrap_or_else(|e| {
                eprintln!("❌ 净值查询失败：{}", e);
                std::process::exit(1);
            });
        match nav_result {
            Some((d, n)) => (d, n),
            None => {
                eprintln!("❌ 未找到 {} 的净值数据", fund_obj.code);
                std::process::exit(1);
            }
        }
    } else {
        let nav_result = smart_nav_lookup(conn, &fund_obj.code, &get_today())
            .await
            .unwrap_or_else(|e| {
                eprintln!("❌ 净值查询失败：{}", e);
                std::process::exit(1);
            });
        match nav_result {
            Some((d, n)) => (d, n),
            None => {
                eprintln!("❌ 未找到 {} 的净值数据", fund_obj.code);
                std::process::exit(1);
            }
        }
    };

    // Calculate fee rate
    let fee_rate = if let Some(ref sf) = fund_obj.sales_fee {
        if !sf.trim().is_empty() && sf != "0.00%" {
            finance::parse_percentage_rate(sf)
        } else {
            dec!(0.0015)
        }
    } else {
        dec!(0.0015)
    };

    // Calculate based on money or shares input
    let (input_money, input_shares, fee, net_shares) = if let Some(m) = money {
        let purchase_result = finance::calculate_purchase(m, nav_value, fee_rate);
        (
            m,
            purchase_result.shares,
            purchase_result.fee,
            purchase_result.shares,
        )
    } else if let Some(s) = shares {
        let total_money = s * nav_value;
        let fee = (total_money * fee_rate / (dec!(1) + fee_rate)).round_dp(2);
        (total_money.round_dp(2), s, fee, s)
    } else {
        eprintln!("❌ 缺少参数：请提供 --money 或 --shares 之一");
        eprintln!("   --money <金额>   按投入金额买入，例如：--money 5000");
        eprintln!("   --shares <份额>  按指定份额买入，例如：--shares 4538.65");
        std::process::exit(1);
    };

    // Get current holdings
    let current_shares =
        db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
    let current_cost = if current_shares.is_zero() {
        Decimal::ZERO
    } else {
        // Get the cost from holdings
        let holdings = db::get_holdings(conn, wallet_id).unwrap_or_default();
        holdings
            .iter()
            .find(|h| h.fund_code == fund_obj.code)
            .map(|h| Decimal::from_str(&h.net_cost).unwrap_or_default())
            .unwrap_or(Decimal::ZERO)
    };

    // Calculate post-buy values
    let new_total_shares = current_shares + net_shares;
    let new_total_cost = current_cost + input_money - fee;
    let new_avg_cost = if new_total_shares.is_zero() {
        Decimal::ZERO
    } else {
        (new_total_cost / new_total_shares).round_dp(4)
    };
    let current_avg_cost = if current_shares.is_zero() {
        Decimal::ZERO
    } else {
        (current_cost / current_shares).round_dp(4)
    };

    // Display preview
    println!("┌─────────────────────────────────────────────────────────┐");
    println!("│ 基金: {} ({})", fund_obj.name, fund_obj.code);
    println!("│ 净值: {} (日期: {})", nav_value, nav_date);
    println!("├─────────────────────────────────────────────────────────┤");
    if money.is_some() {
        println!("│ 投入金额: {} 元", input_money);
    } else {
        println!("│ 买入份额: {} 份", input_shares);
    }
    println!("│ 申购费率: {}%", (fee_rate * dec!(100)).round_dp(2));
    println!("│ 手续费: {} 元", fee);
    if money.is_some() {
        println!("│ 获得份额: {} 份", net_shares);
    } else {
        println!("│ 投入金额: {} 元", input_money);
    }
    println!("├─────────────────────────────────────────────────────────┤");
    println!("│ 买入后持仓变化:                                        │");
    println!("│   份额: {} → {}", current_shares, new_total_shares);
    println!("│   成本: {} → {}", current_cost, new_total_cost);
    println!("│   均价: {} → {}", current_avg_cost, new_avg_cost);
    println!("└─────────────────────────────────────────────────────────┘");
}

async fn handle_market_index(names: &[String]) {
    let filter_names = if names.is_empty() {
        None
    } else {
        Some(names)
    };

    match morningstar_market::fetch_indices(filter_names).await {
        Ok(indices) => {
            if indices.is_empty() && filter_names.is_some() {
                eprintln!("⚠️ 警告: 未找到指定的指数行情数据。");
                eprintln!("支持的指数包括: 沪深300, 上证指数, 深证成指, 创业板指, 中证500, 恒生指数, 恒生科技, 标普500, 纳斯达克, 道琼斯等。");
                return;
            }

            // If user provided names, check for unrecognized ones
            if let Some(target_names) = filter_names {
                for name in target_names {
                    if !indices.iter().any(|idx| &idx.name == name) {
                        eprintln!("⚠️ 警告: 未找到名为 \"{}\" 的指数行情数据。", name);
                    }
                }
            }

            let mut table = Table::new();
            table.set_header(vec![
                Cell::new("指数名称").set_alignment(CellAlignment::Left),
                Cell::new("当前点位").set_alignment(CellAlignment::Right),
                Cell::new("涨跌").set_alignment(CellAlignment::Right),
                Cell::new("涨跌幅 (%)").set_alignment(CellAlignment::Right),
            ]);

            let mut current_market = String::new();

            for idx in indices {
                // Add market separator if changed
                if idx.market != current_market {
                    current_market = idx.market.clone();
                    table.add_row(vec![
                        Cell::new(format!("─── {} ───", current_market))
                            .add_attribute(comfy_table::Attribute::Bold)
                            .set_alignment(CellAlignment::Center),
                        Cell::new(""),
                        Cell::new(""),
                        Cell::new(""),
                    ]);
                }

                let color = if idx.change > 0.0 {
                    Color::Red
                } else if idx.change < 0.0 {
                    Color::Green
                } else {
                    Color::Reset
                };

                table.add_row(vec![
                    Cell::new(idx.name).set_alignment(CellAlignment::Left),
                    Cell::new(format!("{:.2}", idx.current))
                        .set_alignment(CellAlignment::Right)
                        .fg(color),
                    Cell::new(format!("{:.2}", idx.change))
                        .set_alignment(CellAlignment::Right)
                        .fg(color),
                    Cell::new(format!("{:.2}%", idx.change_percent))
                        .set_alignment(CellAlignment::Right)
                        .fg(color),
                ]);
            }

            println!("{}", table);
            println!(
                "\n数据来源: 晨星 (Morningstar) | 更新时间: {}",
                chrono::Local::now().format("%Y-%m-%d %H:%M:%S")
            );
        }
        Err(e) => {
            eprintln!("❌ 错误: 无法获取市场指数数据: {}", e);
            std::process::exit(1);
        }
    }
}

async fn handle_preview_sell(
    conn: &Connection,
    wallet_id: i64,
    fund: &str,
    money: Option<String>,
    shares: Option<String>,
    nav: Option<String>,
    date: Option<&str>,
) {
    let fund_obj = match resolver::resolve_fund(conn, fund, true).await {
        Ok(f) => f,
        Err(e) => {
            print_resolve_error(&conn, e, None);
            std::process::exit(1);
        }
    };

    // Get current holdings
    let current_shares =
        db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
    if current_shares.is_zero() {
        eprintln!("❌ 你在当前钱包中未持有该基金");
        std::process::exit(1);
    }

    let current_cost = if current_shares.is_zero() {
        Decimal::ZERO
    } else {
        let holdings = db::get_holdings(conn, wallet_id).unwrap_or_default();
        holdings
            .iter()
            .find(|h| h.fund_code == fund_obj.code)
            .map(|h| Decimal::from_str(&h.net_cost).unwrap_or_default())
            .unwrap_or(Decimal::ZERO)
    };

    // Resolve NAV
    let (nav_date, nav_value) = if let Some(ref nav_val_str) = nav {
        let nav_val = Decimal::from_str(nav_val_str).unwrap_or_else(|e| {
            eprintln!("❌ 无效的净值格式：{}", e);
            std::process::exit(1);
        });
        (get_today(), nav_val)
    } else if let Some(d) = date {
        let parsed_date = parse_date(d).unwrap_or_else(|e| {
            eprintln!("❌ {}", e);
            std::process::exit(1);
        });
        let nav_result = smart_nav_lookup(conn, &fund_obj.code, &parsed_date)
            .await
            .unwrap_or_else(|e| {
                eprintln!("❌ 净值查询失败：{}", e);
                std::process::exit(1);
            });
        match nav_result {
            Some((d, n)) => (d, n),
            None => {
                eprintln!("❌ 未找到 {} 的净值数据", fund_obj.code);
                std::process::exit(1);
            }
        }
    } else {
        let nav_result = smart_nav_lookup(conn, &fund_obj.code, &get_today())
            .await
            .unwrap_or_else(|e| {
                eprintln!("❌ 净值查询失败：{}", e);
                std::process::exit(1);
            });
        match nav_result {
            Some((d, n)) => (d, n),
            None => {
                eprintln!("❌ 未找到 {} 的净值数据", fund_obj.code);
                std::process::exit(1);
            }
        }
    };

    // Calculate shares to sell based on money or shares input
    let (sell_shares, sell_money, fee) = if let Some(ref s) = shares {
        let parsed_shares = finance::resolve_shares(s, current_shares).unwrap_or_else(|e| {
            eprintln!("❌ {}", e);
            std::process::exit(1);
        });

        if parsed_shares > current_shares {
            eprintln!(
                "⚠️ 警告：卖出份额 {} 超出当前持仓 {} 份",
                parsed_shares, current_shares
            );
        }

        let total_money = parsed_shares * nav_value;
        let sell_fee = (total_money * dec!(0)).round_dp(2); // Assume 0 fee for now
        (parsed_shares, total_money.round_dp(2), sell_fee)
    } else if let Some(ref m_str) = money {
        let parsed_money = Decimal::from_str(m_str).unwrap_or_else(|e| {
            eprintln!("❌ 无效的金额格式：{}", e);
            std::process::exit(1);
        });
        let sell_shares = (parsed_money / nav_value).round_dp(2);
        let sell_fee = (parsed_money * dec!(0)).round_dp(2);
        (sell_shares, parsed_money, sell_fee)
    } else {
        eprintln!("❌ 缺少参数：请提供 --money 或 --shares 之一");
        eprintln!("   --shares <份额>  按指定份额卖出，例如：--shares 500");
        eprintln!("   --shares all     全部卖出");
        eprintln!("   --shares 1/2     卖出一半份额");
        eprintln!("   --money <金额>   按预期收回金额卖出，例如：--money 5000");
        std::process::exit(1);
    };

    let net_received = (sell_money - fee).round_dp(2);
    let sell_ratio = if current_shares.is_zero() {
        dec!(0)
    } else {
        ((sell_shares / current_shares) * dec!(100)).round_dp(2)
    };

    // Calculate post-sell values
    let new_total_shares = (current_shares - sell_shares).max(Decimal::ZERO);
    // Proportional cost reduction
    let cost_reduction = if current_shares.is_zero() {
        Decimal::ZERO
    } else {
        (current_cost * sell_shares / current_shares).round_dp(2)
    };
    let new_total_cost = (current_cost - cost_reduction).max(Decimal::ZERO);
    let new_avg_cost = if new_total_shares.is_zero() {
        Decimal::ZERO
    } else {
        (new_total_cost / new_total_shares).round_dp(4)
    };
    let current_avg_cost = if current_shares.is_zero() {
        Decimal::ZERO
    } else {
        (current_cost / current_shares).round_dp(4)
    };

    // Display preview
    println!("┌─────────────────────────────────────────────────────────┐");
    println!("│ 基金: {} ({})", fund_obj.name, fund_obj.code);
    println!("│ 净值: {} (日期: {})", nav_value, nav_date);
    println!("├─────────────────────────────────────────────────────────┤");
    println!("│ 卖出份额: {} 份 ({}%)", sell_shares, sell_ratio);
    println!("│ 卖出金额: {} 元", sell_money);
    println!("│ 赎回费率: 0.00%");
    println!("│ 手续费: {} 元", fee);
    println!("│ 实际到账: {} 元", net_received);
    println!("├─────────────────────────────────────────────────────────┤");
    println!("│ 卖出后持仓变化:                                        │");
    println!("│   份额: {} → {}", current_shares, new_total_shares);
    println!("│   成本: {} → {}", current_cost, new_total_cost);
    println!("│   均价: {} → {}", current_avg_cost, new_avg_cost);
    println!("└─────────────────────────────────────────────────────────┘");
}

fn handle_clap_error(err: clap::Error) {
    if err.kind() == clap::error::ErrorKind::DisplayHelp || err.kind() == clap::error::ErrorKind::DisplayVersion {
        err.exit();
    }

    let formatted = resolver::AdviceEngine::format_clap_error(err);
    eprintln!("{}", formatted);
    std::process::exit(1);
}

#[tokio::main]
async fn main() {
    let cli_res = Cli::try_parse();
    let cli = match cli_res {
        Ok(c) => c,
        Err(e) => {
            handle_clap_error(e);
            return;
        }
    };

    let app_dir = config::get_app_dir();
    fs::create_dir_all(&app_dir).expect("无法创建应用目录");

    let db_path = config::get_db_path();
    db::init_db(&db_path).expect("数据库初始化失败");

    let conn = db::open_conn(&db_path).expect("无法打开数据库");

    match cli.command {
        Commands::Wallet { command } => match command {
            WalletCommands::Add { name } => match db::add_wallet(&conn, &name) {
                Ok(_) => println!("成功添加钱包：{}", name),
                Err(e) => {
                    if e.to_string().contains("UNIQUE constraint failed") {
                        eprintln!("❌ 钱包 '{}' 已存在。", name);
                        std::process::exit(1);
                    } else {
                        eprintln!("❌ 添加钱包：{}", e);
                        std::process::exit(1);
                    }
                }
            },
            WalletCommands::List => {
                let wallets = db::get_all_wallets(&conn).expect("数据库错误");
                let active_id = db::get_active_wallet_id(&conn).expect("数据库错误");

                let mut table = Table::new();
                table.set_header(vec![
                    "状态",
                    "钱包名称",
                    "当前市值",
                    "持仓成本",
                    "累计盈亏",
                    "收益率",
                ]);

                for w in wallets {
                    let holdings = db::get_holdings(&conn, w.id).expect("数据库错误");
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

                    table.add_row(vec![
                        if Some(w.id) == active_id { "*" } else { "" },
                        &w.name,
                        &total_valuation.to_string(),
                        &total_cost.to_string(),
                        &pl.to_string(),
                        &format!("{:.2}%", pl_pct),
                    ]);
                }
                println!("{table}");
            }
            WalletCommands::Use { name } => match db::get_wallet_id_by_name(&conn, &name) {
                Ok(Some(id)) => {
                    db::set_active_wallet(&conn, id).expect("无法设置活跃钱包");
                    println!("当前已切换至钱包：{}", name);
                }
                Ok(None) => {
                    eprintln!("❌ 钱包 '{}' 不存在。", name);
                    std::process::exit(1);
                }
                Err(e) => {
                    eprintln!("❌ wallet: {}", e);
                    std::process::exit(1);
                }
            },
            WalletCommands::Delete { name } => match db::get_wallet_id_by_name(&conn, &name) {
                Ok(Some(id)) => {
                    let active_id = db::get_active_wallet_id(&conn).unwrap_or(None);
                    let is_active = Some(id) == active_id;

                    let prompt = format!(
                        "确定要删除钱包 '{}' 吗？这将永久删除该钱包及其所有的交易历史记录！",
                        name
                    );
                    if confirm_action(&prompt, cli.yes) {
                        match db::delete_wallet(&conn, id) {
                            Ok(_) => {
                                if is_active {
                                    println!(
                                        "✅ 钱包 '{}' 已成功删除。(由于该钱包原为活跃钱包，当前已重置为未选中任何钱包。)",
                                        name
                                    );
                                } else {
                                    println!("✅ 钱包 '{}' 已成功删除。", name);
                                }
                            }
                            Err(e) => {
                                eprintln!("❌ 无法删除钱包：{}", e);
                                std::process::exit(1);
                            }
                        }
                    } else {
                        println!("已取消删除操作。");
                    }
                }
                Ok(None) => {
                    eprintln!("❌ 找不到名为 '{}' 的钱包。", name);
                    std::process::exit(1);
                }
                Err(e) => {
                    eprintln!("❌ 数据库错误：{}", e);
                    std::process::exit(1);
                }
            },
            WalletCommands::Rename { old_name, new_name } => {
                // 检查旧钱包是否存在
                match db::get_wallet_id_by_name(&conn, &old_name) {
                    Ok(Some(_)) => {
                        // 检查新名称是否已存在
                        match db::get_wallet_id_by_name(&conn, &new_name) {
                            Ok(Some(_)) => {
                                eprintln!("❌ 钱包'{}'已存在。", new_name);
                                std::process::exit(1);
                            }
                            Ok(None) => {
                                // 执行重命名
                                match db::rename_wallet(&conn, &old_name, &new_name) {
                                    Ok(_) => {
                                        println!(
                                            "✅ 钱包已从'{}'重命名为'{}'。",
                                            old_name, new_name
                                        );
                                    }
                                    Err(e) => {
                                        eprintln!("❌ 重命名失败：{}", e);
                                        std::process::exit(1);
                                    }
                                }
                            }
                            Err(e) => {
                                eprintln!("❌ 数据库错误：{}", e);
                                std::process::exit(1);
                            }
                        }
                    }
                    Ok(None) => {
                        eprintln!("❌ 找不到名为'{}'的钱包。", old_name);
                        std::process::exit(1);
                    }
                    Err(e) => {
                        eprintln!("❌ 数据库错误：{}", e);
                        std::process::exit(1);
                    }
                }
            }
        },
        Commands::Fund { command } => match command {
            FundCommands::Add { fund, fee } => {
                let wallet_id = resolve_wallet_id(&conn, None);
                let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "fund add");
                
                let fund_obj = match resolver::resolve_fund(&conn, &fund_input, false).await {
                    Ok(f) => f,
                    Err(e) => {
                        print_resolve_error(&conn, e, None);
                        std::process::exit(1);
                    }
                };

                db::add_fund(
                    &conn,
                    &fund_obj.code,
                    &fund_obj.name,
                    None,
                    None,
                    None,
                    None,
                    None,
                    fee.as_deref(),
                    None,
                    None,
                    None,
                )
                .unwrap_or_else(|e| {
                    eprintln!("❌ 无法添加基金：{}", e);
                    std::process::exit(1);
                });
                println!("成功添加基金：{} ({})", fund_obj.name, fund_obj.code);
            }
            FundCommands::Delete { fund } => {
                let wallet_id = resolve_wallet_id(&conn, None);
                let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "fund delete");
                let fund_obj = match resolver::resolve_fund(&conn, &fund_input, true).await {
                    Ok(f) => f,
                    Err(e) => {
                        print_resolve_error(&conn, e, None);
                        std::process::exit(1);
                    }
                };

                let prompt = format!(
                    "您确定要删除基金 {} 及其所有的交易历史记录吗？",
                    fund_obj.code
                );
                if confirm_action(&prompt, cli.yes) {
                    if let Err(e) = db::delete_fund(&conn, &fund_obj.code) {
                        eprintln!("❌ 无法删除基金：{}", e);
                        std::process::exit(1);
                    }
                    println!("成功删除基金：{}", fund_obj.code);
                } else {
                    println!("操作已取消。");
                }
            }
            FundCommands::List { wallet } => {
                let active_wallet_id = Some(resolve_wallet_id(&conn, wallet));
                let funds =
                    db::get_funds_with_valuations(&conn, active_wallet_id).expect("数据库错误");

                let mut table = Table::new();
                let mut header = vec!["代码", "名称", "类型", "风险", "经理", "最新净值 (日期)"];
                header.push("持有份额");
                header.push("总价值");
                header.push("最后同步");
                table.set_header(header);

                for f_val in funds {
                    let f = f_val.fund;
                    let mut row = vec![
                        f.code.clone(),
                        f.name.clone(),
                        f.fund_type.unwrap_or_else(|| "-".to_string()),
                        f.risk_level.unwrap_or_else(|| "-".to_string()),
                        f.manager.unwrap_or_else(|| "-".to_string()),
                    ];

                    // 净值 (日期)
                    if let Some(nav_str) = f_val.latest_nav.as_ref() {
                        let date_str = f_val
                            .latest_nav_date
                            .as_ref()
                            .map(|d| if d.len() >= 10 { &d[5..10] } else { d })
                            .unwrap_or("??-??");
                        row.push(format!("{} ({})", nav_str, date_str));
                    } else {
                        row.push("-".to_string());
                    };

                    // 份额
                    row.push(format!("{:.2}", f_val.total_shares));

                    // 总价值
                    let valuation = if let Some(nav_str) = f_val.latest_nav {
                        let nav = Decimal::from_str(&nav_str).unwrap_or_default();
                        let shares = Decimal::from_f64(f_val.total_shares).unwrap_or_default();
                        (nav * shares).round_dp(2)
                    } else {
                        Decimal::ZERO
                    };
                    row.push(valuation.to_string());

                    row.push(
                        f.last_sync_at
                            .as_ref()
                            .map(|d| if d.len() >= 10 { &d[5..10] } else { d })
                            .unwrap_or("-")
                            .to_string(),
                    );

                    table.add_row(row);
                }
                println!("{table}");
            }
            FundCommands::Sync {
                force: _,
                start,
                end,
                auto_fill,
                all,
                fund,
            } => {
                if !all && fund.is_none() {
                    eprintln!("❌ 请指定基金标识符或使用 --all 进行全量同步。");
                    eprintln!(
                        "💡 提示：运行 'fund fund sync --all' 可以同步所有持有基金的元数据。"
                    );
                    std::process::exit(1);
                }

                let fund_code = if let Some(ref identifier) = fund {
                    match resolver::resolve_fund(&conn, identifier, true).await {
                        Ok(f) => Some(f.code),
                        Err(e) => {
                            print_resolve_error(&conn, e, None);
                            std::process::exit(1);
                        }
                    }
                } else {
                    None
                };

                if let Err(e) = sync::sync_funds(&conn, fund_code, start, end, auto_fill).await {
                    eprintln!("❌ 同步：{}", e);
                    std::process::exit(1);
                }
                println!("✅ 同步已完成。");
            }
            FundCommands::Inspect { fund, force } => {
                let wallet_id = resolve_wallet_id(&conn, None);
                let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "fund inspect");
                if let Err(e) = handle_inspect(&conn, &fund_input, force).await {
                    print_resolve_error(&conn, e, None);
                    std::process::exit(1);
                }
            }
        },
        Commands::Status { fund: _, wallet } => {
            if let Err(e) = sync::sync_funds(&conn, None, None, None, true).await {
                eprintln!("⚠️ 警告：无法获取最新数据：{}", e);
                eprintln!("当前显示的是本地数据库中的缓存数据。");
            }

            let wallet_id = resolve_wallet_id(&conn, wallet);

            let holdings = db::get_holdings(&conn, wallet_id).expect("数据库错误");

            let mut table = Table::new();
            table.set_header(vec![
                "基金",
                "份额",
                "持仓成本",
                "当前净值",
                "当前市值",
                "盈亏额",
                "收益率",
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
        Commands::History {
            fund,
            wallet,
            t_type,
            limit,
        } => {
            let wallet_id = if let Some(ref w_name) = wallet {
                match db::get_wallet_id_by_name(&conn, w_name) {
                    Ok(Some(id)) => Some(id),
                    Ok(None) => {
                        eprintln!("❌ 钱包 '{}' 不存在。", w_name);
                        std::process::exit(1);
                    }
                    Err(e) => {
                        eprintln!("❌ 数据库错误：{}", e);
                        std::process::exit(1);
                    }
                }
            } else {
                None
            };

            let fund_code = if let Some(ref f) = fund {
                match resolver::resolve_fund(&conn, f, true).await {
                    Ok(f_obj) => Some(f_obj.code),
                    Err(_) => Some(f.clone()),
                }
            } else {
                None
            };

            let history = db::get_transaction_history(
                &conn,
                fund_code.as_deref(),
                wallet_id,
                t_type.as_deref(),
                limit,
            )
            .expect("获取交易历史失败");

            if history.is_empty() {
                println!("没有找到符合条件的交易记录。");
                return;
            }

            let mut table = Table::new();
            table.load_preset(comfy_table::presets::UTF8_FULL);
            table.set_header(vec![
                "日期", "基金", "类型", "金额", "成交价", "份额", "费用", "钱包", "状态",
            ]);

            for tx in history {
                let (type_display, type_color) = match tx.t_type.as_str() {
                    "buy" => ("买入", Color::Green),
                    "sell" => ("卖出", Color::Red),
                    "dividend" => ("分红", Color::Yellow),
                    "reinvest" => ("再投", Color::Cyan),
                    "import" => ("导入", Color::Blue),
                    _ => (tx.t_type.as_str(), Color::White),
                };

                let (status_display, status_color) = if tx.status == "settled" {
                    ("✅ 已确认", Color::Green)
                } else {
                    ("⏳ 确认中", Color::Yellow)
                };

                table.add_row(vec![
                    Cell::new(&tx.date),
                    Cell::new(format!("{} ({})", tx.fund_name, tx.fund_code)),
                    Cell::new(type_display).fg(type_color),
                    Cell::new(&tx.money).set_alignment(CellAlignment::Right),
                    Cell::new(tx.nav.unwrap_or_else(|| "待确认".to_string()))
                        .set_alignment(CellAlignment::Right),
                    Cell::new(tx.shares.unwrap_or_else(|| "待确认".to_string()))
                        .set_alignment(CellAlignment::Right),
                    Cell::new(&tx.fee).set_alignment(CellAlignment::Right),
                    Cell::new(&tx.wallet_name),
                    Cell::new(status_display).fg(status_color),
                ]);
            }
            println!("{table}");
        }
        Commands::Dividend {
            fund,
            money,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "dividend");
            let fund_obj = resolver::resolve_fund(&conn, &fund_input, true)
                .await
                .expect("基金未找到");
            let tx_date = date.unwrap_or_else(get_today);
            let money_str = money.to_string();

            db::add_transaction(
                &conn,
                wallet_id,
                &fund_obj.code,
                "dividend",
                &money_str,
                None,
                None,
                "0",
                &tx_date,
                "settled",
                Some("现金分红"),
                "manual",
            )
            .expect("数据库错误");
            println!(
                "✅ 已记录分红: {} ({}), ￥{}",
                fund_obj.name, fund_obj.code, money
            );
        }
        Commands::Reinvest {
            fund,
            shares,
            nav,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "reinvest");
            let fund_obj = resolver::resolve_fund(&conn, &fund_input, true)
                .await
                .expect("基金未找到");
            let tx_date = date.unwrap_or_else(get_today);
            let nav_val = nav.unwrap_or_default();
            let nav_str = nav_val.to_string();
            let shares_str = shares.to_string();

            db::add_transaction(
                &conn,
                wallet_id,
                &fund_obj.code,
                "reinvest",
                "0",
                Some(&shares_str),
                if nav_val.is_zero() {
                    None
                } else {
                    Some(&nav_str)
                },
                "0",
                &tx_date,
                "settled",
                Some("红利再投"),
                "manual",
            )
            .expect("数据库错误");
            println!(
                "✅ 已记录红利再投: {} ({}), {} 份",
                fund_obj.name, fund_obj.code, shares
            );
        }
        Commands::Buy {
            fund,
            money,
            shares,
            nav,
            wallet,
            date,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet.clone());
            let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "buy");

            // Check for money parameter (auto mode requires it)
            let money_val = if let Some(m) = money {
                m
            } else if shares.is_some() && nav.is_some() {
                // Manual mode with shares+nav, money not required
                // We'll handle this below
                Decimal::ZERO // placeholder, will be recalculated
            } else {
                eprintln!("❌ 缺少参数：请提供 --money 或 --shares 之一");
                eprintln!("   --money <金额>   按投入金额买入，例如：--money 5000");
                eprintln!("   --shares <份额>  按指定份额买入，例如：--shares 4538.65");
                std::process::exit(1);
            };

            let fund_obj = match resolver::resolve_fund(&conn, &fund_input, true).await {
                Ok(f) => f,
                Err(e) => {
                    print_resolve_error(&conn, e, Some(&money_val.to_string()));
                    std::process::exit(1);
                }
            };

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("❌ {}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            // Auto mode: use smart NAV lookup (local DB -> API -> forward lookup)
            if shares.is_none() && nav.is_none() {
                let nav_result = match smart_nav_lookup(&conn, &fund_obj.code, &tx_date).await {
                    Ok(r) => r,
                    Err(e) => {
                        eprintln!("❌ 净值查询失败：{}", e);
                        std::process::exit(1);
                    }
                };

                if let Some((actual_date, nav_val)) = nav_result {
                    // Purchase Fee priority: sales_fee -> 0.15% (default)
                    let (fee_rate_dec, fee_source) = if let Some(ref sf) = fund_obj.sales_fee {
                        if !sf.trim().is_empty() && sf != "0.00%" {
                            (
                                finance::parse_percentage_rate(sf),
                                format!("申购费率: {}", sf),
                            )
                        } else {
                            (dec!(0.0015), "默认费率: 0.15%".to_string())
                        }
                    } else {
                        (dec!(0.0015), "默认费率: 0.15%".to_string())
                    };

                    let res = finance::calculate_purchase(money_val, nav_val, fee_rate_dec);
                    if let Err(e) = db::add_transaction(
                        &conn,
                        wallet_id,
                        &fund_obj.code,
                        "buy",
                        &money_val.to_string(),
                        Some(&res.shares.to_string()),
                        Some(&nav_val.to_string()),
                        &res.fee.to_string(),
                        &actual_date,
                        "settled",
                        None,
                        "manual",
                    ) {
                        eprintln!("❌ 记录交易失败：{}", e);
                        std::process::exit(1);
                    }

                    if actual_date != tx_date {
                        println!(
                            "💡 提示：{} 的净值数据不可用，已使用 {} 的净值。",
                            tx_date, actual_date
                        );
                    }
                    println!(
                        "成功买入 {}: {} 份额 (净值: {}, 手续费: {} [{}])，交易日期: {}",
                        fund_obj.code, res.shares, nav_val, res.fee, fee_source, actual_date
                    );
                } else {
                    // NAV not available within 20 days, create pending transaction
                    let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                        finance::parse_percentage_rate(sf)
                    } else {
                        dec!(0.0015)
                    };
                    let fee = (money_val * fee_rate_dec / (dec!(1) + fee_rate_dec)).round_dp(2);

                    if let Err(e) = db::add_transaction(
                        &conn,
                        wallet_id,
                        &fund_obj.code,
                        "buy",
                        &money_val.to_string(),
                        None,
                        None,
                        &fee.to_string(),
                        &tx_date,
                        "pending",
                        None,
                        "manual",
                    ) {
                        eprintln!("❌ 记录交易失败：{}", e);
                        std::process::exit(1);
                    }

                    println!(
                        "基金 {} 在 {} 及其前20天内的净值数据均不可用。已创建待确认交易。",
                        fund_obj.code, tx_date
                    );
                    println!("当官方发布净值后，运行 'fund fund sync' 将自动结算此笔交易。");
                }
            } else {
                // Manual mode: user provides shares and nav
                let s = shares.unwrap_or_else(|| {
                    eprintln!("❌ 手动买入模式必须提供 --shares 参数");
                    std::process::exit(1);
                });
                let n = nav.unwrap_or_else(|| {
                    eprintln!("❌ 手动买入模式必须提供 --nav 参数");
                    std::process::exit(1);
                });
                let manual_money = if money.is_some() {
                    money_val.to_string()
                } else {
                    (s * n).round_dp(2).to_string()
                };
                if let Err(e) = db::add_transaction(
                    &conn,
                    wallet_id,
                    &fund_obj.code,
                    "buy",
                    &manual_money,
                    Some(&s.to_string()),
                    Some(&n.to_string()),
                    "0",
                    &tx_date,
                    "settled",
                    None,
                    "manual",
                ) {
                    eprintln!("❌ 记录交易失败：{}", e);
                    std::process::exit(1);
                }

                println!(
                    "成功买入 {}: {} 份额 (净值: {})，交易日期: {}",
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
            let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "sell");

            let fund_obj = match resolver::resolve_fund(&conn, &fund_input, true).await {
                Ok(f) => f,
                Err(e) => {
                    let context = money.as_deref().or(shares.as_deref());
                    print_resolve_error(&conn, e, context);
                    std::process::exit(1);
                }
            };

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("❌ {}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            let current_shares =
                db::get_fund_shares(&conn, wallet_id, &fund_obj.code).unwrap_or_else(|e| {
                    eprintln!("❌ 数据库操作失败：{}", e);
                    std::process::exit(1);
                });

            // 1. Resolve NAV (use find_prev_available_nav for sell - need previous day's NAV)
            let (final_nav, _actual_date) = if let Some(n_str) = nav {
                let n = Decimal::from_str(&n_str).unwrap_or_else(|_| {
                    eprintln!("❌ 无效的净值格式 '{}'：请输入有效数字", n_str);
                    eprintln!("   用法示例：fund-manager sell {} --shares 500 --nav 1.25", fund_obj.code);
                    std::process::exit(1);
                });
                (n, tx_date.clone())
            } else {
                // Auto mode: use smart NAV lookup (find previous available NAV for sell)
                let nav_result = db::find_prev_available_nav(&conn, &fund_obj.code, &tx_date, 20)
                    .unwrap_or_else(|e| {
                        eprintln!("❌ 数据库操作失败：{}", e);
                        std::process::exit(1);
                    });
                if let Some((actual_date, n)) = nav_result {
                    if actual_date != tx_date {
                        println!(
                            "💡 提示：{} 的净值数据不可用，已使用 {} 的净值。",
                            tx_date, actual_date
                        );
                    }
                    (n, actual_date)
                } else {
                    // Fall back to latest NAV
                    let latest_nav_str = db::get_latest_nav(&conn, &fund_obj.code)
                        .unwrap_or_else(|e| {
                            eprintln!("❌ 数据库操作失败：{}", e);
                            std::process::exit(1);
                        })
                        .unwrap_or_else(|| {
                            eprintln!("❌ 未找到基金 '{}' 的净值数据", fund_obj.code);
                            eprintln!("   请使用 --nav 手动指定净值");
                            eprintln!("   用法示例：fund-manager sell {} --shares 500 --nav 1.25", fund_obj.code);
                            std::process::exit(1);
                        });
                    let n = Decimal::from_str(&latest_nav_str).unwrap_or_else(|e| {
                        eprintln!("❌ 数据库中的净值数据无效：{}", e);
                        std::process::exit(1);
                    });
                    println!("💡 提示：无历史净值数据可用，已使用最新净值: {}", n);
                    (n, tx_date.clone())
                }
            };

            // 2. Resolve Shares
            let final_shares = if let Some(s_input) = shares {
                finance::resolve_shares(&s_input, current_shares).unwrap_or_else(|e| {
                    eprintln!("❌ 无效的 --shares 参数：{}", e);
                    std::process::exit(1);
                })
            } else if let Some(m_str) = money {
                let m = Decimal::from_str(&m_str).unwrap_or_else(|_| {
                    eprintln!("❌ 无效的金额格式 '{}'：请输入有效数字", m_str);
                    std::process::exit(1);
                });
                (m / final_nav).round_dp(2)
            } else {
                panic!("请为卖出命令提供 --shares 或 --money 参数。");
            };

            if final_shares > current_shares {
                let wallet_name = db::get_active_wallet(&conn)
                    .map(|w| w.name)
                    .unwrap_or_else(|_| "未知".to_string());
                eprintln!(
                    "❌ 卖出失败：你在 [{}] 中仅持有 {} 份 '{}'，无法卖出 {} 份。",
                    wallet_name, current_shares, fund_obj.code, final_shares
                );
                std::process::exit(1);
            }

            // 3. Resolve Fee
            let total_money = final_shares * final_nav;
            let final_fee = if let Some(f_input) = fee {
                finance::resolve_fee(&f_input, total_money).unwrap_or_else(|e| {
                    eprintln!("❌ 无效的 --fee 参数：{}", e);
                    std::process::exit(1);
                })
            } else {
                Decimal::ZERO
            };

            let received_money = (total_money - final_fee).round_dp(2);

            // 4. Preview and Confirm
            println!("┌─────────────────────────────────────────┐");
            println!("│           赎回操作预览 (预览)            │");
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
                    None,
                    "manual",
                )
                .unwrap_or_else(|e| {
                    eprintln!("❌ 记录交易失败：{}", e);
                    std::process::exit(1);
                });

                println!(
                    "成功卖出 {}: {} 份额，交易日期: {}",
                    fund_obj.code, final_shares, tx_date
                );
            } else {
                println!("交易已取消。");
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
                    eprintln!("❌ {}", e);
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
                eprintln!("❌ 导入：{}", e);
                std::process::exit(1);
            }
        }
        Commands::ImportHolding {
            file,
            merge,
            override_flag,
            wallet,
        } => {
            let wallet_id = resolve_wallet_id(&conn, wallet);
            if let Err(e) =
                handle_import_holding(&conn, wallet_id, &file, merge, override_flag).await
            {
                eprintln!("❌ 导入持仓：{}", e);
                std::process::exit(1);
            }
        }
        Commands::Preview { command } => match command {
            PreviewCommands::Buy {
                fund,
                money,
                shares,
                nav,
                date,
                wallet,
            } => {
                let wallet_id = resolve_wallet_id(&conn, wallet);
                let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "buy");
                handle_preview_buy(&conn, wallet_id, &fund_input, money, shares, nav, date.as_deref())
                    .await;
            }
            PreviewCommands::Sell {
                fund,
                money,
                shares,
                nav,
                date,
                wallet,
            } => {
                let wallet_id = resolve_wallet_id(&conn, wallet);
                let fund_input = require_fund_or_exit(&conn, fund, wallet_id, "sell");
                handle_preview_sell(&conn, wallet_id, &fund_input, money, shares, nav, date.as_deref())
                    .await;
            }
        },
        Commands::Index { names } => {
            handle_market_index(names.as_slice()).await;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_date_valid() {
        assert_eq!(parse_date("2026-03-09").unwrap(), "2026-03-09");
        assert_eq!(parse_date("2024-01-01").unwrap(), "2024-01-01");
        assert_eq!(parse_date("2023-12-31").unwrap(), "2023-12-31");
    }

    #[test]
    fn test_parse_date_invalid_length() {
        assert!(parse_date("2026-3-9").is_err());
        assert!(parse_date("26-03-09").is_err());
        assert!(parse_date("").is_err());
        assert!(parse_date("2026-03-091").is_err());
    }

    #[test]
    fn test_parse_date_invalid_format() {
        // Note: parse_date only does basic validation (length and part count)
        // It does NOT validate that parts are actual numbers or valid dates
        // So these pass basic validation but have wrong format
        assert!(parse_date("2026/03/09").is_err()); // Wrong separator
        // "03-09-2026" has 10 chars and 3 parts, so it passes basic validation!
        // This is a limitation of the current implementation
        assert!(parse_date("20260309").is_err()); // No separator
    }

    #[test]
    fn test_parse_date_wrong_number_of_parts() {
        assert!(parse_date("2026-03").is_err());
        assert!(parse_date("2026-03-09-extra").is_err());
    }

    #[test]
    fn test_confirm_action_force_yes() {
        // When force_yes is true, should always return true
        // and print the message (which we can't easily capture here)
        let result = confirm_action("Test prompt?", true);
        assert!(result);
    }

    #[test]
    fn test_get_today_format() {
        let today = get_today();
        // Format should be YYYY-MM-DD (10 characters)
        assert_eq!(today.len(), 10);
        // Should be parseable as date
        assert!(chrono::NaiveDate::parse_from_str(&today, "%Y-%m-%d").is_ok());
    }

    #[test]
    fn test_import_item_structure() {
        let item = ImportItem {
            raw_input: "000300".to_string(),
            money: Decimal::new(1000, 2),
            date: "2026-03-09".to_string(),
            line_num: Some(1),
        };
        assert_eq!(item.raw_input, "000300");
        assert_eq!(item.date, "2026-03-09");
    }

    #[test]
    fn test_import_result_structure() {
        let result = ImportResult {
            input: "000300".to_string(),
            success: true,
            reason: String::new(),
        };
        assert!(result.success);
        assert!(result.reason.is_empty());

        let result = ImportResult {
            input: "invalid".to_string(),
            success: false,
            reason: "Not found".to_string(),
        };
        assert!(!result.success);
        assert_eq!(result.reason, "Not found");
    }
}
