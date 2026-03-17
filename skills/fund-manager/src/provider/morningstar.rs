use super::{FundData, Provider};
use async_trait::async_trait;
use serde::Deserialize;

#[derive(Debug, Deserialize)]
struct MorningstarResponse {
    data: MorningstarData,
}

#[derive(Debug, Deserialize)]
struct MorningstarData {
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

pub struct MorningstarProvider;

#[async_trait]
impl Provider for MorningstarProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        let url = format!("https://www.morningstar.cn/cn-api/v2/funds/{}/performance", code);
        let client = super::build_http_client()?;
        
        let resp = client
            .get(&url)
            .header("Referer", "https://www.morningstar.cn/")
            .send()
            .await
            .map_err(|e| e.to_string())?;

        if !resp.status().is_success() {
            return Err(format!("Morningstar API error: {}", resp.status()));
        }

        let body: MorningstarResponse = resp.json().await.map_err(|e| e.to_string())?;
        let d = body.data;

        let mut data = FundData {
            code: code.to_string(),
            fund_type: d.category_name,
            ..Default::default()
        };

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

        Ok(data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_morningstar_json() {
        let json = r#"{
            "data": {
                "categoryName": "大盘成长股票",
                "rating": { "Y3": "5", "Y5": "4" },
                "risk": {
                    "Y3": {
                        "risk": {
                            "sharpeRatio": 1.25,
                            "calmarRatio": 0.85,
                            "maxDrawdown": -22.4,
                            "returnRankOver": 8.0,
                            "riskDate": "2024-02-28"
                        }
                    }
                },
                "investorReturn": {
                    "Y3": {
                        "investorReturn": 10.0,
                        "return": 15.0
                    }
                }
            }
        }"#;

        let body: MorningstarResponse = serde_json::from_str(json).unwrap();
        let d = body.data;

        assert_eq!(d.category_name.unwrap(), "大盘成长股票");
        assert_eq!(d.rating.unwrap().y3.unwrap(), "5");
        
        let risk = d.risk.unwrap().y3.unwrap().risk.unwrap();
        assert_eq!(risk.sharpe_ratio.unwrap(), 1.25);
        assert_eq!(risk.max_drawdown.unwrap(), -22.4);

        let ir = d.investor_return.unwrap().y3.unwrap();
        assert_eq!(ir.fund_return.unwrap() - ir.investor_return.unwrap(), 5.0);
    }
}
