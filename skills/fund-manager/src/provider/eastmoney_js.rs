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
        let body = resp.text().await.map_err(|e| e.to_string())?;
        parse_js_response(&body)
    }
}

pub fn parse_js_response(body: &str) -> Result<FundData, String> {
    let caps = JS_RE.captures(body).ok_or("Failed to match jsonpgz pattern")?;
    let json_str = caps.get(1).ok_or("Failed to extract JSON from JS")?.as_str();
    
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
        let body = r#"jsonpgz({"fundcode":"160119","name":"南方中证500ETF联接A","jzrq":"2025-03-07","dwjz":"1.3705","gsz":"1.3705","gszzl":"0.00%","gztime":"2025-03-07 15:00"});"#;
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
