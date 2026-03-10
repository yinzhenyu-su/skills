use rusqlite::Connection;
use crate::db;
use tempfile::NamedTempFile;

#[test]
fn test_nav_history_idempotency() {
    let tmp_file = NamedTempFile::new().unwrap();
    let path = tmp_file.path();
    db::init_db(path).unwrap();
    let conn = Connection::open(path).unwrap();

    conn.execute(
        "INSERT INTO fund (code, name) VALUES (?1, ?2)",
        ["000300", "沪深300"],
    ).unwrap();

    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.00").unwrap();
    db::insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.10").unwrap();
    
    let nav: String = conn.query_row(
        "SELECT nav FROM nav_history WHERE fund_code = '000300' AND date = '2026-03-09'",
        [],
        |row| row.get(0),
    ).unwrap();
    
    assert_eq!(nav, "1.10");

    let count: i64 = conn.query_row(
        "SELECT COUNT(*) FROM nav_history WHERE fund_code = '000300'",
        [],
        |row| row.get(0),
    ).unwrap();
    assert_eq!(count, 1);
}
