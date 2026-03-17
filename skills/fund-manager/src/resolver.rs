use crate::db::{self, Fund};
use crate::provider::aggregator::Aggregator;
use crate::provider::morningstar_search::MorningstarSearchProvider;
use chrono::{Duration, NaiveDateTime, Utc};
use inquire::Select;
use rusqlite::Connection;

fn is_6_digit_code(input: &str) -> bool {
    input.len() == 6 && input.chars().all(|c| c.is_ascii_digit())
}

pub async fn resolve_fund(
    conn: &Connection,
    input: &str,
    interactive: bool,
    local_only: bool,
) -> Result<Fund, String> {
    // 1. Precise match locally (by code or exact name)
    if let Some(f) = db::get_fund_by_code_or_name(conn, input).map_err(|e| e.to_string())? {
        return maybe_sync_fund(conn, f).await;
    }

    // 2. Fallback to direct fetch for 6-digit codes (if not local_only)
    if !local_only && is_6_digit_code(input) {
        println!(
            "✨ '{}' looks like a fund code. Trying direct fetch...",
            input
        );
        match sync_fund_details(conn, input).await {
            Ok(fund) => return Ok(fund),
            Err(e) => {
                return Err(format!(
                    "Failed to fetch fund details for '{}': {}",
                    input, e
                ));
            }
        }
    }

    // 3. Local fuzzy search
    let local_results = db::search_funds_locally(conn, input).map_err(|e| e.to_string())?;
    if !local_results.is_empty() {
        if local_results.len() == 1 {
            let f = local_results[0].clone();
            println!("✨ Found 1 local match: {} ({})", f.name, f.code);
            return maybe_sync_fund(conn, f).await;
        }

        if interactive {
            let options: Vec<String> = local_results
                .iter()
                .map(|f| format!("[{}] {}", f.code, f.name))
                .collect();

            let ans = Select::new(
                &format!("Multiple matches found in local database for '{}'. Please select:", input),
                options,
            )
            .prompt()
            .map_err(|e| e.to_string())?;

            let selected_code = ans
                .split(']')
                .next()
                .unwrap()
                .trim_start_matches('[')
                .to_string();

            let f = local_results.iter().find(|r| r.code == selected_code).unwrap().clone();
            return maybe_sync_fund(conn, f).await;
        } else {
            let matches = local_results
                .iter()
                .map(|f| format!("{} ({})", f.name, f.code))
                .collect::<Vec<_>>()
                .join(", ");
            return Err(format!(
                "Ambiguous local match for '{}'. Found multiple: {}",
                input, matches
            ));
        }
    }

    if local_only {
        return Err(format!("No funds found locally matching '{}'", input));
    }

    // 4. Fuzzy search remotely
    println!(
        "🔍 Fund '{}' not found locally. Searching remotely...",
        input
    );
    let search_provider = MorningstarSearchProvider;
    let results = search_provider.search(input).await?;

    if results.is_empty() {
        return Err(format!("No funds found matching '{}'", input));
    }

    let selected_code = if results.len() == 1 {
        let r = &results[0];
        println!("✨ Found matching fund: {} ({})", r.name, r.code);
        r.code.clone()
    } else {
        if !interactive {
            let matches = results
                .iter()
                .map(|r| format!("{} ({})", r.name, r.code))
                .collect::<Vec<_>>()
                .join(", ");
            return Err(format!(
                "Ambiguous remote name '{}'. Found multiple matches: {}",
                input, matches
            ));
        }

        let options: Vec<String> = results
            .iter()
            .map(|r| {
                if let Some(t) = &r.fund_type {
                    format!("[{}] {} ({})", r.code, r.name, t)
                } else {
                    format!("[{}] {}", r.code, r.name)
                }
            })
            .collect();

        let ans = Select::new(
            &format!("Multiple remote matches found for '{}'. Please select:", input),
            options,
        )
        .prompt()
        .map_err(|e| e.to_string())?;

        // Extract code from "[code] name"
        ans.split(']')
            .next()
            .unwrap()
            .trim_start_matches('[')
            .to_string()
    };

    // 5. Sync details and return
    sync_fund_details(conn, &selected_code).await
}

async fn maybe_sync_fund(conn: &Connection, fund: Fund) -> Result<Fund, String> {
    if std::env::var("SKIP_SYNC").is_ok() {
        return Ok(fund);
    }
    let mut needs_sync = false;
    match &fund.last_sync_at {
        None => needs_sync = true,
        Some(date_str) => {
            if let Ok(last_sync) = NaiveDateTime::parse_from_str(date_str, "%Y-%m-%d %H:%M:%S") {
                if Utc::now().naive_utc() - last_sync > Duration::days(30) {
                    needs_sync = true;
                }
            } else if let Ok(last_sync_date) =
                chrono::NaiveDate::parse_from_str(date_str, "%Y-%m-%d")
            {
                let last_sync = last_sync_date.and_hms_opt(0, 0, 0).unwrap();
                if Utc::now().naive_utc() - last_sync > Duration::days(30) {
                    needs_sync = true;
                }
            } else {
                needs_sync = true;
            }
        }
    };

    if needs_sync {
        println!("🔄 Metadata for {} is outdated. Syncing...", fund.code);
        sync_fund_details(conn, &fund.code).await
    } else {
        Ok(fund)
    }
}

pub async fn sync_fund_details(conn: &Connection, code: &str) -> Result<Fund, String> {
    let mut aggregator = Aggregator::new();
    aggregator.add_provider(Box::new(
        crate::provider::eastmoney_details::EastmoneyDetailProvider,
    ));
    aggregator.add_provider(Box::new(crate::provider::eastmoney_js::EastmoneyJsProvider));
    aggregator.add_provider(Box::new(
        crate::provider::eastmoney_html::EastmoneyHtmlProvider,
    ));
    aggregator.add_provider(Box::new(crate::provider::morningstar::MorningstarProvider));

    let data = aggregator.fetch_all(code).await?;

    let now = Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();

    db::add_fund(
        conn,
        &data.code,
        &data.name.clone().unwrap_or_else(|| "Unknown".to_string()),
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

    if let (Some(nav), Some(date)) = (data.nav, data.date) {
        db::insert_nav_history_idempotent(conn, &data.code, &date, &nav.to_string())
            .map_err(|e| e.to_string())?;
    }

    // Store analysis data if available (e.g. from Morningstar)
    if data.rating_3y.is_some() || data.sharpe_3y.is_some() {
        let analysis = db::FundAnalysis {
            fund_code: code.to_string(),
            snapshot_date: data.snapshot_date,
            rating_3y: data.rating_3y,
            rating_5y: data.rating_5y,
            rank_pct_3y: data.rank_pct_3y,
            sharpe_3y: data.sharpe_3y,
            calmar_3y: data.calmar_3y,
            max_drawdown_3y: data.max_drawdown_3y,
            investor_gap_3y: data.investor_gap_3y,
            last_update: now,
        };
        db::add_fund_analysis(conn, &analysis).map_err(|e| e.to_string())?;
    }

    db::get_fund_by_code_or_name(conn, code)
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "Failed to retrieve fund after sync".to_string())
}
