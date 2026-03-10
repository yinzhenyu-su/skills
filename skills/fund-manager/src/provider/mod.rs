use async_trait::async_trait;
use rust_decimal::Decimal;

#[derive(Debug, Clone)]
pub struct FundData {
    pub code: String,
    pub name: Option<String>,
    pub nav: Option<Decimal>,
    pub acc_nav: Option<Decimal>,
    pub fee_rate: Option<Decimal>,
    pub date: Option<String>,
}

#[async_trait]
pub trait Provider {
    async fn fetch(&self, code: &str) -> Result<FundData, String>;
}

pub mod eastmoney_js;
pub mod eastmoney_html;
pub mod aggregator;
