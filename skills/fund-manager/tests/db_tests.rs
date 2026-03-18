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
