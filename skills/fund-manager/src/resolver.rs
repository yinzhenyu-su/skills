use crate::db::{self, Fund};
use crate::provider::aggregator::Aggregator;
use crate::provider::morningstar_search::MorningstarSearchProvider;
use chrono::{Duration, NaiveDateTime, Utc};
use clap::{Command, CommandFactory};
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

pub struct CommandSuggestion {
    pub hint: String,
    pub path: Option<Vec<String>>,
}

pub struct AdviceEngine;

impl AdviceEngine {
    pub fn format_clap_error(err: clap::Error) -> String {
        use clap::error::ErrorKind;
        use crate::cli::Cli;

        let mut output = String::new();
        let mut suggestion_path = None;

        match err.kind() {
            ErrorKind::UnknownArgument | ErrorKind::InvalidSubcommand => {
                let arg_str = err.context().find_map(|(k, v)| {
                    if let clap::error::ContextKind::InvalidArg | clap::error::ContextKind::InvalidSubcommand = k {
                        if let clap::error::ContextValue::String(s) = v {
                            return Some(s.to_string());
                        }
                    }
                    None
                }).unwrap_or_else(|| "未知参数".to_string());
                let arg = &arg_str;

                output.push_str(&format!("❌ 未识别的参数或子命令 '{}'\n", arg));

                // Try to find intent
                if let Some(suggestion) = Self::check_unexpected_arg_intent(arg) {
                    output.push_str(&format!("{}\n", suggestion.hint));
                    suggestion_path = suggestion.path;
                }
            }
            ErrorKind::MissingRequiredArgument => {
                let missing = err.context().find_map(|(k, v)| {
                    if let clap::error::ContextKind::InvalidArg = k {
                        if let clap::error::ContextValue::Strings(s) = v {
                            return Some(s.join(", "));
                        }
                    }
                    None
                }).unwrap_or_else(|| "必要参数".to_string());

                let chinese_missing = match missing.as_str() {
                    "<FUND>" => "基金标识符",
                    "<WALLET>" => "钱包名称",
                    "<NAME>" => "名称",
                    _ => &missing,
                };
                output.push_str(&format!("❌ 缺少{}参数\n", chinese_missing));
            }
            ErrorKind::DisplayHelpOnMissingArgumentOrSubcommand => {
                output.push_str("❌ 请提供一个子命令\n");
                output.push_str("💡 Hint: 运行 'fund --help' 查看可用命令列表。\n");
            }
            _ => {
                output.push_str(&format!("❌ 错误：{}\n", err));
            }
        }

        // Try to get usage from suggestion if available
        let mut usage_found = false;
        if let Some(path) = suggestion_path {
            let cmd = Cli::command();
            if let Some(target_cmd) = Self::find_command_by_path(&cmd, &path[1..]) {
                let mut target_cmd_clone = target_cmd.clone();
                let usage = target_cmd_clone.render_usage().to_string();
                let simplified = usage.replace("Usage:", "   用法示例：").replace("fund-manager", "fund");
                output.push_str(&simplified);
                usage_found = true;
            }
        }

        if !usage_found {
            // Add usage example from error if available
            let usage = err.context().find_map(|(k, v)| {
                if let clap::error::ContextKind::Usage = k {
                    if let clap::error::ContextValue::StyledStr(s) = v {
                        return Some(s.to_string());
                    }
                }
                None
            });

            if let Some(u) = usage {
                // Simplified usage string to just "用法示例: ..."
                let u_str: &str = &u;
                let simplified = u_str.replace("Usage:", "   用法示例：").replace("fund-manager", "fund");
                output.push_str(&simplified);
            } else {
                // Default usage based on command if possible
                output.push_str("   用法示例：fund --help");
            }
        }

        output
    }

    fn check_unexpected_arg_intent(input: &str) -> Option<CommandSuggestion> {
        use crate::cli::Cli;
        let cmd = Cli::command();
        
        // 1. Check if it's a misplaced subcommand (e.g., 'use' instead of 'wallet use')
        if let Some(path) = Self::find_subcommand_path(&cmd, input, vec!["fund".to_string()]) {
            // Only suggest if it's not just the input itself at the root
            if path.len() > 1 && path.last().unwrap() == input {
                return Some(CommandSuggestion {
                    hint: format!("💡 Hint: 你是不是想找：'{}'？", path.join(" ")),
                    path: Some(path),
                });
            }
        }

        // 2. Fuzzy match
        if let Some(suggestion) = Self::suggest_command_spelling(&cmd, input) {
            return Some(CommandSuggestion {
                hint: format!("❓ 未识别的子命令 '{}'。你是不是想找：'{}'？", input, suggestion),
                path: None, // Path not easily available for fuzzy match without deep search
            });
        }

        None
    }

    fn find_command_by_path<'a>(cmd: &'a Command, path: &[String]) -> Option<&'a Command> {
        if path.is_empty() {
            return Some(cmd);
        }
        for sub in cmd.get_subcommands() {
            if sub.get_name() == path[0] {
                return Self::find_command_by_path(sub, &path[1..]);
            }
        }
        None
    }

    fn find_subcommand_path(cmd: &Command, target: &str, current_path: Vec<String>) -> Option<Vec<String>> {
        for sub in cmd.get_subcommands() {
            let mut path = current_path.clone();
            path.push(sub.get_name().to_string());
            if sub.get_name() == target {
                return Some(path);
            }
            if let Some(found_path) = Self::find_subcommand_path(sub, target, path) {
                return Some(found_path);
            }
        }
        None
    }

    fn suggest_command_spelling(cmd: &Command, input: &str) -> Option<String> {
        let mut best_match: Option<String> = None;
        let mut min_dist = 3; // Max distance of 2

        for sub in cmd.get_subcommands() {
            let name = sub.get_name();
            let dist = strsim::levenshtein(input, name);
            if dist < min_dist {
                min_dist = dist;
                best_match = Some(name.to_string());
            }
        }

        best_match
    }

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
        db::insert_nav_history_idempotent(
            conn,
            &data.code,
            &date,
            &nav.to_string(),
            data.acc_nav.as_ref().map(|d| d.to_string()).as_deref(),
        )
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
            Some(&Utc::now().format("%Y-%m-%d %H:%M:%S").to_string()),
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
            Some(&Utc::now().format("%Y-%m-%d %H:%M:%S").to_string()),
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
            Some(&Utc::now().format("%Y-%m-%d %H:%M:%S").to_string()),
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
