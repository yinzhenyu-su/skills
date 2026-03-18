use fund_manager::db;
use fund_manager::finance;
use rust_decimal_macros::dec;

#[test]
fn test_basic_transaction_flow() {
    let conn = db::setup_test_db().expect("Failed to setup memory db");
    
    // 1. Setup wallet and fund
    db::add_wallet(&conn, "Test").unwrap();
    db::add_fund(&conn, "000300", "沪深300", None, None, None, None, None, Some("0.0015"), None, None, None).unwrap();
    
    // 2. Buy
    let money = dec!(1000.00);
    let nav = dec!(1.00);
    let fee_rate = dec!(0.0015);
    let calculation = finance::calculate_purchase(money, nav, fee_rate);
    
    db::add_transaction(&conn, 1, "000300", "buy", "1000.00", Some(&calculation.shares.to_string()), Some(&nav.to_string()), &calculation.fee.to_string(), "2026-03-09", "settled").unwrap();
    
    // 3. Verify status (simplified)
    let wallets = db::get_all_wallets(&conn).unwrap();
    assert_eq!(wallets.len(), 1);
}
