use super::{FundData, Provider};
use async_trait::async_trait;

pub struct Aggregator {
    pub providers: Vec<Box<dyn Provider + Send + Sync>>,
}

impl Aggregator {
    pub fn new() -> Self {
        Self { providers: Vec::new() }
    }

    pub fn add_provider(&mut self, provider: Box<dyn Provider + Send + Sync>) {
        self.providers.push(provider);
    }

    pub async fn fetch_all(&self, code: &str) -> Result<FundData, String> {
        let mut final_data = FundData {
            code: code.to_string(),
            name: None,
            nav: None,
            acc_nav: None,
            fee_rate: None,
            date: None,
        };

        let mut futures = Vec::new();
        for p in &self.providers {
            futures.push(p.fetch(code));
        }

        let results = tokio::time::timeout(
            tokio::time::Duration::from_secs(10),
            futures::future::join_all(futures)
        ).await.map_err(|_| "Fetch operation timed out".to_string())?;

        for res in results {
            if let Ok(data) = res {
                if data.name.is_some() { final_data.name = data.name; }
                if data.nav.is_some() { final_data.nav = data.nav; }
                if data.acc_nav.is_some() { final_data.acc_nav = data.acc_nav; }
                if data.fee_rate.is_some() { final_data.fee_rate = data.fee_rate; }
                if data.date.is_some() { final_data.date = data.date; }
            }
        }

        if final_data.name.is_none() && final_data.nav.is_none() {
            return Err("Failed to fetch any useful fund data".to_string());
        }

        Ok(final_data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use rust_decimal_macros::dec;
    use mockall::predicate::*;
    use mockall::*;

    mock! {
        pub MyProvider {}
        #[async_trait]
        impl Provider for MyProvider {
            async fn fetch(&self, code: &str) -> Result<FundData, String>;
        }
    }

    #[tokio::test]
    async fn test_aggregator_merging_logic() {
        let mut mock_js = MockMyProvider::new();
        mock_js.expect_fetch()
            .returning(|code| Ok(FundData {
                code: code.to_string(),
                name: Some("Fund".to_string()),
                nav: Some(dec!(1.0)),
                acc_nav: None,
                fee_rate: None,
                date: Some("2026-03-09".to_string()),
            }));

        let mut mock_html = MockMyProvider::new();
        mock_html.expect_fetch()
            .returning(|_| Err("HTML Error".to_string()));

        let mut aggregator = Aggregator::new();
        aggregator.add_provider(Box::new(mock_js));
        aggregator.add_provider(Box::new(mock_html));

        let res = aggregator.fetch_all("000300").await.unwrap();
        
        assert_eq!(res.name.unwrap(), "Fund");
        assert_eq!(res.nav.unwrap(), dec!(1.0));
        assert!(res.fee_rate.is_none());
    }

    #[tokio::test]
    #[ignore]
    async fn test_real_fetch_000300() {
        let mut aggregator = Aggregator::new();
        aggregator.add_provider(Box::new(crate::provider::eastmoney_js::EastmoneyJsProvider));
        aggregator.add_provider(Box::new(crate::provider::eastmoney_html::EastmoneyHtmlProvider));

        let res = aggregator.fetch_all("000300").await.unwrap();
        assert_eq!(res.code, "000300");
        assert!(res.name.is_some());
        assert!(res.nav.is_some());
    }
}
