use crate::db::{self, Fund};
use crate::provider::aggregator::Aggregator;
use crate::provider::morningstar_search::MorningstarSearchProvider;
use chrono::{Duration, NaiveDateTime, Utc};
use rusqlite::Connection;

fn is_6_digit_code(input: &str) -> bool {
    input.len() == 6 && input.chars().all(|c| c.is_ascii_digit())
}

#[derive(Debug)]
pub enum ResolveError {
    NotFound(String),
    Ambiguous(String, Vec<String>),
    FetchFailed(String, String),
    DatabaseError(String),
}

impl std::fmt::Display for ResolveError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ResolveError::NotFound(input) => write!(f, "未找到基金: '{}'", input),
            ResolveError::Ambiguous(input, matches) => {
                write!(
                    f,
                    "输入的 '{}' 存在歧义，找到多个匹配项: {}",
                    input,
                    matches.join(", ")
                )
            }
            ResolveError::FetchFailed(input, err) => {
                write!(f, "拉取基金 '{}' 详情失败: {}", input, err)
            }
            ResolveError::DatabaseError(err) => write!(f, "数据库错误: {}", err),
        }
    }
}

pub type ResolveResult<T> = Result<T, ResolveError>;

pub struct AdviceEngine;

impl AdviceEngine {
    pub fn check_param_swap(fund_input: &str, value_input: &str) -> Option<String> {
        // If fund_input is a number but not 6 digits, and value_input is exactly 6 digits
        let is_fund_num = fund_input.chars().all(|c| c.is_ascii_digit());
        let is_value_6_digits = is_6_digit_code(value_input);

        if is_fund_num && fund_input.len() != 6 && is_value_6_digits {
            return Some(format!(
                "💡 Hint: 你是不是把[基金代码]和[金额/份额]写反了？\n   当前参数: 名称='{}', 数值='{}'\n   建议用法: fund ... {} {}",
                fund_input, value_input, value_input, fund_input
            ));
        }
        None
    }

    pub fn suggest_spelling(conn: &Connection, input: &str) -> Option<String> {
        let all_local = db::get_all_fund_names_and_codes(conn).ok()?;
        if all_local.is_empty() {
            return None;
        }

        let mut matches = Vec::new();
        for (code, name) in all_local {
            // Simple case-insensitive contains or prefix match
            let lower_input = input.to_lowercase();
            let lower_name = name.to_lowercase();
            let lower_code = code.to_lowercase();

            if lower_name.contains(&lower_input)
                || lower_code.contains(&lower_input)
                || lower_input.contains(&lower_name)
            {
                matches.push(format!("'{}' ({})", name, code));
            }

            if matches.len() >= 2 {
                break;
            }
        }

        if !matches.is_empty() {
            return Some(format!(
                "❓ 未找到基金 '{}'。你是不是想找: {}？",
                input,
                matches.join(" 或 ")
            ));
        }
        None
    }
}

pub async fn resolve_fund(
    conn: &Connection,
    input: &str,
    local_only: bool,
) -> ResolveResult<Fund> {
    // 1. Precise match locally (by code or exact name)
    if let Some(f) = db::get_fund_by_code_or_name(conn, input)
        .map_err(|e| ResolveError::DatabaseError(e.to_string()))?
    {
        return maybe_sync_fund(conn, f).await;
    }

    // 2. Fallback to direct fetch for 6-digit codes (if not local_only)
    if !local_only && is_6_digit_code(input) {
        println!("✨ '{}' 看起来像基金代码，正在尝试直接获取详情...", input);
        match sync_fund_details(conn, input).await {
            Ok(fund) => return Ok(fund),
            Err(e) => {
                return Err(ResolveError::FetchFailed(input.to_string(), e));
            }
        }
    }

    // 3. Local fuzzy search
    let local_results = db::search_funds_locally(conn, input)
        .map_err(|e| ResolveError::DatabaseError(e.to_string()))?;
    if !local_results.is_empty() {
        if local_results.len() == 1 {
            let f = local_results[0].clone();
            println!("✨ 找到 1 个本地匹配项：{} ({})", f.name, f.code);
            return maybe_sync_fund(conn, f).await;
        }

        let matches: Vec<String> = local_results
            .iter()
            .map(|f| format!("{} ({})", f.name, f.code))
            .collect();
        return Err(ResolveError::Ambiguous(input.to_string(), matches));
    }

    if local_only {
        return Err(ResolveError::NotFound(input.to_string()));
    }

    // 4. Fuzzy search remotely
    println!("🔍 本地未找到基金 '{}'，正在尝试远程搜索...", input);
    let search_provider = MorningstarSearchProvider;
    let results = search_provider
        .search(input)
        .await
        .map_err(|e| ResolveError::FetchFailed(input.to_string(), e))?;

    if results.is_empty() {
        return Err(ResolveError::NotFound(input.to_string()));
    }

    let selected_code = if results.len() == 1 {
        let r = &results[0];
        println!("✨ 发现匹配基金：{} ({})", r.name, r.code);
        r.code.clone()
    } else {
        let matches: Vec<String> = results
            .iter()
            .map(|r| format!("{} ({})", r.name, r.code))
            .collect();
        return Err(ResolveError::Ambiguous(input.to_string(), matches));
    };

    // 5. Sync details and return
    sync_fund_details(conn, &selected_code)
        .await
        .map_err(|e| ResolveError::FetchFailed(selected_code, e))
}

