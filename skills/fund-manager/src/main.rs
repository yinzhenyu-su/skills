use clap::Parser;
use comfy_table::Table;
use csv::ReaderBuilder;
use fund_manager::cli::{Cli, Commands, FundCommands, WalletCommands};
use fund_manager::db;
use fund_manager::finance;
use fund_manager::provider::{eastmoney_lsjz::EastmoneyLsjzProvider, Provider};
use fund_manager::{config, resolver, sync};
use rusqlite::Connection;
use rust_decimal::prelude::FromPrimitive;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::fs;
use std::io::{self, Write};
use std::str::FromStr;

struct ImportItem {
    raw_input: String,
    money: Decimal,
    date: String,
    line_num: Option<usize>,
}

struct ImportResult {
    input: String,
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
    println!("🔍 正在从天天基金获取 {} 在 {} 的净值...", code, requested_date);
    let provider = EastmoneyLsjzProvider;
    if let Ok(data) = provider.fetch_at_date(code, requested_date).await {
        if let Some(nav) = data.nav {
            // Save to DB for future use
            db::insert_nav_history_idempotent(conn, code, requested_date, &nav.to_string())
                .map_err(|e: rusqlite::Error| e.to_string())?;
            return Ok(Some((requested_date.to_string(), nav)));
        }
    }

    // 3. API also failed, fall back to forward lookup (up to 20 days)
    let nav_result =
        db::find_next_available_nav(conn, code, requested_date, 20).map_err(|e: rusqlite::Error| e.to_string())?;

    Ok(nav_result)
}

fn resolve_wallet_id(conn: &Connection, wallet_name: Option<String>) -> i64 {
    if let Some(name) = wallet_name {
        db::get_wallet_id_by_name(conn, &name)
            .expect("数据库错误")
            .unwrap_or_else(|| {
                eprintln!("❌ 错误：未找到名为 '{}' 的钱包。", name);
                std::process::exit(1);
            })
    } else {
        db::get_active_wallet_id(conn)
            .expect("数据库错误")
            .expect("未选择活跃钱包。请使用 'fund wallet use <名称>' 或指定 --wallet 参数。")
    }
}

fn confirm_action(prompt: &str, force_yes: bool) -> bool {
    if force_yes {
        println!(
            "{} [y/N]: y (由于使用了 -y/--yes，已跳过确认)",
            prompt
        );
        return true;
    }

    print!("{} [y/N]: ", prompt);
    io::stdout().flush().unwrap();

    let mut input = String::new();
    io::stdin()
        .read_line(&mut input)
        .expect("读取输入失败");

    input.trim().to_lowercase() == "y"
}

