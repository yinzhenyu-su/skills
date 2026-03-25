use fund_manager::db;
use fund_manager::finance;
use rust_decimal_macros::dec;

#[test]
fn test_basic_transaction_flow() {
    let conn = db::setup_test_db().expect("Failed to setup memory db");

    // 1. Setup wallet and fund
    db::add_wallet(&conn, "Test").unwrap();
    db::add_fund(
        &conn,
        "000300",
        "沪深300",
        None,
        None,
        None,
        None,
        None,
        Some("0.0015"),
        None,
        None,
        None,
    )
    .unwrap();

    // 2. Buy
    let money = dec!(1000.00);
    let nav = dec!(1.00);
    let fee_rate = dec!(0.0015);
    let calculation = finance::calculate_purchase(money, nav, fee_rate);

    db::add_transaction(
        &conn,
        1,
        "000300",
        "buy",
        "1000.00",
        Some(&calculation.shares.to_string()),
        Some(&nav.to_string()),
        &calculation.fee.to_string(),
        "2026-03-09",
        "settled",
        None,
        "manual",
    )
    .unwrap();

    // 3. Verify status (simplified)
    let wallets = db::get_all_wallets(&conn).unwrap();
    assert_eq!(wallets.len(), 1);
}

#[tokio::test]
async fn test_transaction_lifecycle_with_auto_settlement() {
    let conn = db::setup_test_db().expect("Failed to setup memory db");

    // 1. Setup
    db::add_wallet(&conn, "Main").unwrap();
    db::add_fund(
        &conn,
        "000300",
        "HS300",
        None,
        None,
        None,
        None,
        None,
        Some("0.15%"),
        None,
        None,
        None,
    )
    .unwrap();

    // 2. Add PENDING Buy (Money = 1000)
    db::add_transaction(
        &conn,
        1,
        "000300",
        "buy",
        "1000.00",
        None,
        None,
        "1.50",
        "2026-03-10",
        "pending",
        None,
        "manual",
    )
    .unwrap();

    // 3. Verify it's pending
    let pending = db::get_pending_transactions(&conn).unwrap();
    assert_eq!(pending.len(), 1);
    assert_eq!(pending[0].status, "pending");

    // 4. Add NAV for that date
    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-10", "2.00", None).unwrap();

    // 5. Run Settlement
    let settled = fund_manager::sync::settle_pending_transactions(&conn)
        .await
        .unwrap();
    assert_eq!(settled, 1);

    // 6. Verify it's settled and has shares
    let pending_after = db::get_pending_transactions(&conn).unwrap();
    assert_eq!(pending_after.len(), 0);

    let history = db::get_transaction_history(&conn, Some("000300"), None, None, 0).unwrap();
    assert_eq!(history.len(), 1);
    assert_eq!(history[0].status, "settled");
    // (1000 / 1.0015) = 998.502... Round Fee to 1.50. Net = 998.50. Shares = 998.50 / 2.0 = 499.25
    assert_eq!(history[0].shares, Some("499.25".to_string()));
    assert_eq!(history[0].nav, Some("2.00".to_string()));
}

#[test]
fn test_dividend_impact_on_net_cost() {
    let conn = db::setup_test_db().expect("Failed to setup memory db");
    db::add_wallet(&conn, "Main").unwrap();
    db::add_fund(
        &conn, "000300", "HS300", None, None, None, None, None, None, None, None, None,
    )
    .unwrap();

    // Buy 1000
    db::add_transaction(
        &conn,
        1,
        "000300",
        "buy",
        "1000.00",
        Some("500.00"),
        Some("2.00"),
        "0",
        "2026-03-10",
        "settled",
        None,
        "manual",
    )
    .unwrap();

    // Dividend 100
    db::add_transaction(
        &conn,
        1,
        "000300",
        "dividend",
        "100.00",
        None,
        None,
        "0",
        "2026-03-15",
        "settled",
        None,
        "manual",
    )
    .unwrap();

    let holdings = db::get_holdings(&conn, Some(1), None).unwrap();
    assert_eq!(holdings.len(), 1);
    // net_cost = 1000 - 100 = 900
    assert_eq!(holdings[0].net_cost, dec!(900.00));
    assert_eq!(holdings[0].shares, dec!(500.00));
}
