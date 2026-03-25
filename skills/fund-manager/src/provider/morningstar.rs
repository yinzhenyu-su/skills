use super::{FundData, Provider};
use async_trait::async_trait;
use rust_decimal::prelude::{FromPrimitive, ToPrimitive};
use rust_decimal::Decimal;
use serde::Deserialize;
use serde_json::Value;
use std::str::FromStr;

#[derive(Debug, Deserialize)]
struct PerformanceResponse {
    data: PerformanceData,
}

#[derive(Debug, Deserialize)]
struct PerformanceData {
    #[serde(rename = "categoryName")]
    category_name: Option<String>,
    rating: Option<Rating>,
    risk: Option<RiskPeriods>,
    #[serde(rename = "investorReturn")]
    investor_return: Option<InvestorReturnPeriods>,
}

#[derive(Debug, Deserialize)]
struct Rating {
    #[serde(rename = "Y3")]
    y3: Option<String>,
    #[serde(rename = "Y5")]
    y5: Option<String>,
}

#[derive(Debug, Deserialize)]
struct RiskPeriods {
    #[serde(rename = "Y3")]
    y3: Option<RiskPeriod>,
}

#[derive(Debug, Deserialize)]
struct RiskPeriod {
    risk: Option<RiskMetrics>,
}

#[derive(Debug, Deserialize)]
struct RiskMetrics {
    #[serde(rename = "sharpeRatio")]
    sharpe_ratio: Option<f64>,
    #[serde(rename = "calmarRatio")]
    calmar_ratio: Option<f64>,
    #[serde(rename = "maxDrawdown")]
    max_drawdown: Option<f64>,
    #[serde(rename = "returnRankOver")]
    return_rank_over: Option<f64>,
    #[serde(rename = "riskDate")]
    risk_date: Option<String>,
}

#[derive(Debug, Deserialize)]
struct InvestorReturnPeriods {
    #[serde(rename = "Y3")]
    y3: Option<InvestorReturnMetrics>,
}

#[derive(Debug, Deserialize)]
struct InvestorReturnMetrics {
    #[serde(rename = "investorReturn")]
    investor_return: Option<f64>,
    #[serde(rename = "return")]
    fund_return: Option<f64>,
}

#[derive(Debug, Deserialize)]
struct CommonDataResponse {
    data: CommonData,
}

#[derive(Debug, Deserialize)]
struct CommonData {
    #[serde(rename = "morningstarCategory")]
    morningstar_category: Option<String>,
    #[serde(rename = "riskLevel")]
    risk_level: Option<String>,
    #[serde(rename = "managerName")]
    manager_name: Option<String>,
    #[serde(rename = "companyName")]
    company_name: Option<String>,
    #[serde(rename = "inceptionDate")]
    inception_date: Option<String>,
}

#[derive(Debug, Deserialize)]
struct FeesResponse {
    data: FeesData,
}

#[derive(Debug, Deserialize)]
struct FeesData {
    #[serde(rename = "managementFee")]
    management_fee: Option<String>,
    #[serde(rename = "custodianFee")]
    custodian_fee: Option<String>,
    #[serde(rename = "distributionFee")]
    distribution_fee: Option<String>,
    #[serde(rename = "minInvestment")]
    min_investment: Option<Value>,
    #[serde(rename = "frontLoadFee", default)]
    front_load_fee: Vec<FeeTierRaw>,
    #[serde(rename = "deferLoadFee", default)]
    defer_load_fee: Vec<FeeTierRaw>,
    #[serde(rename = "redemptionFee", default)]
    redemption_fee: Vec<FeeTierRaw>,
    #[serde(rename = "purchaseAndRedeem")]
    purchase_and_redeem: Option<PurchaseAndRedeemRaw>,
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct FeeTierRaw {
    floor: f64,
    fee: f64,
    #[serde(rename = "feeUnit")]
    fee_unit: f64,
    #[serde(rename = "floorUnit")]
    floor_unit: f64,
}

#[derive(Debug, Deserialize)]
#[allow(non_snake_case)]
struct PurchaseAndRedeemRaw {
    applyingMaxIII: Option<Value>,
    applyingMaxIV: Option<Value>,
    applyingMaxVII: Option<Value>,
    applyingMaxVIII: Option<Value>,
}

fn value_to_decimal(value: &Value) -> Option<Decimal> {
    match value {
        Value::Number(n) => n.as_f64().and_then(Decimal::from_f64),
        Value::String(s) => Decimal::from_str(s.trim()).ok(),
        _ => None,
    }
}

fn floor_to_days(floor: Decimal, floor_unit: f64) -> Option<i32> {
    if floor_unit == 10.0 {
        return floor.round_dp(0).to_i32();
    }

    if floor_unit == 4.0 {
        let days = floor * Decimal::from(30_i32);
        return days.round_dp(0).to_i32();
    }

    None
}

fn normalize_redemption_tiers(tiers: &[FeeTierRaw]) -> Vec<(i32, Option<i32>, Decimal)> {
    let mut normalized: Vec<(i32, Decimal)> = tiers
        .iter()
        .filter_map(|tier| {
            if tier.fee_unit != 2.0 {
                return None;
            }

            let min_days = floor_to_days(Decimal::from_f64(tier.floor)?, tier.floor_unit)?;
            let fee = Decimal::from_f64(tier.fee)? / Decimal::from(100_i32);
            Some((min_days, fee))
        })
        .collect();

    normalized.sort_by_key(|(min_days, _)| *min_days);

    normalized
        .iter()
        .enumerate()
        .map(|(idx, (min_days, fee))| {
            let max_days = normalized
                .get(idx + 1)
                .map(|(next_min, _)| (*next_min - 1).max(*min_days));
            (*min_days, max_days, *fee)
        })
        .collect()
}

fn extract_limit_per_transaction(raw: &PurchaseAndRedeemRaw) -> Option<Decimal> {
    [
        &raw.applyingMaxIII,
        &raw.applyingMaxIV,
        &raw.applyingMaxVII,
        &raw.applyingMaxVIII,
    ]
    .iter()
    .filter_map(|v| v.as_ref())
    .filter_map(value_to_decimal)
    .filter(|d| *d > Decimal::ZERO)
    .max()
}

pub struct MorningstarProvider;

#[async_trait]
impl Provider for MorningstarProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        let perf_url = format!(
            "https://www.morningstar.cn/cn-api/v2/funds/{}/performance",
            code
        );
        let common_url = format!(
            "https://www.morningstar.cn/cn-api/v2/funds/{}/common-data",
            code
        );
        let fees_url = format!("https://www.morningstar.cn/cn-api/v2/funds/{}/fees", code);