fn print_resolve_error(conn: &Connection, err: resolver::ResolveError, context_val: Option<&str>) {
    eprintln!("❌ 错误：{}", err);

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
    let fund = resolver::resolve_fund(conn, identifier, true, false).await?;

    let analysis_opt = db::get_fund_analysis(conn, &fund.code)
        .map_err(|e| resolver::ResolveError::DatabaseError(e.to_string()))?;

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
        match resolver::resolve_fund(conn, &item.raw_input, false, false).await {
            Ok(fund_obj) => {
                // Check existing holdings
                let current_shares =
                    db::get_fund_shares(conn, wallet_id, &fund_obj.code).unwrap_or(Decimal::ZERO);
                let already_exists = !current_shares.is_zero();

                if already_exists && !merge && !override_flag {
                    result.reason = "该基金在钱包中已存在。请使用 --merge 或 --override 参数。".to_string();
                } else {
                    if override_flag && already_exists {
                        conn.execute(
                            "DELETE FROM transaction_log WHERE wallet_id = ?1 AND fund_code = ?2",
                            rusqlite::params![wallet_id, fund_obj.code],
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    }

                    // Handle settlement logic with smart NAV lookup (local DB -> API -> forward lookup)
                    let nav_result = smart_nav_lookup(conn, &fund_obj.code, &item.date)
                        .await?;

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
                        )
                        .map_err(|e: rusqlite::Error| e.to_string())?;
                    } else {
                        // NAV not available, create pending
                        let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                            finance::parse_percentage_rate(sf)
                        } else {
                            dec!(0.0015)
                        };
                        let fee = (item.money * fee_rate_dec / (dec!(1) + fee_rate_dec)).round_dp(2);

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
                    if let Some(suggestion) = resolver::AdviceEngine::suggest_spelling(conn, input) {
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
            table.add_row(vec![
                r.input.clone(),
                "失败".to_string(),
                r.reason.clone(),
            ]);
        }
        println!("\n失败详情：");
        println!("{table}");
    }

    Ok(())
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    let app_dir = config::get_app_dir();
    fs::create_dir_all(&app_dir).expect("无法创建应用目录");

    let db_path = config::get_db_path();
    db::init_db(&db_path).expect("数据库初始化失败");

    let conn = Connection::open(&db_path).expect("无法打开数据库");

    match cli.command {
        Commands::Wallet { command } => match command {
            WalletCommands::Add { name } => match db::add_wallet(&conn, &name) {
                Ok(_) => println!("成功添加钱包：{}", name),
                Err(e) => {
                    if e.to_string().contains("UNIQUE constraint failed") {
                        eprintln!("❌ 错误：钱包 '{}' 已存在。", name);
                        std::process::exit(1);
                    } else {
                        eprintln!("❌ 错误（执行过程失败）：添加钱包：{}", e);
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
                        if Some(w.id) == active_id {
                            "*"
                        } else {
                            ""
                        },
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
                    eprintln!("❌ 错误：钱包 '{}' 不存在。", name);
                    std::process::exit(1);
                }
                Err(e) => {
                    eprintln!("❌ 错误（查找失败）：wallet: {}", e);
                    std::process::exit(1);
                }
            },
            WalletCommands::Delete { name } => {
                match db::get_wallet_id_by_name(&conn, &name) {
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
                                        println!("✅ 钱包 '{}' 已成功删除。(由于该钱包原为活跃钱包，当前已重置为未选中任何钱包。)", name);
                                    } else {
                                        println!("✅ 钱包 '{}' 已成功删除。", name);
                                    }
                                }
                                Err(e) => {
                                    eprintln!("❌ 错误：无法删除钱包：{}", e);
                                    std::process::exit(1);
                                }
                            }
                        } else {
                            println!("已取消删除操作。");
                        }
                    }
                    Ok(None) => {
                        eprintln!("❌ 错误：找不到名为 '{}' 的钱包。", name);
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
            FundCommands::Add { code, name, fee } => {
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
                .expect("无法添加基金");
                println!("成功添加基金：{} ({})", name, code);
            }
            FundCommands::Delete { fund } => {
                let fund_obj = match resolver::resolve_fund(&conn, &fund, true, true).await {
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
                    db::delete_fund(&conn, &fund_obj.code).expect("无法删除基金");
                    println!("成功删除基金：{}", fund_obj.code);
                } else {
                    println!("操作已取消。");
                }
            }
            FundCommands::List => {
                let active_wallet_id = db::get_active_wallet_id(&conn).expect("数据库错误");
                let funds =
                    db::get_funds_with_valuations(&conn, active_wallet_id).expect("数据库错误");

                let mut table = Table::new();
                let mut header = vec!["代码", "名称", "类型", "风险", "经理", "最新净值 (日期)"];
                if active_wallet_id.is_some() {
                    header.push("持有份额");
                    header.push("总价值");
                }
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
                        let date_str = f_val.latest_nav_date.as_ref()
                            .map(|d| if d.len() >= 10 { &d[5..10] } else { d })
                            .unwrap_or("??-??");
                        row.push(format!("{} ({})", nav_str, date_str));
                    } else {
                        row.push("-".to_string());
                    };

                    if active_wallet_id.is_some() {
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
                    }

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
                    eprintln!("❌ 错误：请指定基金标识符或使用 --all 进行全量同步。");
                    eprintln!("💡 提示：运行 'fund fund sync --all' 可以同步所有持有基金的元数据。");
                    std::process::exit(1);
                }

                let fund_code = if let Some(ref identifier) = fund {
                    match resolver::resolve_fund(&conn, identifier, true, false).await {
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
                    eprintln!("❌ 错误（执行过程失败）：同步：{}", e);
                    std::process::exit(1);
                }
                println!("✅ 同步已完成。");
            }
            FundCommands::Inspect { fund, force } => {
                if let Err(e) = handle_inspect(&conn, &fund, force).await {
                    print_resolve_error(&conn, e, None);
                    std::process::exit(1);
                }
            }
        },
        Commands::Status { fund: _ } => {
            if let Err(e) = sync::sync_funds(&conn, None, None, None, true).await {
                eprintln!("⚠️ 警告：无法获取最新数据：{}", e);
                eprintln!("当前显示的是本地数据库中的缓存数据。");
            }

            let wallet_id = db::get_active_wallet_id(&conn)
                .expect("数据库错误")
                .expect("未选择活跃钱包。请使用 'fund wallet use <名称>' 或指定 --wallet 参数。");

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
        Commands::History { fund: _ } => {
            println!("交易历史记录 (暂未实现)");
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
            let fund_obj = match resolver::resolve_fund(&conn, &fund, true, false).await {
                Ok(f) => f,
                Err(e) => {
                    print_resolve_error(&conn, e, Some(&money.to_string()));
                    std::process::exit(1);
                }
            };

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("❌ 错误：{}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            // Auto mode: use smart NAV lookup (local DB -> API -> forward lookup)
            if shares.is_none() && nav.is_none() {
                let nav_result = smart_nav_lookup(&conn, &fund_obj.code, &tx_date)
                    .await
                    .expect("无法查询净值");

                if let Some((actual_date, nav_val)) = nav_result {
                    // Purchase Fee priority: sales_fee -> 0.15% (default)
                    let (fee_rate_dec, fee_source) = if let Some(ref sf) = fund_obj.sales_fee {
                        if !sf.trim().is_empty() && sf != "0.00%" {
                            (finance::parse_percentage_rate(sf), format!("申购费率: {}", sf))
                        } else {
                            (dec!(0.0015), "默认费率: 0.15%".to_string())
                        }
                    } else {
                        (dec!(0.0015), "默认费率: 0.15%".to_string())
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
                    .expect("记录交易失败");

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
                    .expect("记录交易失败");

                    println!(
                        "基金 {} 在 {} 及其前20天内的净值数据均不可用。已创建待确认交易。",
                        fund_obj.code, tx_date
                    );
                    println!(
                        "当官方发布净值后，运行 'fund fund sync' 将自动结算此笔交易。"
                    );
                }
            } else {
                // Manual mode: user provides shares and nav
                let s = shares.expect("手动买入模式必须提供 --shares");
                let n = nav.expect("手动买入模式必须提供 --nav");
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
                .expect("记录交易失败");

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

            let fund_obj = match resolver::resolve_fund(&conn, &fund, true, false).await {
                Ok(f) => f,
                Err(e) => {
                    let context = money.as_deref().or(shares.as_deref());
                    print_resolve_error(&conn, e, context);
                    std::process::exit(1);
                }
            };

            let tx_date = if let Some(d) = date {
                parse_date(&d).unwrap_or_else(|e| {
                    eprintln!("❌ 错误：{}", e);
                    std::process::exit(1);
                })
            } else {
                get_today()
            };

            let current_shares =
                db::get_fund_shares(&conn, wallet_id, &fund_obj.code).expect("数据库错误");

            // 1. Resolve NAV (use find_prev_available_nav for sell - need previous day's NAV)
            let (final_nav, actual_date) = if let Some(n_str) = nav {
                let n = Decimal::from_str(&n_str).expect("无效的 --nav 参数");
                (n, tx_date.clone())
            } else {
                // Auto mode: use smart NAV lookup (find previous available NAV for sell)
                let nav_result = db::find_prev_available_nav(&conn, &fund_obj.code, &tx_date, 20)
                    .expect("数据库错误");
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
                        .expect("数据库错误")
                        .expect("未找到该基金的净值数据。请使用 --nav 手动指定。");
                    let n = Decimal::from_str(&latest_nav_str).expect("数据库中的净值数据无效");
                    println!("💡 提示：无历史净值数据可用，已使用最新净值: {}", n);
                    (n, tx_date.clone())
                }
            };

            // 2. Resolve Shares
            let final_shares = if let Some(s_input) = shares {
                finance::resolve_shares(&s_input, current_shares).expect("无效的 --shares 参数")
            } else if let Some(m_str) = money {
                let m = Decimal::from_str(&m_str).expect("无效的 --money 参数");
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
                finance::resolve_fee(&f_input, total_money).expect("无效的 --fee 参数")
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
                )
                .expect("记录交易失败");

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
                    eprintln!("❌ 错误：{}", e);
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
                eprintln!("❌ 错误（执行过程失败）：导入：{}", e);
                std::process::exit(1);
            }
        }
    }
}
