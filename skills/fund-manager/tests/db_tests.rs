use fund_manager::db;

#[test]
fn test_nav_history_idempotency() {
    let conn = db::setup_test_db().expect("Failed to setup test db");

    conn.execute(
        "INSERT INTO fund (code, name) VALUES (?1, ?2)",
        ["000300", "沪深300"],
    )
    .unwrap();

    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.00").unwrap();
    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.10").unwrap();

    let nav: String = conn
        .query_row(
            "SELECT nav FROM nav_history WHERE fund_code = '000300' AND date = '2026-03-09'",
            [],
            |row| row.get(0),
        )
        .unwrap();

    assert_eq!(nav, "1.10");

    let count: i64 = conn
        .query_row(
            "SELECT COUNT(*) FROM nav_history WHERE fund_code = '000300'",
            [],
            |row| row.get(0),
        )
        .unwrap();
    assert_eq!(count, 1);
}

#[test]
fn test_search_funds_locally() {
    let conn = db::setup_test_db().expect("Failed to setup test db");

    conn.execute(
        "INSERT INTO fund (code, name) VALUES (?1, ?2)",
        ["000513", "富国高端制造行业股票A"],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO fund (code, name) VALUES (?1, ?2)",
        ["014930", "富国高端制造行业股票C"],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO fund (code, name) VALUES (?1, ?2)",
        ["160119", "南方中证500ETF联接(LOF)A"],
    )
    .unwrap();

    // 1. Precise match (code)
    let res = db::search_funds_locally(&conn, "000513").unwrap();
    assert_eq!(res.len(), 1);
    assert_eq!(res[0].name, "富国高端制造行业股票A");

    // 2. Fuzzy match (name part)
    let res = db::search_funds_locally(&conn, "富国高端").unwrap();
    assert_eq!(res.len(), 2);

    // 3. Precise match (name)
    let res = db::search_funds_locally(&conn, "南方中证500ETF联接(LOF)A").unwrap();
    assert_eq!(res.len(), 1);
    assert_eq!(res[0].code, "160119");

    // 4. No match
    let res = db::search_funds_locally(&conn, "不存在").unwrap();
    assert!(res.is_empty());
}

#[test]
fn test_get_funds_with_valuations() {
    let conn = db::setup_test_db().expect("Failed to setup test db");

    // 1. Setup Data
    conn.execute(
        "INSERT INTO fund (code, name) VALUES ('000300', '沪深300')",
        [],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO fund (code, name) VALUES ('000513', '富国高端')",
        [],
    )
    .unwrap();

    // Add Nav
    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-01", "1.00").unwrap();
    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-02", "1.10").unwrap();

    // Add Wallet & Transactions
    conn.execute(
        "INSERT INTO wallet (id, name) VALUES (1, 'Test Wallet')",
        [],
    )
    .unwrap();
    db::add_transaction(
        &conn,
        1,
        "000300",
        "buy",
        "1000",
        Some("1000"),
        Some("1.00"),
        "0",
        "2026-03-01",
        "settled",
    )
    .unwrap();
    db::add_transaction(
        &conn,
        1,
        "000300",
        "sell",
        "550",
        Some("500"),
        Some("1.10"),
        "0",
        "2026-03-02",
        "settled",
    )
    .unwrap();

    // 2. Test with active wallet
    let results = db::get_funds_with_valuations(&conn, Some(1)).unwrap();
    assert_eq!(results.len(), 2);

    let hs300 = results.iter().find(|r| r.fund.code == "000300").unwrap();
    assert_eq!(hs300.latest_nav, Some("1.10".to_string()));
    assert_eq!(hs300.latest_nav_date, Some("2026-03-02".to_string()));
    assert_eq!(hs300.total_shares, 500.0);

    let fg = results.iter().find(|r| r.fund.code == "000513").unwrap();
    assert_eq!(fg.latest_nav, None);
    assert_eq!(fg.total_shares, 0.0);

    // 3. Test without active wallet (None)
    let results_no_wallet = db::get_funds_with_valuations(&conn, None).unwrap();
    let hs300_no_wallet = results_no_wallet
        .iter()
        .find(|r| r.fund.code == "000300")
        .unwrap();
    assert_eq!(hs300_no_wallet.total_shares, 0.0);
}
