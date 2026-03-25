use rust_decimal::Decimal;
use rust_decimal_macros::dec;
use std::str::FromStr;

pub fn days_since(date_str: &str) -> i64 {
    let target = match chrono::NaiveDate::parse_from_str(date_str, "%Y-%m-%d") {
        Ok(d) => d,
        Err(_) => return 0,
    };
    let today = chrono::Local::now().date_naive();
    (today - target).num_days()
}

pub struct PurchaseResult {
    pub shares: Decimal,
    pub fee: Decimal,
}

pub struct SellResult {
    pub money: Decimal,
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

pub fn calculate_sell(shares: Decimal, nav: Decimal, fee_rate: Decimal) -> SellResult {
    let total_money = shares * nav;
    let fee = (total_money * fee_rate).round_dp(2);
    let money = (total_money - fee).round_dp(2);

    SellResult { money, fee }
}

/// Parses a percentage rate string (e.g. "0.15%") into a Decimal (e.g. 0.0015).
pub fn parse_percentage_rate(input: &str) -> Decimal {
    let input = input.trim();
    if input.is_empty() {
        return Decimal::ZERO;
    }

    if input.ends_with('%') {
        let val_str = &input[..input.len() - 1];
        let val = Decimal::from_str(val_str.trim()).unwrap_or(Decimal::ZERO);
        (val / dec!(100)).round_dp(6)
    } else {
        Decimal::from_str(input).unwrap_or(Decimal::ZERO)
    }
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

/// Calculate holding info from holding amount, profit, and NAV.
/// Returns (shares, cost_basis, cost_per_share)
pub fn calculate_holding_from_profit(
    holding_amount: Decimal,
    holding_profit: Decimal,
    nav: Decimal,
) -> (Decimal, Decimal, Decimal) {
    // shares = holding_amount / nav
    let shares = (holding_amount / nav).round_dp(2);
    // cost_basis = holding_amount - holding_profit
    let cost_basis = (holding_amount - holding_profit).round_dp(2);
    // cost_per_share = cost_basis / shares
    let cost_per_share = if shares.is_zero() {
        Decimal::ZERO
    } else {
        (cost_basis / shares).round_dp(4)
    };
    (shares, cost_basis, cost_per_share)
}

/// Detects dividend per share by comparing changes in NAV and AccNAV.
/// D = (AccNAV_t - AccNAV_{t-1}) - (NAV_t - NAV_{t-1})
pub fn detect_dividend(
    nav_t: Decimal,
    acc_nav_t: Decimal,
    nav_prev: Decimal,
    acc_nav_prev: Decimal,
) -> Decimal {
    let d_acc = acc_nav_t - acc_nav_prev;
    let d_nav = nav_t - nav_prev;
    let dividend = d_acc - d_nav;
    if dividend > dec!(0.0001) {
        dividend.round_dp(4)
    } else {
        Decimal::ZERO
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_dividend() {
        // No dividend: AccNAV and NAV move together
        // Day 1: NAV=1.0, AccNAV=1.0
        // Day 2: NAV=1.1, AccNAV=1.1
        assert_eq!(
            detect_dividend(dec!(1.1), dec!(1.1), dec!(1.0), dec!(1.0)),
            dec!(0)
        );

        // Dividend of 0.1: AccNAV stays at 1.1, but NAV drops to 1.0
        // Day 1: NAV=1.1, AccNAV=1.1
        // Day 2: NAV=1.0, AccNAV=1.1
        // (1.1 - 1.1) - (1.0 - 1.1) = 0 - (-0.1) = 0.1
        assert_eq!(
            detect_dividend(dec!(1.0), dec!(1.1), dec!(1.1), dec!(1.1)),
            dec!(0.1)
        );

        // Dividend of 0.05: AccNAV increases more than NAV
        // Day 1: NAV=1.0, AccNAV=1.0
        // Day 2: NAV=1.05, AccNAV=1.10
        // (1.10 - 1.0) - (1.05 - 1.0) = 0.1 - 0.05 = 0.05
        assert_eq!(
            detect_dividend(dec!(1.05), dec!(1.10), dec!(1.0), dec!(1.0)),
            dec!(0.05)
        );

        // Minor precision difference should be ignored
        assert_eq!(
            detect_dividend(dec!(1.00001), dec!(1.00002), dec!(1.0), dec!(1.0)),
            dec!(0)
        );
    }

    #[test]
    fn test_parse_percentage_rate() {
        assert_eq!(parse_percentage_rate("0.15%"), dec!(0.0015));
        assert_eq!(parse_percentage_rate("1.5%"), dec!(0.015));
        assert_eq!(parse_percentage_rate("0.0015"), dec!(0.0015));
        assert_eq!(parse_percentage_rate("  0.12%  "), dec!(0.0012));
        assert_eq!(parse_percentage_rate(""), dec!(0));
    }

    #[test]
    fn test_calculate_purchase_with_low_fee() {
        // 100 money, 0.15% fee, 2.3026 NAV
        let money = dec!(100);
        let nav = dec!(2.3026);
        let fee_rate = parse_percentage_rate("0.15%");
        let result = calculate_purchase(money, nav, fee_rate);

        // 100 - 100/1.0015 = 0.1497... -> 0.15
        assert_eq!(result.fee, dec!(0.15));
        // (100 - 0.15) / 2.3026 = 43.3640... -> 43.36
        assert_eq!(result.shares, dec!(43.36));
    }

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

    #[test]
    fn test_calculate_holding_from_profit_normal() {
        // holding_amount = 11000, holding_profit = 1000, nav = 1.1
        // shares = 11000 / 1.1 = 10000
        // cost_basis = 11000 - 1000 = 10000
        // cost_per_share = 10000 / 10000 = 1.0
        let (shares, cost_basis, cost_per_share) =
            calculate_holding_from_profit(dec!(11000), dec!(1000), dec!(1.1));
        assert_eq!(shares, dec!(10000.00));
        assert_eq!(cost_basis, dec!(10000.00));
        assert_eq!(cost_per_share, dec!(1.0000));
    }

    #[test]
    fn test_calculate_holding_from_profit_loss() {
        // holding_amount = 9000, holding_profit = -1000, nav = 0.9
        // shares = 9000 / 0.9 = 10000
        // cost_basis = 9000 - (-1000) = 10000
        // cost_per_share = 10000 / 10000 = 1.0
        let (shares, cost_basis, cost_per_share) =
            calculate_holding_from_profit(dec!(9000), dec!(-1000), dec!(0.9));
        assert_eq!(shares, dec!(10000.00));
        assert_eq!(cost_basis, dec!(10000.00));
        assert_eq!(cost_per_share, dec!(1.0000));
    }

    #[test]
    fn test_calculate_holding_from_profit_zero_profit() {
        // holding_amount = 10000, holding_profit = 0, nav = 1.0
        // shares = 10000 / 1.0 = 10000
        // cost_basis = 10000 - 0 = 10000
        // cost_per_share = 10000 / 10000 = 1.0
        let (shares, cost_basis, cost_per_share) =
            calculate_holding_from_profit(dec!(10000), dec!(0), dec!(1.0));
        assert_eq!(shares, dec!(10000.00));
        assert_eq!(cost_basis, dec!(10000.00));
        assert_eq!(cost_per_share, dec!(1.0000));
    }

    #[test]
    fn test_days_since_today() {
        let today = chrono::Local::now().format("%Y-%m-%d").to_string();
        assert_eq!(days_since(&today), 0);
    }

    #[test]
    fn test_days_since_past_date() {
        let today = chrono::Local::now().date_naive();
        let past = (today - chrono::Duration::days(10))
            .format("%Y-%m-%d")
            .to_string();
        assert_eq!(days_since(&past), 10);
    }

    #[test]
    fn test_days_since_future_date() {
        let today = chrono::Local::now().date_naive();
        let future = (today + chrono::Duration::days(3))
            .format("%Y-%m-%d")
            .to_string();
        assert_eq!(days_since(&future), -3);
    }
}
