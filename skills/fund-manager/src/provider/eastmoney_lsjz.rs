use super::{FundData, Provider};
use async_trait::async_trait;
use once_cell::sync::Lazy;
use regex::Regex;
use rust_decimal::Decimal;
use serde::Deserialize;
use std::str::FromStr;

static JSONP_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"jQuery\d+_\d+\((.*)\)").unwrap());

pub struct EastmoneyLsjzProvider;

#[derive(Deserialize, Debug)]
#[allow(non_snake_case)]
struct LsjzResponse {
    #[serde(rename = "Data")]
    pub Data: Option<LsjzData>,
}

#[derive(Deserialize, Debug)]
#[allow(non_snake_case)]
struct LsjzData {
    #[serde(rename = "LSJZList")]
    pub LSJZList: Vec<LsjzItem>,
}

#[derive(Deserialize, Debug)]
#[allow(non_snake_case)]
struct LsjzItem {
    #[serde(rename = "FSRQ")]
    pub FSRQ: String, // Date
    #[serde(rename = "DWJZ")]
    pub DWJZ: String, // NAV
    #[serde(rename = "LJJZ")]
    pub LJJZ: String, // Acc NAV
}

impl EastmoneyLsjzProvider {
    pub async fn fetch_by_date(&self, code: &str, date: &str) -> Result<FundData, String> {
        // Use a large pageSize or precise dates to ensure we get the right one
        let url = format!(
            "https://api.fund.eastmoney.com/f10/lsjz?fundCode={}&pageIndex=1&pageSize=20&startDate={}&endDate={}",
            code, date, date
        );

        let client = crate::provider::build_http_client()?;
        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;

        // The API might return JSON directly or JSONP depending on headers/params
        // If it starts with jQuery, parse as JSONP
        let json_str = if body.starts_with("jQuery") {
            let caps = JSONP_RE
                .captures(&body)
                .ok_or("Failed to match JSONP pattern in lsjz")?;
            caps.get(1)
                .ok_or("Failed to extract JSON from JSONP in lsjz")?
                .as_str()
                .to_string()
        } else {
            body
        };

        let res: LsjzResponse =
            serde_json::from_str(&json_str).map_err(|e| format!("{}: {}", e, json_str))?;
        let items = res.Data.ok_or("No Data in lsjz response")?.LSJZList;

        let item = items
            .into_iter()
            .find(|i| i.FSRQ == date)
            .ok_or_else(|| format!("No NAV found for date {}", date))?;

        Ok(FundData {
            code: code.to_string(),
            nav: Some(Decimal::from_str(&item.DWJZ).map_err(|e| e.to_string())?),
            acc_nav: Some(Decimal::from_str(&item.LJJZ).map_err(|e| e.to_string())?),
            date: Some(item.FSRQ),
            ..Default::default()
        })
    }
}

#[async_trait]
impl Provider for EastmoneyLsjzProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        // ... (existing implementation)
        let url = format!(
            "https://api.fund.eastmoney.com/f10/lsjz?fundCode={}&pageIndex=1&pageSize=1",
            code
        );
        let client = crate::provider::build_http_client()?;
        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;

        let json_str = if body.starts_with("jQuery") {
            let caps = JSONP_RE
                .captures(&body)
                .ok_or("Failed to match JSONP pattern in lsjz")?;
            caps.get(1)
                .ok_or("Failed to extract JSON from JSONP in lsjz")?
                .as_str()
                .to_string()
        } else {
            body
        };

        let res: LsjzResponse = serde_json::from_str(&json_str).map_err(|e| e.to_string())?;
        let items = res.Data.ok_or("No Data in lsjz response")?.LSJZList;
        let item = items.first().ok_or("No items in lsjz response")?;

        Ok(FundData {
            code: code.to_string(),
            nav: Some(Decimal::from_str(&item.DWJZ).map_err(|e| e.to_string())?),
            acc_nav: Some(Decimal::from_str(&item.LJJZ).map_err(|e| e.to_string())?),
            date: Some(item.FSRQ.clone()),
            ..Default::default()
        })
    }

    async fn fetch_at_date(&self, code: &str, date: &str) -> Result<FundData, String> {
        self.fetch_by_date(code, date).await
    }

    async fn fetch_range(
        &self,
        code: &str,
        start: &str,
        end: &str,
    ) -> Result<Vec<FundData>, String> {
        let url = format!(
            "https://api.fund.eastmoney.com/f10/lsjz?fundCode={}&pageIndex=1&pageSize=1000&startDate={}&endDate={}",
            code, start, end
        );

        let client = crate::provider::build_http_client()?;
        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;

        let json_str = if body.starts_with("jQuery") {
            let caps = JSONP_RE
                .captures(&body)
                .ok_or("Failed to match JSONP pattern in lsjz")?;
            caps.get(1)
                .ok_or("Failed to extract JSON from JSONP in lsjz")?
                .as_str()
                .to_string()
        } else {
            body
        };

        let res: LsjzResponse =
            serde_json::from_str(&json_str).map_err(|e| format!("{}: {}", e, json_str))?;
        let items = res.Data.ok_or("No Data in lsjz response")?.LSJZList;

        let results = items
            .into_iter()
            .map(|item| FundData {
                code: code.to_string(),
                nav: Decimal::from_str(&item.DWJZ).ok(),
                acc_nav: Decimal::from_str(&item.LJJZ).ok(),
                date: Some(item.FSRQ),
                ..Default::default()
            })
            .collect();

        Ok(results)
    }
}
