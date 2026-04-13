use crate::db::Holding;
use crate::provider::morningstar::MorningstarProvider;
use crate::provider::morningstar_market::{fetch_market_items_by_names, MarketItem};
use crate::provider::{FundData, Provider};
use futures::future::join_all;
use rust_decimal::Decimal;
use rust_decimal::prelude::FromPrimitive;
use rust_decimal_macros::dec;
use std::collections::{HashMap, HashSet};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RealtimeValuationStatus {
    Estimated,
    MarketClosed,
    UnsupportedType,
    NoReliableEstimate,
    MissingConfirmedNav,
}

#[derive(Debug, Clone)]
pub struct RealtimeValuation {
    pub estimated_nav: Option<Decimal>,
    pub estimated_value: Option<Decimal>,
    pub intraday_profit: Option<Decimal>,
    pub intraday_profit_pct: Option<Decimal>,
    pub index_name: Option<String>,
    pub benchmark_name: Option<String>,
    pub status: RealtimeValuationStatus,
    pub note: String,
    pub is_proxy: bool,
}

#[derive(Debug, Clone)]
struct IndexMapping {
    market_name: String,
    is_proxy: bool,
}

pub async fn estimate_holdings(
    holdings: &[Holding],
) -> Result<HashMap<String, RealtimeValuation>, String> {
    let metadata_results = join_all(holdings.iter().map(|holding| async move {
        let provider = MorningstarProvider;
        let result = provider.fetch(&holding.fund_code).await.ok();
        (holding.fund_code.clone(), result)
    }))
    .await;

    let metadata_map: HashMap<String, Option<FundData>> = metadata_results.into_iter().collect();

    let mut requested_market_names = HashSet::new();
    let mut resolved_mappings = HashMap::new();

    for holding in holdings {
        let metadata = metadata_map
            .get(&holding.fund_code)
            .and_then(|data| data.as_ref());
        if let Some(mapping) = resolve_index_mapping(
            &holding.fund_code,
            &holding.fund_name,
            metadata.and_then(|data| data.fund_type.as_deref()),
            metadata.and_then(|data| data.benchmark_name.as_deref()),
        ) {
            requested_market_names.insert(mapping.market_name.clone());
            resolved_mappings.insert(holding.fund_code.clone(), mapping);
        }
    }

    let requested_names: Vec<String> = requested_market_names.into_iter().collect();
    let market_items = if requested_names.is_empty() {
        Vec::new()
    } else {
        fetch_market_items_by_names(&requested_names).await?
    };
    let market_map: HashMap<String, MarketItem> = market_items
        .into_iter()
        .map(|item| (item.name.clone(), item))
        .collect();

    let mut valuations = HashMap::new();

    for holding in holdings {
        let metadata = metadata_map
            .get(&holding.fund_code)
            .and_then(|data| data.as_ref());
        let benchmark_name = metadata.and_then(|data| data.benchmark_name.clone());

        let valuation = if holding.latest_nav.is_none() {
            RealtimeValuation {
                estimated_nav: None,
                estimated_value: None,
                intraday_profit: None,
                intraday_profit_pct: None,
                index_name: None,
                benchmark_name,
                status: RealtimeValuationStatus::MissingConfirmedNav,
                note: "缺少确认净值".to_string(),
                is_proxy: false,
            }
        } else if let Some(reason) = unsupported_reason(
            metadata.and_then(|data| data.fund_type.as_deref()),
            &holding.fund_name,
        ) {
            RealtimeValuation {
                estimated_nav: None,
                estimated_value: None,
                intraday_profit: None,
                intraday_profit_pct: None,
                index_name: None,
                benchmark_name,
                status: RealtimeValuationStatus::UnsupportedType,
                note: reason.to_string(),
                is_proxy: false,
            }
        } else if let Some(mapping) = resolved_mappings.get(&holding.fund_code) {
            match market_map.get(&mapping.market_name) {
                Some(item) if item.status == Some(0) => {
                    let confirmed_nav = holding.latest_nav.unwrap_or_default();
                    let confirmed_value = (holding.shares * confirmed_nav).round_dp(2);
                    let pct = Decimal::from_f64(item.pct / 100.0).unwrap_or(Decimal::ZERO);
                    let estimated_nav = (confirmed_nav * (dec!(1) + pct)).round_dp(4);
                    let estimated_value = (holding.shares * estimated_nav).round_dp(2);
                    let intraday_profit = (estimated_value - confirmed_value).round_dp(2);
                    let intraday_profit_pct = if confirmed_value.is_zero() {
                        None
                    } else {
                        Some(((intraday_profit / confirmed_value) * dec!(100)).round_dp(2))
                    };
                    let note = if mapping.is_proxy {
                        format!("{}（代理，仅供参考）", mapping.market_name)
                    } else {
                        mapping.market_name.clone()
                    };

                    RealtimeValuation {
                        estimated_nav: Some(estimated_nav),
                        estimated_value: Some(estimated_value),
                        intraday_profit: Some(intraday_profit),
                        intraday_profit_pct,
                        index_name: Some(mapping.market_name.clone()),
                        benchmark_name,
                        status: RealtimeValuationStatus::Estimated,
                        note,
                        is_proxy: mapping.is_proxy,
                    }
                }
                Some(_) => RealtimeValuation {
                    estimated_nav: None,
                    estimated_value: None,
                    intraday_profit: None,
                    intraday_profit_pct: None,
                    index_name: Some(mapping.market_name.clone()),
                    benchmark_name,
                    status: RealtimeValuationStatus::MarketClosed,
                    note: format!("{} 休市", mapping.market_name),
                    is_proxy: mapping.is_proxy,
                },
                None => RealtimeValuation {
                    estimated_nav: None,
                    estimated_value: None,
                    intraday_profit: None,
                    intraday_profit_pct: None,
                    index_name: Some(mapping.market_name.clone()),
                    benchmark_name,
                    status: RealtimeValuationStatus::NoReliableEstimate,
                    note: format!("{} 行情不可用", mapping.market_name),
                    is_proxy: mapping.is_proxy,
                },
            }
        } else {
            RealtimeValuation {
                estimated_nav: None,
                estimated_value: None,
                intraday_profit: None,
                intraday_profit_pct: None,
                index_name: None,
                benchmark_name,
                status: RealtimeValuationStatus::NoReliableEstimate,
                note: "无可靠实时估算".to_string(),
                is_proxy: false,
            }
        };

        valuations.insert(holding.fund_code.clone(), valuation);
    }

    Ok(valuations)
}