        let client = super::build_http_client()?;

        // Concurrent fetch
        let perf_fut = client
            .get(&perf_url)
            .header("Referer", "https://www.morningstar.cn/")
            .send();
        let common_fut = client
            .get(&common_url)
            .header("Referer", "https://www.morningstar.cn/")
            .send();
        let fees_fut = client
            .get(&fees_url)
            .header("Referer", "https://www.morningstar.cn/")
            .send();

        let (perf_res, common_res, fees_res) = tokio::join!(perf_fut, common_fut, fees_fut);

        let mut data = FundData {
            code: code.to_string(),
            ..Default::default()
        };

        // Parse Performance
        if let Ok(resp) = perf_res {
            if resp.status().is_success() {
                if let Ok(body) = resp.json::<PerformanceResponse>().await {
                    let d = body.data;
                    data.fund_type = d.category_name;
                    if let Some(rating) = d.rating {
                        data.rating_3y = rating.y3.and_then(|s| s.parse().ok());
                        data.rating_5y = rating.y5.and_then(|s| s.parse().ok());
                    }
                    if let Some(risk_periods) = d.risk {
                        if let Some(y3) = risk_periods.y3 {
                            if let Some(m) = y3.risk {
                                data.sharpe_3y = m.sharpe_ratio;
                                data.calmar_3y = m.calmar_ratio;
                                data.max_drawdown_3y = m.max_drawdown;
                                data.rank_pct_3y = m.return_rank_over;
                                data.snapshot_date = m.risk_date;
                            }
                        }
                    }
                    if let Some(ir_periods) = d.investor_return {
                        if let Some(y3) = ir_periods.y3 {
                            if let (Some(ir), Some(fr)) = (y3.investor_return, y3.fund_return) {
                                data.investor_gap_3y = Some(fr - ir);
                            }
                        }
                    }
                }
            }
        }

        // Parse Common Data
        if let Ok(resp) = common_res {
            if resp.status().is_success() {
                if let Ok(body) = resp.json::<CommonDataResponse>().await {
                    let d = body.data;
                    if d.morningstar_category.is_some() {
                        data.fund_type = d.morningstar_category;
                    }
                    data.risk_level = d.risk_level;
                    data.manager = d.manager_name;
                    data.company = d.company_name;
                    data.establish_date = d.inception_date;
                }
            }
        }

        // Parse Fees
        if let Ok(resp) = fees_res {
            if resp.status().is_success() {
                if let Ok(body) = resp.json::<FeesResponse>().await {
                    let d = body.data;
                    data.mgmt_fee = d.management_fee;
                    data.trust_fee = d.custodian_fee;
                    data.sales_fee = d.distribution_fee;
                    data.min_subscription_amount =
                        d.min_investment.as_ref().and_then(value_to_decimal);

                    if let Some(raw) = d.purchase_and_redeem.as_ref() {
                        data.limit_per_transaction = extract_limit_per_transaction(raw);
                    }

                    data.redemption_fee_tiers = normalize_redemption_tiers(&d.redemption_fee);

                    // Extract front load fee (first tier, typically 0-500k)
                    if let Some(first_tier) = d.front_load_fee.first() {
                        if let Some(rate) = Decimal::from_f64(first_tier.fee) {
                            if first_tier.fee_unit == 2.0 {
                                data.fee_rate = Some(rate / Decimal::from(100_i32));
                            } else {
                                data.fee_rate = Some(rate);
                            }
                        }
                    }
                }
            }
        }

