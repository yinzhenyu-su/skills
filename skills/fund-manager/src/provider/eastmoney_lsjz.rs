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
    #[serde(rename = "TotalCount")]
    pub TotalCount: i32,
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
    async fn fetch_page(
        &self,
        code: &str,
        page_index: i32,
        page_size: i32,
        start: Option<&str>,
        end: Option<&str>,
    ) -> Result<LsjzResponse, String> {
        let mut url = format!(
            "https://api.fund.eastmoney.com/f10/lsjz?fundCode={}&pageIndex={}&pageSize={}",
            code, page_index, page_size
        );
        if let Some(s) = start {
            url.push_str(&format!("&startDate={}", s));
        }
        if let Some(e) = end {
            url.push_str(&format!("&endDate={}", e));
        }

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
        Ok(res)
    }

    pub async fn fetch_by_date(&self, code: &str, date: &str) -> Result<FundData, String> {
        let res = self.fetch_page(code, 1, 20, Some(date), Some(date)).await?;
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
        let res = self.fetch_page(code, 1, 1, None, None).await?;
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
        let page_size = 20;
        let first_page = self
            .fetch_page(code, 1, page_size, Some(start), Some(end))
            .await?;

        let total_count = first_page.TotalCount;
        let mut all_items = Vec::new();

        if let Some(data) = first_page.Data {
            all_items.extend(data.LSJZList);
        }

        let total_pages = (total_count as f64 / page_size as f64).ceil() as i32;

        for page_index in 2..=total_pages {
            let page_res = self
                .fetch_page(code, page_index, page_size, Some(start), Some(end))
                .await?;
            if let Some(data) = page_res.Data {
                all_items.extend(data.LSJZList);
            }
        }

        let results: Vec<FundData> = all_items
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_jsonp_regex_matches() {
        let jsonp_body = r#"jQuery1234567890_1234567890({"Data":{"LSJZList":[]},"TotalCount":0})"#;
        let caps = JSONP_RE.captures(jsonp_body);
        assert!(caps.is_some());
        let caps = caps.unwrap();
        assert!(caps.get(1).is_some());
    }

    #[test]
    fn test_jsonp_regex_non_jsonp() {
        // Plain JSON should not match
        let json_body = r#"{"Data":{"LSJZList":[]},"TotalCount":0}"#;
        let caps = JSONP_RE.captures(json_body);
        // This regex won't match non-JSONP, but our code handles this case
        // by using body directly if not starting with jQuery
        assert!(caps.is_none());
    }

    #[test]
    fn test_lsjz_response_deserialization() {
        let json = r#"{
            "Data": {
                "LSJZList": [
                    {"FSRQ": "2026-03-09", "DWJZ": "1.5000", "LJJZ": "1.6500"}
                ]
            },
            "TotalCount": 1
        }"#;

        let res: LsjzResponse = serde_json::from_str(json).unwrap();
        assert_eq!(res.TotalCount, 1);
        let data = res.Data.unwrap();
        assert_eq!(data.LSJZList.len(), 1);
        assert_eq!(data.LSJZList[0].FSRQ, "2026-03-09");
        assert_eq!(data.LSJZList[0].DWJZ, "1.5000");
        assert_eq!(data.LSJZList[0].LJJZ, "1.6500");
    }

    #[test]
    fn test_lsjz_item_deserialization() {
        let json = r#"{"FSRQ": "2026-03-09", "DWJZ": "1.5000", "LJJZ": "1.6500"}"#;
        let item: LsjzItem = serde_json::from_str(json).unwrap();
        assert_eq!(item.FSRQ, "2026-03-09");
        assert_eq!(item.DWJZ, "1.5000");
        assert_eq!(item.LJJZ, "1.6500");
    }

    #[test]
    fn test_lsjz_response_empty_list() {
        let json = r#"{"Data":{"LSJZList":[]},"TotalCount":0}"#;
        let res: LsjzResponse = serde_json::from_str(json).unwrap();
        assert_eq!(res.TotalCount, 0);
        assert!(res.Data.unwrap().LSJZList.is_empty());
    }

    #[test]
    fn test_lsjz_response_no_data() {
        let json = r#"{"Data":null,"TotalCount":0}"#;
        let res: LsjzResponse = serde_json::from_str(json).unwrap();
        assert!(res.Data.is_none());
        assert_eq!(res.TotalCount, 0);
    }
}
