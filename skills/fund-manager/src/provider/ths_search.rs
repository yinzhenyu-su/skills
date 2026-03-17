use once_cell::sync::Lazy;
use regex::Regex;

static SEARCH_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"jQuery\d+\((.*)\)").unwrap());

pub struct ThsSearchProvider;

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub code: String,
    pub name: String,
}

impl ThsSearchProvider {
    pub async fn search(&self, text: &str) -> Result<Vec<SearchResult>, String> {
        let url = format!(
            "https://news.10jqka.com.cn/public/index_keyboard.php?type=fund&search-text={}&jsoncallback=jQuery123",
            urlencoding::encode(text)
        );
        let client = crate::provider::build_http_client()?;

        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;
        parse_search_response(&body)
    }
}

pub fn parse_search_response(body: &str) -> Result<Vec<SearchResult>, String> {
    let caps = SEARCH_RE
        .captures(body)
        .ok_or("Failed to match search response pattern")?;
    let json_str = caps
        .get(1)
        .ok_or("Failed to extract JSON from search response")?
        .as_str();

    let results: Vec<String> = serde_json::from_str(json_str).map_err(|e| e.to_string())?;

    let mut parsed = Vec::new();
    for r in results {
        // Format example: "0||015000 \u534e\u6cf0\u4fdd\u5174\u5409\u5e74\u76c8\u6df7\u5408C \u57fa\u91d1"
        let parts: Vec<&str> = r.split("||").collect();
        if parts.len() >= 2 {
            let data = parts[1];
            let sub_parts: Vec<&str> = data.split_whitespace().collect();
            if sub_parts.len() >= 2 {
                parsed.push(SearchResult {
                    code: sub_parts[0].to_string(),
                    name: sub_parts[1].to_string(),
                });
            }
        }
    }
    Ok(parsed)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_search_response() {
        let body = r#"jQuery123(["0||015000 \u534e\u6cf0\u4fdd\u5174\u5409\u5e74\u76c8\u6df7\u5408C \u57fa\u91d1"])"#;
        let results = parse_search_response(body).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].code, "015000");
        assert_eq!(results[0].name, "华泰保兴吉年盈混合C");
    }
}
