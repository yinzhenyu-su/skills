use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::str::FromStr;

pub struct PurchaseResult {
    pub shares: Decimal,
    pub fee: Decimal,
}

pub fn calculate_purchase(money: Decimal, nav: Decimal, fee_rate: Decimal) -> PurchaseResult {
    // Net Amount = Money / (1 + Fee Rate)
    let one = dec!(1);
    let net_amount = money / (one + fee_rate);
    let fee = money - net_amount;

    // Round fee to 2 decimal places
    let fee = fee.round_dp(2);
    let net_amount = money - fee;

    let shares = net_amount / nav;
    // Round shares to 2 decimal places
    let shares = shares.round_dp(2);

    PurchaseResult { shares, fee }
}

pub fn resolve_shares(input: &str, total: Decimal) -> Result<Decimal, String> {
    if input.to_lowercase() == "all" {
        return Ok(total);
    }

    if input.contains('/') {
        let parts: Vec<&str> = input.split('/').collect();
        if parts.len() == 2 {
            let num = Decimal::from_str(parts[0].trim()).map_err(|e| e.to_string())?;
            let den = Decimal::from_str(parts[1].trim()).map_err(|e| e.to_string())?;
            if den.is_zero() {
                return Err("Denominator cannot be zero".to_string());
            }
            return Ok((total * (num / den)).round_dp(2));
        }
    }

    Decimal::from_str(input)
        .map(|d| d.round_dp(2))
        .map_err(|e| e.to_string())
}

pub fn resolve_fee(input: &str, total_money: Decimal) -> Result<Decimal, String> {
    let input = input.trim();
    if input.ends_with('%') {
        let rate_str = &input[..input.len() - 1];
        let rate = Decimal::from_str(rate_str.trim()).map_err(|e| e.to_string())?;
        Ok((total_money * (rate / dec!(100))).round_dp(2))
    } else {
        Decimal::from_str(input)
            .map(|d| d.round_dp(2))
            .map_err(|e| e.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_calculate_purchase() {
        let money = dec!(1001.50);
        let nav = dec!(1.25);
        let fee_rate = dec!(0.0015); // 0.15%
        let result = calculate_purchase(money, nav, fee_rate);

        assert_eq!(result.fee, dec!(1.50));
        assert_eq!(result.shares, dec!(800.00)); // 1000 / 1.25 = 800
    }

    #[test]
    fn test_resolve_shares_numeric() {
        let total = dec!(1000.00);
        assert_eq!(resolve_shares("500", total).unwrap(), dec!(500.00));
        assert_eq!(resolve_shares("123.45", total).unwrap(), dec!(123.45));
    }

    #[test]
    fn test_resolve_shares_fractions() {
        let total = dec!(1200.00);
        assert_eq!(resolve_shares("1/2", total).unwrap(), dec!(600.00));
        assert_eq!(resolve_shares("1/3", total).unwrap(), dec!(400.00));
        assert_eq!(resolve_shares("1/4", total).unwrap(), dec!(300.00));
        assert_eq!(resolve_shares("1/5", total).unwrap(), dec!(240.00));
    }

    #[test]
    fn test_resolve_shares_all() {
        let total = dec!(1234.56);
        assert_eq!(resolve_shares("all", total).unwrap(), total);
    }

    #[test]
    fn test_resolve_shares_invalid() {
        let total = dec!(1000.00);
        assert!(resolve_shares("invalid", total).is_err());
        assert!(resolve_shares("1/0", total).is_err());
    }

    #[test]
    fn test_resolve_fee_numeric() {
        let total_money = dec!(1000.00);
        assert_eq!(resolve_fee("5.0", total_money).unwrap(), dec!(5.00));
        assert_eq!(resolve_fee("12.34", total_money).unwrap(), dec!(12.34));
    }

    #[test]
    fn test_resolve_fee_percentage() {
        let total_money = dec!(1000.00);
        assert_eq!(resolve_fee("0.5%", total_money).unwrap(), dec!(5.00));
        assert_eq!(resolve_fee("1.5%", total_money).unwrap(), dec!(15.00));
        assert_eq!(resolve_fee("0%", total_money).unwrap(), dec!(0.00));
    }

    #[test]
    fn test_resolve_fee_invalid() {
        let total_money = dec!(1000.00);
        assert!(resolve_fee("invalid", total_money).is_err());
        assert!(resolve_fee("%", total_money).is_err());
    }
}
