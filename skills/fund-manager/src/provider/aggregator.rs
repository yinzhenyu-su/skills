use super::{FundData, Provider};

pub struct Aggregator {
    pub providers: Vec<Box<dyn Provider + Send + Sync>>,
}

impl Aggregator {
    pub fn new() -> Self {
        Self {
            providers: Vec::new(),
        }
    }

    pub fn add_provider(&mut self, provider: Box<dyn Provider + Send + Sync>) {
        self.providers.push(provider);
    }

    pub async fn fetch_all(&self, code: &str) -> Result<FundData, String> {
        self.fetch_at_date(code, None).await
    }

    pub async fn fetch_at_date(&self, code: &str, date: Option<&str>) -> Result<FundData, String> {
        let mut final_data = FundData {
            code: code.to_string(),
            ..Default::default()
        };

        let mut futures = Vec::new();
        for p in &self.providers {
            if let Some(d) = date {
                futures.push(p.fetch_at_date(code, d));
            } else {
                futures.push(p.fetch(code));
            }
        }

        let results = tokio::time::timeout(
            tokio::time::Duration::from_secs(10),
            futures::future::join_all(futures),
        )
        .await
        .map_err(|_| "Fetch operation timed out".to_string())?;

        let mut errors = Vec::new();
        let mut success_count = 0;

        for res in results {
            match res {
                Ok(data) => {
                    success_count += 1;
                    if data.name.is_some() {
                        final_data.name = data.name;
                    }
                    if data.nav.is_some() {
                        final_data.nav = data.nav;
                    }
                    if data.acc_nav.is_some() {
                        final_data.acc_nav = data.acc_nav;
                    }
                    if data.fee_rate.is_some() {
                        final_data.fee_rate = data.fee_rate;
                    }
                    if data.date.is_some() {
                        final_data.date = data.date;
                    }
                    if data.fund_type.is_some() {
                        final_data.fund_type = data.fund_type;
                    }
                    if data.risk_level.is_some() {
                        final_data.risk_level = data.risk_level;
                    }
                    if data.manager.is_some() {
                        final_data.manager = data.manager;
                    }
                    if data.company.is_some() {
                        final_data.company = data.company;
                    }
                    if data.establish_date.is_some() {
                        final_data.establish_date = data.establish_date;
                    }
                    if data.mgmt_fee.is_some() {
                        final_data.mgmt_fee = data.mgmt_fee;
                    }
                    if data.trust_fee.is_some() {
                        final_data.trust_fee = data.trust_fee;
                    }
                    if data.sales_fee.is_some() {
                        final_data.sales_fee = data.sales_fee;
                    }
                    if data.snapshot_date.is_some() {
                        final_data.snapshot_date = data.snapshot_date;
                    }
                    if data.rating_3y.is_some() {
                        final_data.rating_3y = data.rating_3y;
                    }
                    if data.rating_5y.is_some() {
                        final_data.rating_5y = data.rating_5y;
                    }
                    if data.rank_pct_3y.is_some() {
                        final_data.rank_pct_3y = data.rank_pct_3y;
                    }
                    if data.sharpe_3y.is_some() {
                        final_data.sharpe_3y = data.sharpe_3y;
                    }
                    if data.calmar_3y.is_some() {
                        final_data.calmar_3y = data.calmar_3y;
                    }
                    if data.max_drawdown_3y.is_some() {
                        final_data.max_drawdown_3y = data.max_drawdown_3y;
                    }
                    if data.investor_gap_3y.is_some() {
                        final_data.investor_gap_3y = data.investor_gap_3y;
                    }
                }
                Err(e) => {
                    errors.push(e);
                }
            }
        }

        if success_count == 0 {
            return Err(format!("所有 Provider 均抓取失败: {}", errors.join("; ")));
        }

        if final_data.name.is_none() && final_data.nav.is_none() {
            return Err(format!(
                "未能获取到有效的基金数据。Provider 错误摘要: {}",
                errors.join("; ")
            ));
        }

        Ok(final_data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use mockall::predicate::*;
    use mockall::*;
    use rust_decimal_macros::dec;
    use async_trait::async_trait;

    mock! {
        pub MyProvider {}
        #[async_trait]
        impl Provider for MyProvider {
            async fn fetch(&self, code: &str) -> Result<FundData, String>;
            async fn fetch_at_date(&self, code: &str, date: &str) -> Result<FundData, String>;
        }
    }

    #[tokio::test]
    async fn test_aggregator_merging_logic() {
        let mut mock_js = MockMyProvider::new();
        mock_js.expect_fetch().returning(|code| {
            Ok(FundData {
                code: code.to_string(),
                name: Some("Fund".to_string()),
                nav: Some(dec!(1.0)),
                date: Some("2026-03-09".to_string()),
                ..Default::default()
            })
        });

        let mut mock_html = MockMyProvider::new();
        mock_html
            .expect_fetch()
            .returning(|_| Err("HTML Error".to_string()));

        let mut aggregator = Aggregator::new();
        aggregator.add_provider(Box::new(mock_js));
        aggregator.add_provider(Box::new(mock_html));

        let res = aggregator.fetch_all("000300").await.unwrap();

        assert_eq!(res.name.unwrap(), "Fund");
        assert_eq!(res.nav.unwrap(), dec!(1.0));
        assert!(res.fee_rate.is_none());
    }
}
