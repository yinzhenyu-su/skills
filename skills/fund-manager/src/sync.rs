use crate::db;
use crate::finance;
use crate::provider::Provider;
use crate::provider::aggregator::Aggregator;
use crate::provider::eastmoney_html::EastmoneyHtmlProvider;
use crate::provider::eastmoney_js::EastmoneyJsProvider;
use crate::provider::eastmoney_lsjz::EastmoneyLsjzProvider;
use crate::provider::morningstar::MorningstarProvider;
use rusqlite::Connection;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::env;
use std::str::FromStr;

fn get_today() -> String {
    chrono::Local::now().format("%Y-%m-%d").to_string()
}

pub async fn settle_pending_transactions(conn: &Connection) -> Result<usize, String> {
    let pending = db::get_pending_transactions(conn).map_err(|e| e.to_string())?;
    let mut settled_count = 0;

    for tx in pending {
        if let Ok(Some(nav)) = db::get_nav_at_date(conn, &tx.fund_code, &tx.date) {
            settle_tx(conn, &tx, nav).map_err(|e| e.to_string())?;
            settled_count += 1;
            continue;
        }

        // Try to fetch from API for that specific date
        let provider = EastmoneyLsjzProvider;
        if let Ok(data) = provider.fetch_at_date(&tx.fund_code, &tx.date).await {
            if let Some(nav) = data.nav {
                db::insert_nav_history_idempotent(
                    conn,
                    &tx.fund_code,
                    &tx.date,
                    &nav.to_string(),
                    None,
                )
                .map_err(|e| e.to_string())?;
                settle_tx(conn, &tx, nav).map_err(|e| e.to_string())?;
                settled_count += 1;
            }
        }
    }

    Ok(settled_count)
}

async fn record_dividend_for_wallets(
    conn: &Connection,
    code: &str,
    date: &str,
    dividend_per_share: Decimal,
    nav: Decimal,
) -> Result<(), String> {
    // 1. Get fund dividend mode
    let fund = db::get_fund_by_code_or_name(conn, code)
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "Fund not found".to_string())?;
    let mode = fund.dividend_mode.unwrap_or_else(|| "cash".to_string());

    // 2. Get all wallets holding this fund
    let holdings = db::get_holdings(conn, None, Some(code)).map_err(|e| e.to_string())?;

    for h in holdings {
        // 3. Time Isolation Wall
        let earliest_date = db::get_earliest_transaction_date(conn, h.wallet_id, code)
            .map_err(|e| e.to_string())?;

        if let Some(ed) = earliest_date {
            if date <= ed.as_str() {
                continue; // Ignore dividends on or before the first transaction date
            }
        } else {
            continue; // No transactions yet
        }

        // 4. Calculate total money/shares
        let total_shares = h.shares;
        if total_shares.is_zero() {
            continue;
        }

        let total_dividend_money = (dividend_per_share * total_shares).round_dp(2);

        if mode == "reinvest" {
            let reinvest_shares = (total_dividend_money / nav).round_dp(2);
            db::add_transaction(
                conn,
                h.wallet_id,
                code,
                "reinvest",
                &format!("{:.2}", total_dividend_money),
                Some(&format!("{:.2}", reinvest_shares)),
                Some(&nav.to_string()),
                "0",
                date,
                "settled",
                Some(&format!("自动记录分红再投资 (每份分红: {})", dividend_per_share)),
                "auto",
            )
            .map_err(|e| e.to_string())?;
        } else {
            db::add_transaction(
                conn,
                h.wallet_id,
                code,
                "dividend",
                &format!("{:.2}", total_dividend_money),
                None,
                None,
                "0",
                date,
                "settled",
                Some(&format!("自动记录现金分红 (每份分红: {})", dividend_per_share)),
                "auto",
            )
            .map_err(|e| e.to_string())?;
        }
    }
    Ok(())
}

