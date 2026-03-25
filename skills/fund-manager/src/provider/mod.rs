use async_trait::async_trait;
use rust_decimal::Decimal;

#[derive(Debug, Clone, Default)]
pub struct FundData {
    pub code: String,
    pub name: Option<String>,
    pub nav: Option<Decimal>,
    pub acc_nav: Option<Decimal>,
    pub fee_rate: Option<Decimal>,
    pub date: Option<String>,
    pub fund_type: Option<String>,
    pub risk_level: Option<String>,
    pub manager: Option<String>,
    pub company: Option<String>,
    pub establish_date: Option<String>,
    pub mgmt_fee: Option<String>,
    pub trust_fee: Option<String>,
    pub sales_fee: Option<String>,
    // Analysis fields
    pub snapshot_date: Option<String>,
    pub rating_3y: Option<i32>,
    pub rating_5y: Option<i32>,
    pub rank_pct_3y: Option<f64>,
    pub sharpe_3y: Option<f64>,
    pub calmar_3y: Option<f64>,
    pub max_drawdown_3y: Option<f64>,
    pub investor_gap_3y: Option<f64>,
}

#[async_trait]
pub trait Provider {
    async fn fetch(&self, code: &str) -> Result<FundData, String>;
    async fn fetch_at_date(&self, code: &str, _date: &str) -> Result<FundData, String> {
        let _ = code;
        // Default: not supported by this provider
        Err("Date-specific fetch not supported by this provider".to_string())
    }
    async fn fetch_range(
        &self,
        code: &str,
        _start: &str,
        _end: &str,
    ) -> Result<Vec<FundData>, String> {
        // Default: not supported by this provider
        let _ = code;
        Err("Date-range fetch not supported by this provider".to_string())
    }
}

use reqwest::header::{HeaderMap, HeaderValue};

pub fn build_http_client() -> Result<reqwest::Client, String> {
    let mut headers = HeaderMap::new();
    headers.insert(
        "Referer",
        HeaderValue::from_static("https://fundf10.eastmoney.com/"),
    );

    reqwest::Client::builder()
        .user_agent(crate::config::get_user_agent())
        .default_headers(headers)
        .build()
        .map_err(|e| e.to_string())
}

pub mod aggregator;
pub mod eastmoney_details;
pub mod eastmoney_html;
pub mod eastmoney_js;
pub mod eastmoney_lsjz;
pub mod morningstar;
pub mod morningstar_market;
pub mod morningstar_search;