fn resolve_index_mapping(
    fund_code: &str,
    fund_name: &str,
    fund_type: Option<&str>,
    benchmark_name: Option<&str>,
) -> Option<IndexMapping> {
    if let Some(mapping) = manual_override(fund_code, fund_name) {
        return Some(mapping);
    }

    if unsupported_reason(fund_type, fund_name).is_some() {
        return None;
    }

    benchmark_name
        .and_then(map_text_to_index)
        .or_else(|| map_text_to_index(fund_name))
}

fn manual_override(fund_code: &str, fund_name: &str) -> Option<IndexMapping> {
    match fund_code {
        "020989" => Some(IndexMapping {
            market_name: "恒生科技".to_string(),
            is_proxy: false,
        }),
        "016453" => Some(IndexMapping {
            market_name: "纳斯达克综合".to_string(),
            is_proxy: true,
        }),
        "009504" => Some(IndexMapping {
            market_name: "COMEX黄金".to_string(),
            is_proxy: true,
        }),
        _ if fund_name.contains("恒生科技") => Some(IndexMapping {
            market_name: "恒生科技".to_string(),
            is_proxy: false,
        }),
        _ if fund_name.contains("纳斯达克100") || fund_name.contains("纳斯达克") => {
            Some(IndexMapping {
                market_name: "纳斯达克综合".to_string(),
                is_proxy: true,
            })
        }
        _ if fund_name.contains("上海金") || fund_name.contains("黄金") => Some(IndexMapping {
            market_name: "COMEX黄金".to_string(),
            is_proxy: true,
        }),
        _ => None,
    }
}