fn settle_tx(conn: &Connection, tx: &db::Transaction, nav: Decimal) -> Result<(), String> {
    match tx.t_type.as_str() {
        "buy" => {
            let money = Decimal::from_str(&tx.money).unwrap_or_default();
            let fund = db::get_fund_by_code_or_name(conn, &tx.fund_code)
                .map_err(|e| e.to_string())?
                .ok_or_else(|| "Fund not found".to_string())?;

            let fee_rate_str = fund.sales_fee.as_deref().unwrap_or("0.15%");
            let fee_rate = finance::parse_percentage_rate(fee_rate_str);

            let res = finance::calculate_purchase(money, nav, fee_rate);

            db::update_transaction_settlement(
                conn,
                tx.id,
                &res.shares.to_string(),
                &nav.to_string(),
                &res.fee.to_string(),
                &money.to_string(),
                "settled",
            )
            .map_err(|e| e.to_string())?;
        }
        "sell" => {
            let shares = tx
                .shares
                .as_ref()
                .and_then(|s| Decimal::from_str(s).ok())
                .unwrap_or_default();
            if shares.is_zero() {
                return Ok(()); // Keep pending if shares is 0
            }

            // Default sell fee 0.5% if not known
            let fee_rate = dec!(0.005);

            let res = finance::calculate_sell(shares, nav, fee_rate);

            db::update_transaction_settlement(
                conn,
                tx.id,
                &shares.to_string(),
                &nav.to_string(),
                &res.fee.to_string(),
                &res.money.to_string(),
                "settled",
            )
            .map_err(|e| e.to_string())?;
        }
        _ => {}
    }
    Ok(())
}

/// Calculate the start date for syncing.
/// Priority: User Provided Start > Earliest Pending Date > Mode-specific Default
fn calculate_sync_range(
    pending_date: Option<String>,
    start: Option<String>,
    is_all: bool,
) -> String {
    let res = if let Some(s) = start {
        s
    } else if let Some(pd) = pending_date {
        pd
    } else if is_all {
        // Default for --all is 7 days ago
        (chrono::Local::now() - chrono::Duration::days(7))
            .format("%Y-%m-%d")
            .to_string()
    } else {
        // Default for specific fund is today
        get_today()
    };
    res
}

