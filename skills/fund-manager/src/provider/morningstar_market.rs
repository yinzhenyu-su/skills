use serde::Deserialize;

#[derive(Debug, Clone, Deserialize)]
pub struct IndexItem {
    pub name: String,
    pub price: f64,
    pub chg: f64,
    pub pct: f64,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WatchListData {
    pub global_equity: Vec<IndexItem>,
    pub china_equity: Vec<IndexItem>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct WatchListResponse {
    pub data: WatchListData,
}

#[derive(Debug, Clone)]
pub struct IndexData {
    pub name: String,
    pub current: f64,
    pub change: f64,
    pub change_percent: f64,
    pub market: String, // e.g. "中国股市", "全球股市"
}

pub async fn fetch_indices(names: Option<&[String]>) -> Result<Vec<IndexData>, String> {
    let client = reqwest::Client::builder()
        .user_agent(crate::config::get_user_agent())
        .build()
        .map_err(|e| format!("构建 HTTP 客户端失败: {}", e))?;

    let url = "https://www.morningstar.cn/cn-api/v2/market/watch-list";
    
    let resp = client
        .get(url)
        .send()
        .await
        .map_err(|e| format!("请求 Morningstar API 失败: {}", e))?;

    if !resp.status().is_success() {
        return Err(format!("API 返回错误状态码: {}", resp.status()));
    }

    let result: WatchListResponse = resp
        .json()
        .await
        .map_err(|e| format!("解析 Morningstar JSON 响应失败: {}", e))?;

    let mut indices = Vec::new();

    let should_include = |name: &str| -> bool {
        match names {
            Some(filter_names) => filter_names.iter().any(|n| n == name),
            None => true,
        }
    };

    for item in result.data.china_equity {
        if should_include(&item.name) {
            indices.push(IndexData {
                name: item.name,
                current: item.price,
                change: item.chg,
                change_percent: item.pct,
                market: "中国股市".to_string(),
            });
        }
    }

    for item in result.data.global_equity {
        if should_include(&item.name) {
            indices.push(IndexData {
                name: item.name,
                current: item.price,
                change: item.chg,
                change_percent: item.pct,
                market: "全球股市".to_string(),
            });
        }
    }

    Ok(indices)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_watchlist_response() {
        let json_data = r#"{
            "data": {
                "chinaEquity": [
                    {
                        "name": "上证指数",
                        "price": 3921.1003,
                        "chg": 39.8206,
                        "pct": 1.026
                    }
                ],
                "globalEquity": [
                    {
                        "name": "标普500",
                        "price": 6556.37,
                        "chg": -24.63,
                        "pct": -0.3743
                    }
                ]
            }
        }"#;

        let parsed: Result<WatchListResponse, _> = serde_json::from_str(json_data);
        assert!(parsed.is_ok());
        let res = parsed.unwrap();
        assert_eq!(res.data.china_equity.len(), 1);
        assert_eq!(res.data.china_equity[0].name, "上证指数");
        assert_eq!(res.data.china_equity[0].price, 3921.1003);
        
        assert_eq!(res.data.global_equity.len(), 1);
        assert_eq!(res.data.global_equity[0].name, "标普500");
    }
}
