use super::{FundData, Provider};
use async_trait::async_trait;
use serde::Deserialize;

pub struct EastmoneyDetailProvider;

#[derive(Deserialize)]
struct DetailResponse {
    #[serde(rename = "Datas")]
    datas: Option<DetailDatas>,
}

#[derive(Deserialize, Debug)]
#[allow(non_snake_case)]
struct DetailDatas {
    #[serde(rename = "FCODE")]
    FCODE: String,
    #[serde(rename = "SHORTNAME")]
    SHORTNAME: String,
    #[serde(rename = "FTYPE")]
    FTYPE: String,
    #[serde(rename = "RISKLEVEL")]
    RISKLEVEL: String,
    #[serde(rename = "JJGS")]
    JJGS: String,
    #[serde(rename = "JJJL")]
    JJJL: String,
    #[serde(rename = "ESTABDATE")]
    ESTABDATE: String,
    #[serde(rename = "MGREXP")]
    MGREXP: String,
    #[serde(rename = "TRUSTEXP")]
    TRUSTEXP: String,
    #[serde(rename = "SALESEXP")]
    SALESEXP: String,
}

#[async_trait]
impl Provider for EastmoneyDetailProvider {
    async fn fetch(&self, code: &str) -> Result<FundData, String> {
        let url = format!(
            "https://fundmobapi.eastmoney.com/FundMNewApi/FundMNDetailInformation?FCODE={}&deviceid=123456&plat=Android&product=EFund&version=6.5.5",
            code
        );
        let client = crate::provider::build_http_client()?;

        let resp = client.get(url).send().await.map_err(|e| e.to_string())?;
        let body = resp.text().await.map_err(|e| e.to_string())?;
        parse_detail_response(&body)
    }
}

pub fn parse_detail_response(body: &str) -> Result<FundData, String> {
    let resp: DetailResponse = serde_json::from_str(body).map_err(|e| e.to_string())?;
    let d = resp.datas.ok_or("No 'Datas' in detail response")?;

    Ok(FundData {
        code: d.FCODE,
        name: Some(d.SHORTNAME),
        fund_type: Some(d.FTYPE),
        risk_level: Some(d.RISKLEVEL),
        manager: Some(d.JJJL),
        company: Some(d.JJGS),
        establish_date: Some(d.ESTABDATE),
        mgmt_fee: Some(d.MGREXP),
        trust_fee: Some(d.TRUSTEXP),
        sales_fee: Some(d.SALESEXP),
        ..Default::default()
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_detail_response() {
        let body = r#"{
            "Datas": {
                "FCODE": "020988",
                "SHORTNAME": "南方恒生科技ETF",
                "FTYPE": "指数型",
                "RISKLEVEL": "4",
                "JJGS": "南方基金",
                "JJJL": "张其思",
                "ESTABDATE": "2024-05-21",
                "MGREXP": "0.15%",
                "TRUSTEXP": "0.05%",
                "SALESEXP": "0.00%"
            }
        }"#;
        let data = parse_detail_response(body).unwrap();
        assert_eq!(data.code, "020988");
        assert_eq!(data.manager.unwrap(), "张其思");
        assert_eq!(data.mgmt_fee.unwrap(), "0.15%");
    }
}
