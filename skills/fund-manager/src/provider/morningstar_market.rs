use serde::Deserialize;

#[derive(Debug, Clone, Deserialize)]
pub struct TimeSeriesPoint {
    pub p: f64, // Price
    pub t: String, // Time
}

#[derive(Debug, Clone, Deserialize)]
pub struct IndexItem {
    pub name: String,
    pub price: f64,
    pub chg: f64,
    pub pct: f64,
    #[serde(rename = "w52h")]
    pub w52_high: Option<f64>,
    #[serde(rename = "w52l")]
    pub w52_low: Option<f64>,
    pub status: Option<i32>,
    pub cur: Option<String>,
    pub ts: Option<Vec<TimeSeriesPoint>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MarketCategory {
    ChinaEquity,
    GlobalEquity,
    Forex,
    Commodity,
    HotAssets,
}

impl MarketCategory {
    pub fn to_str(&self) -> &'static str {
        match self {
            MarketCategory::ChinaEquity => "中国股市",
            MarketCategory::GlobalEquity => "全球股市",
            MarketCategory::Forex => "外汇与汇率",
            MarketCategory::Commodity => "大宗商品",
            MarketCategory::HotAssets => "热门资产",
        }
    }
}

#[derive(Debug, Clone)]
pub struct MarketItem {
    pub name: String,
    pub price: f64,
    pub change: f64,
    pub pct: f64,
    pub w52_high: Option<f64>,
    pub w52_low: Option<f64>,
    pub status: Option<i32>,
    pub currency: Option<String>,
    pub trend: Option<Vec<f64>>,
    pub category: MarketCategory,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WatchListData {
    pub global_equity: Vec<IndexItem>,
    pub china_equity: Vec<IndexItem>,
    pub exchange_rate: Option<Vec<IndexItem>>,
    pub commodity: Option<Vec<IndexItem>>,
    pub hot_assets: Option<Vec<IndexItem>>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct WatchListResponse {
    pub data: WatchListData,
}

pub async fn fetch_market_data(
    names: Option<&[String]>,
    categories: Option<&[MarketCategory]>,
) -> Result<Vec<MarketItem>, String> {
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

    let mut items = Vec::new();

    let should_include_name = |name: &str| -> bool {
        match names {
            Some(filter_names) => filter_names.iter().any(|n| n == name),
            None => true,
        }
    };

    let should_include_category = |cat: MarketCategory| -> bool {
        match categories {
            Some(filter_cats) => filter_cats.contains(&cat),
            None => true,
        }
    };

    let mut process_list = |list: Vec<IndexItem>, cat: MarketCategory| {
        if should_include_category(cat) {
            for item in list {
                if should_include_name(&item.name) {
                    items.push(MarketItem {
                        name: item.name,
                        price: item.price,
                        change: item.chg,
                        pct: item.pct,
                        w52_high: item.w52_high,
                        w52_low: item.w52_low,
                        status: item.status,
                        currency: item.cur,
                        trend: item.ts.map(|ts| ts.into_iter().map(|p| p.p).collect()),
                        category: cat,
                    });
                }
            }
        }
    };

    process_list(result.data.china_equity, MarketCategory::ChinaEquity);
    process_list(result.data.global_equity, MarketCategory::GlobalEquity);
    
    if let Some(list) = result.data.exchange_rate {
        process_list(list, MarketCategory::Forex);
    }
    if let Some(list) = result.data.commodity {
        process_list(list, MarketCategory::Commodity);
    }
    if let Some(list) = result.data.hot_assets {
        process_list(list, MarketCategory::HotAssets);
    }

    Ok(items)
}

pub async fn fetch_market_items_by_names(names: &[String]) -> Result<Vec<MarketItem>, String> {
    fetch_market_data(Some(names), None).await
}

pub async fn fetch_market_item_by_name(name: &str) -> Result<Option<MarketItem>, String> {
    let names = vec![name.to_string()];
    let mut items = fetch_market_items_by_names(&names).await?;
    Ok(items.pop())
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
                        "pct": 1.026,
                        "w52h": 4197.228,
                        "w52l": 3040.6932,
                        "status": 0
                    }
                ],
                "globalEquity": [
                    {
                        "name": "标普500",
                        "price": 6556.37,
                        "chg": -24.63,
                        "pct": -0.3743,
                        "status": 1
                    }
                ],
                "exchangeRate": [
                    {
                        "name": "美元/人民币",
                        "price": 6.8967,
                        "chg": -0.2449,
                        "pct": 0.074,
                        "ts": [
                            {"p": 6.8914, "t": "2026-03-25 00:00:00"},
                            {"p": 6.8915, "t": "2026-03-25 00:10:00"}
                        ]
                    }
                ]
            }
        }"#;

        let parsed: Result<WatchListResponse, _> = serde_json::from_str(json_data);
        assert!(parsed.is_ok());
        let res = parsed.unwrap();
        assert_eq!(res.data.china_equity.len(), 1);
        assert_eq!(res.data.china_equity[0].name, "上证指数");
        assert_eq!(res.data.china_equity[0].w52_high, Some(4197.228));
        
        assert_eq!(res.data.global_equity.len(), 1);
        assert_eq!(res.data.global_equity[0].status, Some(1));

        assert!(res.data.exchange_rate.is_some());
        let fx = res.data.exchange_rate.unwrap();
        assert_eq!(fx[0].name, "美元/人民币");
        assert!(fx[0].ts.is_some());
        assert_eq!(fx[0].ts.as_ref().unwrap().len(), 2);
    }
}
