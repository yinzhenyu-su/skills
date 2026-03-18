use serde::Deserialize;

pub struct MorningstarSearchProvider;

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub code: String,
    pub name: String,
    pub fund_type: Option<String>,
}

#[derive(Debug, Deserialize)]
struct MsResponse {
    data: Vec<MsItem>,
}

#[derive(Debug, Deserialize)]
struct MsItem {
    symbol: String,
    #[serde(rename = "fundNameArr")]
    name: String,
    #[serde(rename = "fundType")]
    fund_type: Option<String>,
}

impl MorningstarSearchProvider {
    pub async fn search(&self, text: &str) -> Result<Vec<SearchResult>, String> {
        let url = format!(
            "https://www.morningstar.cn/cn-api/public/v1/fund-cache/{}",
            urlencoding::encode(text)
        );

        let client = crate::provider::build_http_client()?;
        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;

        if !resp.status().is_success() {
            return Err(format!("Morningstar Search API error: {}", resp.status()));
        }

        let body: MsResponse = resp.json().await.map_err(|e| e.to_string())?;

        Ok(body
            .data
            .into_iter()
            .map(|item| SearchResult {
                code: item.symbol,
                name: item.name,
                fund_type: item.fund_type,
            })
            .collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_ms_response() {
        let body = r#"{
            "data": [
                {
                    "symbol": "160119",
                    "fundNameArr": "南方中证500ETF联接（LOF）A",
                    "fundType": "股票型"
                }
            ]
        }"#;
        let parsed: MsResponse = serde_json::from_str(body).unwrap();
        assert_eq!(parsed.data.len(), 1);
        assert_eq!(parsed.data[0].symbol, "160119");
        assert_eq!(parsed.data[0].name, "南方中证500ETF联接（LOF）A");
        assert_eq!(parsed.data[0].fund_type.as_deref(), Some("股票型"));
    }
}
