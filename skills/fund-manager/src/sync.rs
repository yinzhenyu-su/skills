use crate::db;
use crate::finance;
use crate::provider::Provider;
use crate::provider::aggregator::Aggregator;
use crate::provider::eastmoney_html::EastmoneyHtmlProvider;
use crate::provider::eastmoney_js::EastmoneyJsProvider;
use crate::provider::eastmoney_lsjz::EastmoneyLsjzProvider;
use rusqlite::Connection;
use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::env;
use std::str::FromStr;

fn get_today() -> String {
    chrono::Local::now().format("%Y-%m-%d").to_string()
}

/// Calculate the start date for syncing.
/// Priority: User Provided Start > Earliest Pending Date > Mode-specific Default
fn calculate_sync_range(
    pending_date: Option<String>,
    start: Option<String>,
    is_all: bool,
) -> String {
    if let Some(s) = start {
        return s;
    }

    if let Some(pd) = pending_date {
        return pd;
    }

    if is_all {
        // Default for --all is 7 days ago
        (chrono::Local::now() - chrono::Duration::days(7))
            .format("%Y-%m-%d")
            .to_string()
    } else {
        // Default for specific fund is today
        get_today()
    }
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
        return Err("Network unreachable".to_string());
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
    aggregator.add_provider(Box::new(EastmoneyJsProvider));
    aggregator.add_provider(Box::new(EastmoneyHtmlProvider));
    aggregator.add_provider(Box::new(EastmoneyLsjzProvider));

    let effective_end = end.unwrap_or_else(get_today);
    let total = codes.len();

    for (i, code) in codes.into_iter().enumerate() {
        let fund_name = db::get_fund_by_code_or_name(conn, &code)
            .map(|f| f.map(|obj| obj.name).unwrap_or_else(|| "Unknown".to_string()))
            .unwrap_or_else(|_| "Unknown".to_string());

        println!(
            "[{}/{}] Syncing {} ({}) ...",
            i + 1,
            total,
            code,
            fund_name
        );

        let pending_date = db::get_earliest_pending_date(conn, Some(&code))
            .map_err(|e| e.to_string())?;
        
        let effective_start = calculate_sync_range(pending_date, start.clone(), is_all);

        if effective_start <= effective_end {
            let lsjz = EastmoneyLsjzProvider;
            if let Ok(results) = lsjz
                .fetch_range(&code, &effective_start, &effective_end)
                .await
            {
                let count = results.len();
                if count > 0 {
                    for data in results {
                        if let (Some(nav), Some(date)) = (data.nav, data.date) {
                            db::insert_nav_history_idempotent(conn, &code, &date, &nav.to_string())
                                .map_err(|e| e.to_string())?;
                        }
                    }
                    println!("  ✓ Synced {} days of history", count);
                }
            }
        }

        // Always sync latest and metadata
        if let Ok(data) = aggregator.fetch_all(&code).await {
            if let (Some(nav), Some(date)) = (data.nav, data.date) {
                db::insert_nav_history_idempotent(conn, &code, &date, &nav.to_string())
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
            ).map_err(|e| e.to_string())?;
        }
    }

    // Auto-settle pending transactions
    settle_pending_transactions(conn).await?;

    Ok(())
}

pub async fn settle_pending_transactions(conn: &Connection) -> Result<(), String> {
    let pending = db::get_pending_transactions(conn).map_err(|e| e.to_string())?;
    if pending.is_empty() {
        return Ok(());
    }

    println!(
        "Checking {} pending transactions for settlement...",
        pending.len()
    );

    for p in pending {
        let nav_at_date =
            db::get_nav_at_date(conn, &p.fund_code, &p.date).map_err(|e| e.to_string())?;

        if let Some(nav) = nav_at_date {
            let money = Decimal::from_str(&p.money).map_err(|e| e.to_string())?;
            let fund_obj = db::get_fund_by_code_or_name(conn, &p.fund_code)
                .map_err(|e| e.to_string())?
                .ok_or_else(|| format!("Fund {} not found in DB", p.fund_code))?;

            // Use prioritized fee logic (same as main.rs buy branch)
            let fee_rate_dec = if let Some(ref sf) = fund_obj.sales_fee {
                if !sf.trim().is_empty() && sf != "0.00%" {
                    finance::parse_percentage_rate(sf)
                } else {
                    dec!(0.0015)
                }
            } else {
                dec!(0.0015)
            };

            let res = finance::calculate_purchase(money, nav, fee_rate_dec);

            db::settle_transaction(conn, p.id, &res.shares.to_string(), &nav.to_string())
                .map_err(|e| e.to_string())?;
            println!(
                "✅ Settled transaction for {} on {}: {} shares at NAV {}",
                p.fund_code, p.date, res.shares, nav
            );
        }
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
        assert_eq!(
            calculate_sync_range(None, None, true),
            seven_days_ago
        );

        // 4. no start, no pending, NOT is_all -> returns today
        assert_eq!(
            calculate_sync_range(None, None, false),
            get_today()
        );
    }
}