async fn maybe_sync_fund(conn: &Connection, fund: Fund) -> ResolveResult<Fund> {
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
        println!("🔄 {} 的元数据已过期，正在同步...", fund.code);
        sync_fund_details(conn, &fund.code)
            .await
            .map_err(|e| ResolveError::FetchFailed(fund.code, e))
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_6_digit_code() {
        assert!(is_6_digit_code("123456"));
        assert!(!is_6_digit_code("12345"));
        assert!(!is_6_digit_code("1234567"));
        assert!(!is_6_digit_code("abcdef"));
        assert!(!is_6_digit_code(""));
        assert!(is_6_digit_code("000000")); // All zeros is still 6 digits
        assert!(!is_6_digit_code("123456a")); // Mixed
    }

    #[test]
    fn test_check_param_swap() {
        // Case: swapped
        let hint = AdviceEngine::check_param_swap("1000", "520570");
        assert!(hint.is_some());
        assert!(
            hint.unwrap()
                .contains("你是不是把[基金代码]和[金额/份额]写反了？")
        );

        // Case: not swapped (fund is 6 digits)
        let hint = AdviceEngine::check_param_swap("520570", "1000");
        assert!(hint.is_none());

        // Case: fund is not a number
        let hint = AdviceEngine::check_param_swap("沪深300", "1000");
        assert!(hint.is_none());

        // Case: value is not 6 digits
        let hint = AdviceEngine::check_param_swap("1000", "520");
        assert!(hint.is_none());

        // Case: both are numbers but fund is 6 digits and value is not
        let hint = AdviceEngine::check_param_swap("123456", "999");
        assert!(hint.is_none());

        // Case: empty strings
        let hint = AdviceEngine::check_param_swap("", "");
        assert!(hint.is_none());
    }

    #[test]
    fn test_advice_engine_check_param_swap_format() {
        let hint = AdviceEngine::check_param_swap("500", "000300").unwrap();
        assert!(hint.contains("500"));
        assert!(hint.contains("000300"));
        assert!(hint.contains("fund"));
        // The format is "fund ... {} {}", which is "fund ... 000300 500"
        assert!(hint.contains("000300 500"));
    }

    #[tokio::test]
    async fn test_resolve_fund_local_exact_match_by_code() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // Setup fund
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

        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let result = resolve_fund(&conn, "000300", false).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        assert!(result.is_ok());
        let fund = result.unwrap();
        assert_eq!(fund.code, "000300");
        assert_eq!(fund.name, "沪深300");
    }

    #[tokio::test]
    async fn test_resolve_fund_local_exact_match_by_name() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

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

        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let result = resolve_fund(&conn, "沪深300", false).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        assert!(result.is_ok());
        let fund = result.unwrap();
        assert_eq!(fund.code, "000300");
    }

    #[tokio::test]
    async fn test_resolve_fund_not_found() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // No funds in DB, local_only = true
        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let result = resolve_fund(&conn, "999999", true).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        assert!(result.is_err());
        match result {
            Err(ResolveError::NotFound(_)) => {}
            _ => panic!("Expected NotFound error"),
        }
    }

    #[tokio::test]
    async fn test_resolve_fund_local_only_no_fetch() {
        use tempfile::NamedTempFile;
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        db::init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).expect("Failed to open DB");

        // 6-digit code but local_only = true (should not try to fetch)
        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let result = resolve_fund(&conn, "000300", true).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        // Should not find since fund doesn't exist locally
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_maybe_sync_fund_skips_sync_with_env() {
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

        unsafe {
            std::env::set_var("SKIP_SYNC", "1");
        }
        let fund = db::get_fund_by_code_or_name(&conn, "000300")
            .unwrap()
            .unwrap();
        let result = maybe_sync_fund(&conn, fund).await;
        unsafe {
            std::env::remove_var("SKIP_SYNC");
        }

        assert!(result.is_ok());
    }

    #[test]
    fn test_resolve_error_display() {
        let err = ResolveError::NotFound("000300".to_string());
        assert!(err.to_string().contains("未找到基金"));
        assert!(err.to_string().contains("000300"));

        let err = ResolveError::Ambiguous(
            "300".to_string(),
            vec![
                "沪深300 (000300)".to_string(),
                "易方达300 (001512)".to_string(),
            ],
        );
        assert!(err.to_string().contains("存在歧义"));

        let err = ResolveError::FetchFailed("000300".to_string(), "网络错误".to_string());
        assert!(err.to_string().contains("拉取基金"));

        let err = ResolveError::DatabaseError("连接失败".to_string());
        assert!(err.to_string().contains("数据库错误"));
    }
}
