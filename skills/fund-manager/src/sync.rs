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

fn calculate_sync_range(
    pending_date: Option<String>,
    start: Option<String>,
    auto_fill: bool,
) -> String {
    if auto_fill {
        match (pending_date, start) {
            (Some(pd), Some(s)) => {
                if pd < s {
                    pd
                } else {
                    s
                }
            }
            (Some(pd), None) => pd,
            (None, Some(s)) => s,
            (None, None) => get_today(),
        }
    } else {
        start.unwrap_or_else(get_today)
    }
}

pub async fn sync_funds(
    conn: &Connection,
    specific_code: Option<String>,
    start: Option<String>,
    end: Option<String>,
    auto_fill: bool,
) -> Result<(), String> {
    if env::var("SKIP_SYNC").is_ok() {
        return Ok(());
    }
    if env::var("FORCE_SYNC_FAILURE").is_ok() {
        return Err("Network unreachable".to_string());
    }

    let codes = if let Some(code) = specific_code {
        vec![code]
    } else {
        let mut stmt = conn
            .prepare("SELECT code FROM fund")
            .map_err(|e| e.to_string())?;
        stmt.query_map([], |row| row.get(0))
            .map_err(|e| e.to_string())?
            .map(|r| r.unwrap())
            .collect()
    };

    let mut aggregator = Aggregator::new();
    aggregator.add_provider(Box::new(EastmoneyJsProvider));
    aggregator.add_provider(Box::new(EastmoneyHtmlProvider));
    aggregator.add_provider(Box::new(EastmoneyLsjzProvider));

    let effective_end = end.unwrap_or_else(get_today);

    for code in codes {
        let pending_date = if auto_fill {
            db::get_pending_transactions(conn)
                .map_err(|e| e.to_string())?
                .into_iter()
                .filter(|p| p.fund_code == code)
                .map(|p| p.date)
                .min()
        } else {
            None
        };

        let effective_start = calculate_sync_range(pending_date, start.clone(), auto_fill);

        if effective_start < effective_end {
            println!(
                "📅 Syncing {} history from {} to {}...",
                code, effective_start, effective_end
            );
            let lsjz = EastmoneyLsjzProvider;
            if let Ok(results) = lsjz
                .fetch_range(&code, &effective_start, &effective_end)
                .await
            {
                for data in results {
                    if let (Some(nav), Some(date)) = (data.nav, data.date) {
                        db::insert_nav_history_idempotent(conn, &code, &date, &nav.to_string())
                            .map_err(|e| e.to_string())?;
                    }
                }
            }
        }

        // Always sync latest and metadata
        if let Ok(data) = aggregator.fetch_all(&code).await {
            if let Some(nav) = data.nav {
                let date = data.date.unwrap_or_else(get_today);
                db::insert_nav_history_idempotent(conn, &code, &date, &nav.to_string())
                    .map_err(|e| e.to_string())?;
            }
            if let Some(fee) = data.fee_rate {
                conn.execute(
                    "UPDATE fund SET management_fee = ?1 WHERE code = ?2",
                    [fee.to_string(), code],
                )
                .map_err(|e| e.to_string())?;
            }
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

            let fee_rate = fund_obj.management_fee.as_deref().unwrap_or("0.0015");
            let fee_rate_dec =
                Decimal::from_str(fee_rate.trim_end_matches('%')).unwrap_or(dec!(0.0015));

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
        // 1. No auto_fill, no start -> returns today
        assert_eq!(calculate_sync_range(None, None, false), get_today());

        // 2. No auto_fill, start provided -> returns start
        assert_eq!(
            calculate_sync_range(None, Some("2024-01-01".into()), false),
            "2024-01-01"
        );

        // 3. auto_fill, pending date exists -> returns pending date
        assert_eq!(
            calculate_sync_range(Some("2024-02-01".into()), None, true),
            "2024-02-01"
        );

        // 4. auto_fill, both exist -> returns earlier
        assert_eq!(
            calculate_sync_range(Some("2024-03-01".into()), Some("2024-02-01".into()), true),
            "2024-02-01"
        );
        assert_eq!(
            calculate_sync_range(Some("2024-01-01".into()), Some("2024-02-01".into()), true),
            "2024-01-01"
        );
    }
}