        Ok(data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_morningstar_performance_json() {
        let json = r#"{
            "data": {
                "categoryName": "大盘成长股票",
                "rating": { "Y3": "5", "Y5": "4" },
                "risk": { "Y3": { "risk": { "sharpeRatio": 1.25, "maxDrawdown": -22.4 } } },
                "investorReturn": { "Y3": { "investorReturn": 10.0, "return": 15.0 } }
            }
        }"#;

        let body: PerformanceResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        assert_eq!(d.category_name.unwrap(), "大盘成长股票");
        assert_eq!(d.rating.unwrap().y3.unwrap(), "5");
    }

    #[test]
    fn test_parse_morningstar_common_data_json() {
        let json = r#"{
            "data": {
                "morningstarCategory": "QDII环球股票",
                "riskLevel": "中风险(R3)",
                "managerName": "夏宜冰",
                "companyName": "中银基金",
                "inceptionDate": "2011-03-03"
            }
        }"#;

        let body: CommonDataResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        assert_eq!(d.morningstar_category.unwrap(), "QDII环球股票");
        assert_eq!(d.risk_level.unwrap(), "中风险(R3)");
        assert_eq!(d.manager_name.unwrap(), "夏宜冰");
    }

    #[test]
    fn test_parse_morningstar_fees_json() {
        let json = r#"{
            "data": {
                "managementFee": "1.2%",
                "custodianFee": "0.2%",
                "distributionFee": "0.4%"
            }
        }"#;

        let body: FeesResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        assert_eq!(d.management_fee.unwrap(), "1.2%");
        assert_eq!(d.custodian_fee.unwrap(), "0.2%");
        assert_eq!(d.distribution_fee.unwrap(), "0.4%");
    }

    #[test]
    fn test_parse_front_load_fee() {
        let json = r#"{
            "data": {
                "managementFee": "1.2%",
                "custodianFee": "0.2%",
                "distributionFee": null,
                "frontLoadFee": [
                    {"floor": 0.0, "fee": 1.5, "feeUnit": 2.0, "floorUnit": 1.0},
                    {"floor": 500000.0, "fee": 1.2, "feeUnit": 2.0, "floorUnit": 1.0},
                    {"floor": 1000000.0, "fee": 0.6, "feeUnit": 2.0, "floorUnit": 1.0},
                    {"floor": 5000000.0, "fee": 1000.0, "feeUnit": 1.0, "floorUnit": 1.0}
                ]
            }
        }"#;

        let body: FeesResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        // Check front load fee tiers exist
        let tiers = d.front_load_fee;
        assert_eq!(tiers.len(), 4);

        // First tier: 0-500k, 1.5%
        let first = &tiers[0];
        assert_eq!(first.floor, 0.0);
        assert_eq!(first.fee, 1.5);
        assert_eq!(first.fee_unit, 2.0); // percentage

        // Fourth tier: fixed amount
        let fourth = &tiers[3];
        assert_eq!(fourth.fee, 1000.0);
        assert_eq!(fourth.fee_unit, 1.0); // fixed amount
    }

    #[test]
    fn test_parse_front_load_fee_empty() {
        let json = r#"{
            "data": {
                "managementFee": "0.5%",
                "custodianFee": "0.1%",
                "distributionFee": null
            }
        }"#;

        let body: FeesResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        // No frontLoadFee field means empty vector
        assert!(d.front_load_fee.is_empty());
    }

    #[test]
    fn test_normalize_redemption_tiers() {
        let tiers = vec![
            FeeTierRaw {
                floor: 0.0,
                fee: 1.5,
                fee_unit: 2.0,
                floor_unit: 10.0,
            },
            FeeTierRaw {
                floor: 7.0,
                fee: 0.5,
                fee_unit: 2.0,
                floor_unit: 10.0,
            },
            FeeTierRaw {
                floor: 30.0,
                fee: 0.0,
                fee_unit: 2.0,
                floor_unit: 10.0,
            },
        ];

        let normalized = normalize_redemption_tiers(&tiers);
        assert_eq!(normalized.len(), 3);
        assert_eq!(normalized[0].0, 0);
        assert_eq!(normalized[0].1, Some(6));
        assert_eq!(normalized[1].0, 7);
        assert_eq!(normalized[1].1, Some(29));
        assert_eq!(normalized[2].0, 30);
        assert_eq!(normalized[2].1, None);
    }
}