fn unsupported_reason(fund_type: Option<&str>, fund_name: &str) -> Option<&'static str> {
    let kind = fund_type.unwrap_or_default();
    if kind.contains("货币") {
        return Some("货币基金不支持实时估算");
    }
    if kind.contains("债") || kind.contains("可转债") {
        return Some("债券基金仅显示确认净值");
    }
    if kind.contains("保守混合") {
        return Some("保守混合基金无可靠实时估算");
    }
    if kind.contains("行业股票") || kind.contains("行业混合") || fund_name.contains("消费电子") {
        return Some("行业基金无可靠实时估算");
    }
    if kind.contains("QDII")
        && !fund_name.contains("纳斯达克")
        && !fund_name.contains("恒生科技")
        && !fund_name.contains("黄金")
    {
        return Some("该 QDII 基金缺少可靠代理指数");
    }
    None
}

fn map_text_to_index(text: &str) -> Option<IndexMapping> {
    let normalized = normalize_text(text);
    if normalized.contains("沪深300") {
        return Some(IndexMapping {
            market_name: "沪深300".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("中证500") {
        return Some(IndexMapping {
            market_name: "中证500".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("中证1000") {
        return Some(IndexMapping {
            market_name: "中证1000".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("创业板") {
        return Some(IndexMapping {
            market_name: "创业板".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("恒生科技") {
        return Some(IndexMapping {
            market_name: "恒生科技".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("恒生指数") {
        return Some(IndexMapping {
            market_name: "恒生指数".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("标普500") {
        return Some(IndexMapping {
            market_name: "标普500".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("道琼斯") {
        return Some(IndexMapping {
            market_name: "道琼斯工业".to_string(),
            is_proxy: false,
        });
    }
    if normalized.contains("纳斯达克") || normalized.contains("msci美国") {
        return Some(IndexMapping {
            market_name: "纳斯达克综合".to_string(),
            is_proxy: true,
        });
    }
    if normalized.contains("日经225") {
        return Some(IndexMapping {
            market_name: "日经 225 指数".to_string(),
            is_proxy: false,
        });
    }
    None
}

fn normalize_text(text: &str) -> String {
    text.chars()
        .filter(|c| !c.is_whitespace())
        .collect::<String>()
        .to_lowercase()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_manual_override_has_priority() {
        let mapping = resolve_index_mapping(
            "020989",
            "南方恒生科技ETF联接(QDII)C",
            Some("QDII行业股票"),
            Some("沪深300"),
        )
        .unwrap();

        assert_eq!(mapping.market_name, "恒生科技");
        assert!(!mapping.is_proxy);
    }

    #[test]
    fn test_benchmark_mapping_to_supported_index() {
        let mapping = resolve_index_mapping(
            "160119",
            "南方中证500ETF联接(LOF)A",
            Some("中盘平衡股票"),
            Some("中证500全收益指数"),
        )
        .unwrap();

        assert_eq!(mapping.market_name, "中证500");
        assert!(!mapping.is_proxy);
    }

    #[test]
    fn test_gold_fund_uses_proxy_market() {
        let mapping = resolve_index_mapping(
            "009504",
            "富国上海金ETF联接A",
            Some("商品 - 贵金属"),
            Some("沪深300"),
        )
        .unwrap();

        assert_eq!(mapping.market_name, "COMEX黄金");
        assert!(mapping.is_proxy);
    }

    #[test]
    fn test_industry_fund_is_not_estimated() {
        assert!(resolve_index_mapping(
            "018301",
            "华夏消费电子ETF联接C",
            Some("行业股票-科技、传媒及通讯"),
            Some("中证信息全收益"),
        )
        .is_none());
    }
}