pub async fn sync_funds(
    conn: &Connection,
    specific_code: Option<String>,
    start: Option<String>,
    end: Option<String>,
    _auto_fill: bool, // Param kept for backward compatibility but internal logic refactored
) -> Result<(), String> {
    if env::var("SKIP_SYNC").is_ok() {
        return Ok(());
    }
    if env::var("FORCE_SYNC_FAILURE").is_ok() {
        return Err("网络不可达".to_string());
    }

    let is_all = specific_code.is_none();
    let codes = if let Some(ref code) = specific_code {
        vec![code.clone()]
    } else {
        let mut stmt = conn
            .prepare("SELECT code FROM fund")
            .map_err(|e| e.to_string())?;
        stmt.query_map([], |row| row.get(0))
            .map_err(|e| e.to_string())?
            .map(|r| r.unwrap())
            .collect()
    };

    if codes.is_empty() {
        return Ok(());
    }

    let mut aggregator = Aggregator::new();
    aggregator.add_provider(Box::new(MorningstarProvider));
    aggregator.add_provider(Box::new(EastmoneyJsProvider));
    aggregator.add_provider(Box::new(EastmoneyHtmlProvider));
    aggregator.add_provider(Box::new(EastmoneyLsjzProvider));

    let effective_end = end.unwrap_or_else(get_today);
    let total = codes.len();

    for (i, code) in codes.into_iter().enumerate() {
        let fund_name = db::get_fund_by_code_or_name(conn, &code)
            .map(|f| f.map(|obj| obj.name).unwrap_or_else(|| "未知".to_string()))
            .unwrap_or_else(|_| "未知".to_string());

        println!(
            "[{}/{}] 正在同步 {} ({}) ...",
            i + 1,
            total,
            code,
            fund_name
        );

        let pending_date =
            db::get_earliest_pending_date(conn, Some(&code)).map_err(|e| e.to_string())?;

        let effective_start = calculate_sync_range(pending_date, start.clone(), is_all);

        if effective_start <= effective_end {
            let lsjz = EastmoneyLsjzProvider;
            match lsjz
                .fetch_range(&code, &effective_start, &effective_end)
                .await
            {
                Ok(results) => {
                    let count = results.len();
                    if count > 0 {
                        let mut sorted_results = results;
                        sorted_results.sort_by(|a, b| a.date.cmp(&b.date));

                        let mut prev_nav: Option<Decimal> = None;
                        let mut prev_acc_nav: Option<Decimal> = None;

                        if let Some(first) = sorted_results.first() {
                            if let Some(ref first_date) = first.date {
                                if let Ok(Some((_, n, a))) =
                                    db::get_latest_nav_before(conn, &code, first_date)
                                {
                                    prev_nav = Some(n);
                                    prev_acc_nav = a;
                                }
                            }
                        }

                        for data in sorted_results {
                            if let (Some(nav), Some(date)) = (data.nav, data.date.clone()) {
                                db::insert_nav_history_idempotent(
                                    conn,
                                    &code,
                                    &date,
                                    &nav.to_string(),
                                    data.acc_nav.as_ref().map(|d| d.to_string()).as_deref(),
                                )
                                .map_err(|e| e.to_string())?;

                                if let (Some(acc_nav), Some(p_nav), Some(p_acc_nav)) =
                                    (data.acc_nav, prev_nav, prev_acc_nav)
                                {
                                    let div =
                                        finance::detect_dividend(nav, acc_nav, p_nav, p_acc_nav);
                                    if div > Decimal::ZERO {
                                        record_dividend_for_wallets(conn, &code, &date, div, nav)
                                            .await?;
                                    }
                                }

                                prev_nav = Some(nav);
                                prev_acc_nav = data.acc_nav;
                            }
                        }
                        println!("  ✓ 已同步 {} 天的历史净值并自动检测分红", count);
                    }
                }
                Err(e) => {
                    println!("  ⚠️ 同步历史净值失败 ({}): {}", code, e);
                }
            }
        }

        // Always sync latest and metadata
        match aggregator.fetch_all(&code).await {
            Ok(data) => {
                if let (Some(nav), Some(date)) = (data.nav, data.date) {
                    db::insert_nav_history_idempotent(
                        conn,
                        &code,
                        &date,
                        &nav.to_string(),
                        data.acc_nav.as_ref().map(|d| d.to_string()).as_deref(),
                    )
                    .map_err(|e| e.to_string())?;
                }

                // Update metadata (fee, name, etc.)
                let now = chrono::Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();
                db::add_fund(
                    conn,
                    &data.code,
                    &data.name.unwrap_or(fund_name),
                    data.fund_type.as_deref(),
                    data.risk_level.as_deref(),
                    data.manager.as_deref(),
                    data.company.as_deref(),
                    data.establish_date.as_deref(),
                    data.mgmt_fee.as_deref(),
                    data.trust_fee.as_deref(),
                    data.sales_fee.as_deref(),
                    Some(&now),
                )
                .map_err(|e| e.to_string())?;
            }
            Err(e) => {
                println!("  ⚠️ 同步元数据失败 ({}): {}", code, e);
            }
        }
    }

    // Auto-settle pending transactions
    match settle_pending_transactions(conn).await {
        Ok(count) if count > 0 => {
            println!("✨ 成功自动结算 {} 笔交易记录。", count);
        }
        Err(e) => {
            println!("⚠️ 自动结算失败: {}", e);
        }
        _ => {}
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_calculate_sync_range() {
        // 1. start provided -> returns start (highest priority)
        assert_eq!(
            calculate_sync_range(Some("2024-01-01".into()), Some("2024-02-01".into()), true),
            "2024-02-01"
        );

        // 2. pending date exists, no start -> returns pending date
        assert_eq!(
            calculate_sync_range(Some("2024-03-01".into()), None, false),
            "2024-03-01"
        );

        // 3. no start, no pending, is_all -> returns 7 days ago
        let seven_days_ago = (chrono::Local::now() - chrono::Duration::days(7))
            .format("%Y-%m-%d")
            .to_string();
        assert_eq!(calculate_sync_range(None, None, true), seven_days_ago);

        // 4. no start, no pending, NOT is_all -> returns today
        assert_eq!(calculate_sync_range(None, None, false), get_today());
    }

    #[test]
    fn test_calculate_sync_range_priority() {
        // Start has higher priority than pending date
        assert_eq!(
            calculate_sync_range(Some("2024-01-01".into()), Some("2024-06-01".into()), false),
            "2024-06-01"
        );

        // When start is None, falls back to pending date
        assert_eq!(
            calculate_sync_range(Some("2024-03-15".into()), None, true),
            "2024-03-15"
        );
    }

    #[tokio::test]
    async fn test_sync_funds_empty_codes() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // Should succeed with empty fund list
        let result = sync_funds(&conn, None, None, None, false).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_sync_funds_skips_when_env_set() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // Add a fund
        db::add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");

        // Set SKIP_SYNC env
        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let result = sync_funds(&conn, Some("000300".to_string()), None, None, false).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_sync_funds_returns_error_when_env_set() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        db::add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");

        // Set FORCE_SYNC_FAILURE env
        unsafe {
            std::env::set_var("FORCE_SYNC_FAILURE", "1");
        }
        let result = sync_funds(&conn, Some("000300".to_string()), None, None, false).await;
        unsafe {
            std::env::remove_var("FORCE_SYNC_FAILURE");
        }

        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), "网络不可达");
    }

    #[tokio::test]
    async fn test_settle_pending_transactions_empty() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // No pending transactions - should succeed
        let result = settle_pending_transactions(&conn).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_settle_pending_transactions_success() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // Setup: wallet, fund, nav, pending transaction
        db::add_wallet(&conn, "TestWallet").expect("Failed to add wallet");
        db::add_fund(
            &conn,
            "000300",
            "沪深300",
            Some("股票型"),
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");
        db::insert_nav_history_idempotent(&conn, "000300", "2024-03-01", "1.5000", None)
            .expect("Failed to insert nav");

        // Add pending buy transaction
        db::add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1500.00",
            None, // shares not set - pending
            None, // nav not set - pending
            "2.25",
            "2024-03-01",
            "pending",
            None,
            "manual",
        )
        .expect("Failed to add pending transaction");

        // Settle
        let result = settle_pending_transactions(&conn).await;
        assert!(result.is_ok());

        // Verify transaction is now settled
        let pending = db::get_pending_transactions(&conn).expect("Failed to get pending");
        assert!(pending.is_empty());

        // Verify transaction has shares and nav now
        let conn2 = Connection::open(path).expect("Failed to open DB");
        let mut stmt = conn2
            .prepare("SELECT shares, nav, status FROM transaction_log WHERE id = 1")
            .expect("Failed to prepare");
        let (shares, nav, status): (String, String, String) = stmt
            .query_row([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))
            .expect("Failed to query");
        assert_eq!(status, "settled");
        assert!(!shares.is_empty());
        assert!(!nav.is_empty());
    }

    #[tokio::test]
    async fn test_record_dividend_for_wallets() {
        let conn = db::setup_test_db().unwrap();
        db::add_wallet(&conn, "Test").unwrap();
        db::add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        // 1. Transaction on 2026-03-10
        db::add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1000",
            Some("1000"),
            Some("1.0"),
            "0",
            "2026-03-10",
            "settled",
            None,
            "manual",
        )
        .unwrap();

        // 2. Dividend on 2026-03-05 (Before purchase) -> Should be ignored
        record_dividend_for_wallets(&conn, "000300", "2026-03-05", dec!(0.1), dec!(1.0))
            .await
            .unwrap();
        let history = db::get_transaction_history(&conn, Some("000300"), Some(1), None, 0).unwrap();
        assert_eq!(history.len(), 1); // Only the buy

        // 3. Dividend on 2026-03-15 (After purchase) -> Should be recorded
        record_dividend_for_wallets(&conn, "000300", "2026-03-15", dec!(0.1), dec!(1.0))
            .await
            .unwrap();
        let history = db::get_transaction_history(&conn, Some("000300"), Some(1), None, 0).unwrap();
        assert_eq!(history.len(), 2);
        assert_eq!(history[0].t_type, "dividend");
        assert_eq!(history[0].money, "100.00"); // 1000 shares * 0.1

        // 4. Test reinvestment mode
        db::update_fund_dividend_mode(&conn, "000300", "reinvest").unwrap();
        record_dividend_for_wallets(&conn, "000300", "2026-03-20", dec!(0.2), dec!(1.0))
            .await
            .unwrap();
        let history = db::get_transaction_history(&conn, Some("000300"), Some(1), None, 0).unwrap();
        assert_eq!(history.len(), 3);
        assert_eq!(history[0].t_type, "reinvest");
        assert_eq!(history[0].money, "200.00"); // 1000 shares * 0.2
        assert_eq!(history[0].shares, Some("200.00".to_string())); // 200 / 1.0

        // 5. Test isolation wall with 'import' transaction
        db::add_wallet(&conn, "ImportWallet").unwrap();
        db::add_transaction(
            &conn,
            2,
            "000300",
            "import",
            "5000",
            Some("5000"),
            Some("1.0"),
            "0",
            "2026-03-20",
            "settled",
            None,
            "manual",
        )
        .unwrap();

        // Dividend on 2026-03-15 (Before import) -> Should be ignored for wallet 2
        record_dividend_for_wallets(&conn, "000300", "2026-03-15", dec!(0.1), dec!(1.0))
            .await
            .unwrap();
        let history2 = db::get_transaction_history(&conn, Some("000300"), Some(2), None, 0).unwrap();
        assert_eq!(history2.len(), 1); // Only the import
    }
}
