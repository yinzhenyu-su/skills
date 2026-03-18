use super::{FundData, Provider};
use async_trait::async_trait;
use once_cell::sync::Lazy;
use regex::Regex;
use rust_decimal::Decimal;
use serde::Deserialize;
use std::str::FromStr;

static JS_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"jsonpgz\((.*)\);").unwrap());

pub struct EastmoneyJsProvider;

#[derive(Deserialize)]
struct JsFundResponse {
    pub fundcode: String,
    pub name: String,
    pub jzrq: String,
    pub dwjz: String,
}

#[async_trait]
impl Provider for EastmoneyJsProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        let url = format!("https://fundgz.1234567.com.cn/js/{}.js", code);
        let client = crate::provider::build_http_client()?;

        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        
        let status = resp.status();
        if !status.is_success() {
            return Err(format!("Eastmoney JS API returned error: {}", status));
        }

        // Handle the problematic "UTF-8,gbk" content type by falling back to UTF-8
        let body = if let Some(ct) = resp.headers().get(reqwest::header::CONTENT_TYPE) {
            let ct_str = ct.to_str().unwrap_or("");
            if ct_str.contains("UTF-8,gbk") {
                // If it's the problematic combo, manually decode as UTF-8
                let bytes = resp.bytes().await.map_err(|e| e.to_string())?;
                String::from_utf8_lossy(&bytes).into_owned()
            } else {
                resp.text().await.map_err(|e| e.to_string())?
            }
        } else {
            resp.text().await.map_err(|e| e.to_string())?
        };

        parse_js_response(&body)
    }
}

pub fn parse_js_response(body: &str) -> Result<FundData, String> {
    let caps = JS_RE
        .captures(body)
        .ok_or("Failed to match jsonpgz pattern")?;
    let json_str = caps
        .get(1)
        .ok_or("Failed to extract JSON from JS")?
        .as_str();

    let resp: JsFundResponse = serde_json::from_str(json_str).map_err(|e| e.to_string())?;

    Ok(FundData {
        code: resp.fundcode,
        name: Some(resp.name),
        nav: Some(Decimal::from_str(&resp.dwjz).map_err(|e| e.to_string())?),
        date: Some(resp.jzrq),
        ..Default::default()
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_js_response_valid() {
        let body = include_str!("../../tests/fixtures/eastmoney_js_sample.js");
        let data = parse_js_response(body).unwrap();
        assert_eq!(data.code, "160119");
        assert_eq!(data.name.unwrap(), "南方中证500ETF联接A");
        assert_eq!(data.nav.unwrap().to_string(), "1.3705");
        assert_eq!(data.date.unwrap(), "2025-03-07");
    }

    #[test]
    fn test_parse_js_response_malformed() {
        let body = r#"invalid_function({"fundcode":"123"});"#;
        let res = parse_js_response(body);
        assert!(res.is_err());
    }
}
