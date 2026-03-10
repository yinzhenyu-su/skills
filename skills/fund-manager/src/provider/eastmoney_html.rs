use super::{FundData, Provider};
use async_trait::async_trait;
use scraper::{Html, Selector};
use rust_decimal::Decimal;
use std::str::FromStr;
use rust_decimal_macros::dec;

pub struct EastmoneyHtmlProvider;

#[async_trait]
impl Provider for EastmoneyHtmlProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        let url = format!("https://fund.eastmoney.com/{}.html", code);
        let client = reqwest::Client::builder()
            .user_agent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
            .build()
            .map_err(|e| e.to_string())?;

        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;
        
        let fee = parse_html_fee(&body).ok();
        
        Ok(FundData {
            code: code.to_string(),
            name: None,
            nav: None,
            acc_nav: None,
            fee_rate: fee,
            date: None,
        })
    }
}

pub fn parse_html_fee(body: &str) -> Result<Decimal, String> {
    let document = Html::parse_document(body);
    let selector = Selector::parse(".nowPrice").map_err(|e| e.to_string())?;
    
    if let Some(element) = document.select(&selector).next() {
        let text = element.text().collect::<Vec<_>>().join("");
        let clean_text = text.replace('%', "").trim().to_string();
        
        if clean_text == "免手续费" || clean_text == "0.00" {
            return Ok(Decimal::ZERO);
        }

        let val = Decimal::from_str(&clean_text).map_err(|e| format!("Invalid decimal: {} - {}", e, clean_text))?;
        Ok(val / dec!(100))
    } else {
        Err("Could not find .nowPrice element".to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_html_fee_valid() {
        let html = r#"<div class="buy-info"><span class="fee-list">折后：<span class="nowPrice">0.12%</span></span></div>"#;
        let fee = parse_html_fee(html).unwrap();
        assert_eq!(fee, dec!(0.0012));
    }

    #[test]
    fn test_parse_html_fee_missing() {
        let html = r#"<div>No fee here</div>"#;
        let res = parse_html_fee(html);
        assert!(res.is_err());
    }

    #[test]
    fn test_parse_html_fee_special_texts() {
        // "免手续费"
        let html = r#"<span class="nowPrice">免手续费</span>"#;
        let fee = parse_html_fee(html).unwrap();
        assert_eq!(fee, Decimal::ZERO);

        // "---" or other invalid decimals
        let html = r#"<span class="nowPrice">---</span>"#;
        let res = parse_html_fee(html);
        assert!(res.is_err());
    }
}
