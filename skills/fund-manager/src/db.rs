use rusqlite::{Connection, Result};
use rust_decimal::Decimal;
use rust_decimal::prelude::FromPrimitive;
use std::path::Path;
use std::str::FromStr;

/// 打开数据库连接并启用外键约束
pub fn open_conn<P: AsRef<Path>>(path: P) -> Result<Connection> {
    let conn = Connection::open(path)?;
    conn.execute("PRAGMA foreign_keys = ON", [])?;
    Ok(conn)
}

pub fn init_db<P: AsRef<Path>>(path: P) -> Result<()> {
    let conn = Connection::open(path)?;
    setup_schema(&conn)
}

fn setup_schema(conn: &Connection) -> Result<()> {
    conn.execute("PRAGMA foreign_keys = ON", [])?;

    // Create wallet table
    conn.execute(
        "CREATE TABLE IF NOT EXISTS wallet (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )",
        [],
    )?;

    // Create fund table (Basic Info)
    conn.execute(
        "CREATE TABLE IF NOT EXISTS fund (
            code TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            fund_type TEXT,
            risk_level TEXT,
            manager TEXT,
            company TEXT,
            establish_date TEXT,
            management_fee TEXT,
            trust_fee TEXT,
            sales_fee TEXT,
            last_sync_at DATETIME
        )",
        [],
    )?;

    // Simple migration for existing columns
    let new_cols = [
        ("fund_type", "TEXT"),
        ("risk_level", "TEXT"),
        ("manager", "TEXT"),
        ("company", "TEXT"),
        ("establish_date", "TEXT"),
        ("management_fee", "TEXT"),
        ("trust_fee", "TEXT"),
        ("sales_fee", "TEXT"),
    ];
    for (name, col_type) in new_cols {
        let _ = conn.execute(
            &format!("ALTER TABLE fund ADD COLUMN {} {}", name, col_type),
            [],
        );
    }

    // Create nav_history table
    conn.execute(
        "CREATE TABLE IF NOT EXISTS nav_history (
            fund_code TEXT NOT NULL,
            date TEXT NOT NULL,
            nav TEXT NOT NULL,
            acc_nav TEXT,
            PRIMARY KEY (fund_code, date),
            FOREIGN KEY (fund_code) REFERENCES fund (code) ON DELETE CASCADE
        )",
        [],
    )?;

    // Create transaction table with status and nullable shares/nav
    conn.execute(
        "CREATE TABLE IF NOT EXISTS transaction_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            wallet_id INTEGER NOT NULL,
            fund_code TEXT NOT NULL,
            type TEXT NOT NULL, -- 'buy', 'sell', 'dividend', 'reinvest', 'import'
            money TEXT NOT NULL,
            shares TEXT,        -- Nullable for pending buys
            nav TEXT,           -- Nullable for pending buys
            fee TEXT NOT NULL,
            date TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'settled', -- 'pending' or 'settled'
            memo TEXT,
            source TEXT NOT NULL DEFAULT 'manual',
            FOREIGN KEY (wallet_id) REFERENCES wallet (id) ON DELETE CASCADE,
            FOREIGN KEY (fund_code) REFERENCES fund (code) ON DELETE CASCADE
        )",
        [],
    )?;

    // Migration for transaction_log: add status if not exists
    let has_status: bool = conn
        .prepare("PRAGMA table_info(transaction_log)")?
        .query_map([], |row| row.get::<_, String>(1))?
        .any(|name| name.unwrap_or_default() == "status");

    if !has_status {
        // Since we also want to make shares/nav nullable, and SQLite doesn't support
        // altering constraints easily, we recreate the table if status is missing.
        conn.execute(
            "ALTER TABLE transaction_log RENAME TO transaction_log_old",
            [],
        )?;
        conn.execute(
            "CREATE TABLE transaction_log (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                wallet_id INTEGER NOT NULL,
                fund_code TEXT NOT NULL,
                type TEXT NOT NULL,
                money TEXT NOT NULL,
                shares TEXT,
                nav TEXT,
                fee TEXT NOT NULL,
                date TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'settled',
                memo TEXT,
                source TEXT NOT NULL DEFAULT 'manual',
                FOREIGN KEY (wallet_id) REFERENCES wallet (id) ON DELETE CASCADE,
                FOREIGN KEY (fund_code) REFERENCES fund (code) ON DELETE CASCADE
            )",
            [],
        )?;
        conn.execute(
            "INSERT INTO transaction_log (id, wallet_id, fund_code, type, money, shares, nav, fee, date, status, source)
             SELECT id, wallet_id, fund_code, type, money, shares, nav, fee, date, 'settled', 'manual' FROM transaction_log_old",
            [],
        )?;
        conn.execute("DROP TABLE transaction_log_old", [])?;
    }

    // Migration for transaction_log: add memo and source if missing
    let table_info: Vec<String> = conn
        .prepare("PRAGMA table_info(transaction_log)")?
        .query_map([], |row| row.get::<_, String>(1))?
        .collect::<Result<_, _>>()?;

    if !table_info.iter().any(|name| name == "memo") {
        conn.execute("ALTER TABLE transaction_log ADD COLUMN memo TEXT", [])?;
    }
    if !table_info.iter().any(|name| name == "source") {
        conn.execute(
            "ALTER TABLE transaction_log ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'",
            [],
        )?;
    }

    // Create app_config table for active wallet
    conn.execute(
        "CREATE TABLE IF NOT EXISTS app_config (
            key TEXT PRIMARY KEY,
            value TEXT
        )",
        [],
    )?;

    // Create fund_analysis table
    conn.execute(
        "CREATE TABLE IF NOT EXISTS fund_analysis (
            fund_code TEXT PRIMARY KEY,
            snapshot_date TEXT,
            rating_3y INTEGER,
            rating_5y INTEGER,
            rank_pct_3y REAL,
            sharpe_3y REAL,
            calmar_3y REAL,
            max_drawdown_3y REAL,
            investor_gap_3y REAL,
            last_update DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (fund_code) REFERENCES fund (code) ON DELETE CASCADE
        )",
        [],
    )?;

    Ok(())
}

pub fn setup_test_db() -> Result<Connection> {
    let conn = Connection::open_in_memory()?;
    setup_schema(&conn)?;
    Ok(conn)
}

pub fn add_wallet(conn: &Connection, name: &str) -> Result<()> {
    conn.execute("INSERT INTO wallet (name) VALUES (?1)", [name])?;
    Ok(())
}

pub struct Wallet {
    pub id: i64,
    pub name: String,
    pub created_at: String,
}

pub fn get_all_wallets(conn: &Connection) -> Result<Vec<Wallet>> {
    let mut stmt = conn.prepare("SELECT id, name, created_at FROM wallet")?;
    let rows = stmt.query_map([], |row| {
        Ok(Wallet {
            id: row.get(0)?,
            name: row.get(1)?,
            created_at: row.get(2)?,
        })
    })?;

    let mut wallets = Vec::new();
    for row in rows {
        wallets.push(row?);
    }
    Ok(wallets)
}

pub fn add_fund(
    conn: &Connection,
    code: &str,
    name: &str,
    fund_type: Option<&str>,
    risk_level: Option<&str>,
    manager: Option<&str>,
    company: Option<&str>,
    establish_date: Option<&str>,
    mgmt_fee: Option<&str>,
    trust_fee: Option<&str>,
    sales_fee: Option<&str>,
    last_sync_at: Option<&str>,
) -> Result<()> {
    // We use a manual check and update instead of INSERT OR REPLACE
    // because REPLACE triggers ON DELETE CASCADE on nav_history.
    let mut stmt = conn.prepare("SELECT 1 FROM fund WHERE code = ?1")?;
    let exists = stmt.exists([code])?;

    if exists {
        conn.execute(
            "UPDATE fund SET 
                name = ?2, fund_type = ?3, risk_level = ?4, manager = ?5, 
                company = ?6, establish_date = ?7, management_fee = ?8, 
                trust_fee = ?9, sales_fee = ?10, last_sync_at = ?11
             WHERE code = ?1",
            rusqlite::params![
                code,
                name,
                fund_type,
                risk_level,
                manager,
                company,
                establish_date,
                mgmt_fee,
                trust_fee,
                sales_fee,
                last_sync_at
            ],
        )?;
    } else {
        conn.execute(
            "INSERT INTO fund (
                code, name, fund_type, risk_level, manager, company, establish_date, 
                management_fee, trust_fee, sales_fee, last_sync_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)",
            rusqlite::params![
                code,
                name,
                fund_type,
                risk_level,
                manager,
                company,
                establish_date,
                mgmt_fee,
                trust_fee,
                sales_fee,
                last_sync_at
            ],
        )?;
    }
    Ok(())
}

pub fn get_wallet_id_by_name(conn: &Connection, name: &str) -> Result<Option<i64>> {
    let mut stmt = conn.prepare("SELECT id FROM wallet WHERE name = ?1")?;
    let mut rows = stmt.query([name])?;
    if let Some(row) = rows.next()? {
        Ok(Some(row.get(0)?))
    } else {
        Ok(None)
    }
}

pub fn delete_wallet(conn: &Connection, id: i64) -> Result<()> {
    // 检查是否为当前活跃钱包，如果是则清理配置
    if let Some(active_id) = get_active_wallet_id(conn)? {
        if active_id == id {
            conn.execute("DELETE FROM app_config WHERE key = 'active_wallet_id'", [])?;
        }
    }

    // 删除钱包（依赖数据库级联删除 transaction_log）
    conn.execute("DELETE FROM wallet WHERE id = ?1", [id])?;
    Ok(())
}

pub fn rename_wallet(conn: &Connection, old_name: &str, new_name: &str) -> Result<()> {
    // 检查旧钱包是否存在
    let old_id = get_wallet_id_by_name(conn, old_name)?;
    if old_id.is_none() {
        return Err(rusqlite::Error::QueryReturnedNoRows);
    }

    // 检查新名称是否已存在
    if let Some(_) = get_wallet_id_by_name(conn, new_name)? {
        return Err(rusqlite::Error::QueryReturnedNoRows);
    }

    // 执行重命名
    conn.execute(
        "UPDATE wallet SET name = ?1 WHERE name = ?2",
        [new_name, old_name],
    )?;
    Ok(())
}

pub fn set_active_wallet(conn: &Connection, wallet_id: i64) -> Result<()> {
    conn.execute(
        "INSERT OR REPLACE INTO app_config (key, value) VALUES (?1, ?2)",
        ["active_wallet_id", &wallet_id.to_string()],
    )?;
    Ok(())
}

pub fn get_active_wallet_id(conn: &Connection) -> Result<Option<i64>> {
    let mut stmt = conn.prepare("SELECT value FROM app_config WHERE key = 'active_wallet_id'")?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        let val: String = row.get(0)?;
        Ok(val.parse().ok())
    } else {
        Ok(None)
    }
}

pub fn get_active_wallet(conn: &Connection) -> Result<Wallet> {
    let wallet_id =
        get_active_wallet_id(conn)?.ok_or_else(|| rusqlite::Error::QueryReturnedNoRows)?;

    let mut stmt = conn.prepare("SELECT id, name, created_at FROM wallet WHERE id = ?1")?;
    stmt.query_row([wallet_id], |row| {
        Ok(Wallet {
            id: row.get(0)?,
            name: row.get(1)?,
            created_at: row.get(2)?,
        })
    })
}

pub fn insert_nav_history_idempotent(
    conn: &Connection,
    code: &str,
    date: &str,
    nav: &str,
) -> Result<()> {
    conn.execute(
        "INSERT OR REPLACE INTO nav_history (fund_code, date, nav) VALUES (?1, ?2, ?3)",
        [code, date, nav],
    )?;
    Ok(())
}

#[derive(Clone, Debug)]
pub struct Fund {
    pub code: String,
    pub name: String,
    pub fund_type: Option<String>,
    pub risk_level: Option<String>,
    pub manager: Option<String>,
    pub company: Option<String>,
    pub establish_date: Option<String>,
    pub management_fee: Option<String>,
    pub trust_fee: Option<String>,
    pub sales_fee: Option<String>,
    pub last_sync_at: Option<String>,
}

#[derive(Clone, Debug)]
pub struct FundWithValuation {
    pub fund: Fund,
    pub latest_nav: Option<String>,
    pub latest_nav_date: Option<String>,
    pub total_shares: f64,
}

pub fn get_fund_by_code_or_name(conn: &Connection, identifier: &str) -> Result<Option<Fund>> {
    let mut stmt = conn.prepare(
        "SELECT 
        code, name, fund_type, risk_level, manager, company, establish_date, 
        management_fee, trust_fee, sales_fee, last_sync_at 
        FROM fund WHERE code = ?1 OR name = ?1",
    )?;
    let mut rows = stmt.query([identifier])?;
    if let Some(row) = rows.next()? {
        Ok(Some(Fund {
            code: row.get(0)?,
            name: row.get(1)?,
            fund_type: row.get(2)?,
            risk_level: row.get(3)?,
            manager: row.get(4)?,
            company: row.get(5)?,
            establish_date: row.get(6)?,
            management_fee: row.get(7)?,
            trust_fee: row.get(8)?,
            sales_fee: row.get(9)?,
            last_sync_at: row.get(10)?,
        }))
    } else {
        Ok(None)
    }
}

pub fn search_funds_locally(conn: &Connection, identifier: &str) -> Result<Vec<Fund>> {
    let mut stmt = conn.prepare(
        "SELECT 
        code, name, fund_type, risk_level, manager, company, establish_date, 
        management_fee, trust_fee, sales_fee, last_sync_at 
        FROM fund WHERE code LIKE ?1 OR name LIKE ?1",
    )?;
    let pattern = format!("%{}%", identifier);
    let rows = stmt.query_map([pattern], |row| {
        Ok(Fund {
            code: row.get(0)?,
            name: row.get(1)?,
            fund_type: row.get(2)?,
            risk_level: row.get(3)?,
            manager: row.get(4)?,
            company: row.get(5)?,
            establish_date: row.get(6)?,
            management_fee: row.get(7)?,
            trust_fee: row.get(8)?,
            sales_fee: row.get(9)?,
            last_sync_at: row.get(10)?,
        })
    })?;

    let mut results = Vec::new();
    for row in rows {
        results.push(row?);
    }
    Ok(results)
}

pub fn get_all_funds(conn: &Connection) -> Result<Vec<Fund>> {
    let mut stmt = conn.prepare(
        "SELECT 
        code, name, fund_type, risk_level, manager, company, establish_date, 
        management_fee, trust_fee, sales_fee, last_sync_at 
        FROM fund",
    )?;
    let rows = stmt.query_map([], |row| {
        Ok(Fund {
            code: row.get(0)?,
            name: row.get(1)?,
            fund_type: row.get(2)?,
            risk_level: row.get(3)?,
            manager: row.get(4)?,
            company: row.get(5)?,
            establish_date: row.get(6)?,
            management_fee: row.get(7)?,
            trust_fee: row.get(8)?,
            sales_fee: row.get(9)?,
            last_sync_at: row.get(10)?,
        })
    })?;

    let mut funds = Vec::new();
    for row in rows {
        funds.push(row?);
    }
    Ok(funds)
}

pub fn get_funds_with_valuations(
    conn: &Connection,
    wallet_id: Option<i64>,
) -> Result<Vec<FundWithValuation>> {
    let mut stmt = conn.prepare(
        "SELECT 
            f.code, f.name, f.fund_type, f.risk_level, f.manager, f.company, f.establish_date,
            f.management_fee, f.trust_fee, f.sales_fee, f.last_sync_at,
            (SELECT nav FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav,
            (SELECT date FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav_date,
            SUM(CASE 
                WHEN t.wallet_id = ?1 AND t.status = 'settled' 
                THEN (CASE WHEN t.type IN ('buy', 'import') THEN CAST(t.shares AS REAL) ELSE -CAST(t.shares AS REAL) END)
                ELSE 0 
            END) as total_shares
        FROM fund f
        LEFT JOIN transaction_log t ON f.code = t.fund_code
        GROUP BY f.code",
    )?;

    let rows = stmt.query_map([wallet_id], |row| {
        Ok(FundWithValuation {
            fund: Fund {
                code: row.get(0)?,
                name: row.get(1)?,
                fund_type: row.get(2)?,
                risk_level: row.get(3)?,
                manager: row.get(4)?,
                company: row.get(5)?,
                establish_date: row.get(6)?,
                management_fee: row.get(7)?,
                trust_fee: row.get(8)?,
                sales_fee: row.get(9)?,
                last_sync_at: row.get(10)?,
            },
            latest_nav: row.get(11)?,
            latest_nav_date: row.get(12)?,
            total_shares: row.get::<_, Option<f64>>(13)?.unwrap_or(0.0),
        })
    })?;

    let mut results = Vec::new();
    for row in rows {
        results.push(row?);
    }
    Ok(results)
}

pub fn get_all_fund_names_and_codes(conn: &Connection) -> Result<Vec<(String, String)>> {
    let mut stmt = conn.prepare("SELECT code, name FROM fund")?;
    let rows = stmt.query_map([], |row| Ok((row.get(0)?, row.get(1)?)))?;

    let mut results = Vec::new();
    for row in rows {
        results.push(row?);
    }
    Ok(results)
}

pub fn get_latest_nav_with_date(conn: &Connection, code: &str) -> Result<Option<(String, String)>> {
    let mut stmt = conn.prepare(
        "SELECT nav, date FROM nav_history WHERE fund_code = ?1 ORDER BY date DESC LIMIT 1",
    )?;
    let mut rows = stmt.query([code])?;
    if let Some(row) = rows.next()? {
        Ok(Some((row.get(0)?, row.get(1)?)))
    } else {
        Ok(None)
    }
}

pub fn get_latest_nav(conn: &Connection, code: &str) -> Result<Option<String>> {
    let mut stmt = conn
        .prepare("SELECT nav FROM nav_history WHERE fund_code = ?1 ORDER BY date DESC LIMIT 1")?;
    let mut rows = stmt.query([code])?;
    if let Some(row) = rows.next()? {
        Ok(Some(row.get(0)?))
    } else {
        Ok(None)
    }
}

pub fn get_nav_at_date(conn: &Connection, code: &str, date: &str) -> Result<Option<Decimal>> {
    let mut stmt =
        conn.prepare("SELECT nav FROM nav_history WHERE fund_code = ?1 AND date = ?2")?;
    let mut rows = stmt.query([code, date])?;
    if let Some(row) = rows.next()? {
        let nav_str: String = row.get(0)?;
        Ok(Decimal::from_str(&nav_str).ok())
    } else {
        Ok(None)
    }
}

/// Find the next available NAV starting from a given date, searching forward up to max_days.
/// Returns (actual_date, nav) if found, None if not found within the range.
pub fn find_next_available_nav(
    conn: &Connection,
    code: &str,
    start_date: &str,
    max_days: i64,
) -> Result<Option<(String, Decimal)>> {
    use chrono::NaiveDate;

    let start = NaiveDate::parse_from_str(start_date, "%Y-%m-%d")
        .map_err(|e| rusqlite::Error::InvalidParameterName(e.to_string()))?;

    for i in 0..=max_days {
        let current_date = start + chrono::Duration::days(i);
        let date_str = current_date.format("%Y-%m-%d").to_string();

        if let Some(nav) = get_nav_at_date(conn, code, &date_str)? {
            return Ok(Some((date_str, nav)));
        }
    }

    Ok(None)
}

/// Find the previous available NAV before a given date, searching backward up to max_days.
/// Returns (actual_date, nav) if found, None if not found within the range.
/// Used for sell transactions where we need the NAV from the day before.
pub fn find_prev_available_nav(
    conn: &Connection,
    code: &str,
    start_date: &str,
    max_days: i64,
) -> Result<Option<(String, Decimal)>> {
    use chrono::NaiveDate;

    let start = NaiveDate::parse_from_str(start_date, "%Y-%m-%d")
        .map_err(|e| rusqlite::Error::InvalidParameterName(e.to_string()))?;

    for i in 0..=max_days {
        let current_date = start - chrono::Duration::days(i);
        let date_str = current_date.format("%Y-%m-%d").to_string();

        if let Some(nav) = get_nav_at_date(conn, code, &date_str)? {
            return Ok(Some((date_str, nav)));
        }
    }

    Ok(None)
}

pub fn add_transaction(
    conn: &Connection,
    wallet_id: i64,
    fund_code: &str,
    t_type: &str,
    money: &str,
    shares: Option<&str>,
    nav: Option<&str>,
    fee: &str,
    date: &str,
    status: &str,
    memo: Option<&str>,
    source: &str,
) -> Result<()> {
    conn.execute(
        "INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date, status, memo, source) 
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)",
        rusqlite::params![
            wallet_id, fund_code, t_type, money, shares, nav, fee, date, status, memo, source
        ],
    )?;
    Ok(())
}

#[derive(Debug, Clone)]
pub struct Transaction {
    pub id: i64,
    pub wallet_name: String,
    pub fund_code: String,
    pub fund_name: String,
    pub t_type: String,
    pub money: String,
    pub shares: Option<String>,
    pub nav: Option<String>,
    pub fee: String,
    pub date: String,
    pub status: String,
    pub memo: Option<String>,
    pub source: String,
}

pub fn get_transaction_history(
    conn: &Connection,
    fund_code: Option<&str>,
    wallet_id: Option<i64>,
    t_type: Option<&str>,
    limit: i64,
) -> Result<Vec<Transaction>> {
    let mut query = "
        SELECT t.id, w.name as wallet_name, t.fund_code, f.name as fund_name, 
               t.type, t.money, t.shares, t.nav, t.fee, t.date, t.status, t.memo, t.source
        FROM transaction_log t
        JOIN wallet w ON t.wallet_id = w.id
        JOIN fund f ON t.fund_code = f.code
        WHERE 1=1
    "
    .to_string();

    let mut params: Vec<Box<dyn rusqlite::ToSql>> = Vec::new();

    if let Some(code) = fund_code {
        query.push_str(" AND t.fund_code = ? ");
        params.push(Box::new(code.to_string()));
    }
    if let Some(w_id) = wallet_id {
        query.push_str(" AND t.wallet_id = ? ");
        params.push(Box::new(w_id));
    }
    if let Some(tp) = t_type {
        query.push_str(" AND t.type = ? ");
        params.push(Box::new(tp.to_string()));
    }

    query.push_str(" ORDER BY t.date DESC, t.id DESC ");
    if limit > 0 {
        query.push_str(&format!(" LIMIT {} ", limit));
    }

    let mut stmt = conn.prepare(&query)?;
    let rows = stmt.query_map(rusqlite::params_from_iter(params), |row| {
        Ok(Transaction {
            id: row.get(0)?,
            wallet_name: row.get(1)?,
            fund_code: row.get(2)?,
            fund_name: row.get(3)?,
            t_type: row.get(4)?,
            money: row.get(5)?,
            shares: row.get(6)?,
            nav: row.get(7)?,
            fee: row.get(8)?,
            date: row.get(9)?,
            status: row.get(10)?,
            memo: row.get(11)?,
            source: row.get(12)?,
        })
    })?;

    let mut result = Vec::new();
    for r in rows {
        result.push(r?);
    }
    Ok(result)
}

pub fn get_pending_transactions(conn: &Connection) -> Result<Vec<Transaction>> {
    let mut stmt = conn.prepare(
        "
        SELECT t.id, w.name as wallet_name, t.fund_code, f.name as fund_name, 
               t.type, t.money, t.shares, t.nav, t.fee, t.date, t.status, t.memo, t.source
        FROM transaction_log t
        JOIN wallet w ON t.wallet_id = w.id
        JOIN fund f ON t.fund_code = f.code
        WHERE t.status = 'pending'
    ",
    )?;
    let rows = stmt.query_map([], |row| {
        Ok(Transaction {
            id: row.get(0)?,
            wallet_name: row.get(1)?,
            fund_code: row.get(2)?,
            fund_name: row.get(3)?,
            t_type: row.get(4)?,
            money: row.get(5)?,
            shares: row.get(6)?,
            nav: row.get(7)?,
            fee: row.get(8)?,
            date: row.get(9)?,
            status: row.get(10)?,
            memo: row.get(11)?,
            source: row.get(12)?,
        })
    })?;

    let mut result = Vec::new();
    for r in rows {
        result.push(r?);
    }
    Ok(result)
}

pub fn update_transaction_settlement(
    conn: &Connection,
    id: i64,
    shares: &str,
    nav: &str,
    fee: &str,
    money: &str,
    status: &str,
) -> Result<()> {
    conn.execute(
        "UPDATE transaction_log SET shares = ?1, nav = ?2, fee = ?3, money = ?4, status = ?5 WHERE id = ?6",
        rusqlite::params![shares, nav, fee, money, status, id],
    )?;
    Ok(())
}

pub struct Holding {
    pub fund_code: String,
    pub fund_name: String,
    pub total_shares: String,
    pub net_cost: String,
    pub latest_nav: Option<String>,
}

pub fn get_holdings(conn: &Connection, wallet_id: i64) -> Result<Vec<Holding>> {
    let mut stmt = conn.prepare(
        "SELECT
            f.code,
            f.name,
            SUM(CASE 
                WHEN t.type IN ('buy', 'import', 'reinvest') THEN CAST(IFNULL(t.shares, '0') AS REAL) 
                WHEN t.type = 'sell' THEN -CAST(IFNULL(t.shares, '0') AS REAL) 
                ELSE 0 
            END) as total_shares,
            SUM(CASE 
                WHEN t.type IN ('buy', 'import') THEN CAST(t.money AS REAL) 
                WHEN t.type IN ('sell', 'dividend') THEN -CAST(t.money AS REAL) 
                ELSE 0 
            END) as net_cost,
            (SELECT nav FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav
         FROM fund f
         JOIN transaction_log t ON f.code = t.fund_code
         WHERE t.wallet_id = ?1 AND t.status = 'settled'
         GROUP BY f.code
         HAVING total_shares > 0 OR net_cost != 0",
    )?;

    let rows = stmt.query_map([wallet_id], |row| {
        Ok(Holding {
            fund_code: row.get(0)?,
            fund_name: row.get(1)?,
            total_shares: row.get::<_, f64>(2)?.to_string(),
            net_cost: row.get::<_, f64>(3)?.to_string(),
            latest_nav: row.get(4)?,
        })
    })?;

    let mut holdings = Vec::new();
    for row in rows {
        holdings.push(row?);
    }
    Ok(holdings)
}

pub fn get_fund_shares(conn: &Connection, wallet_id: i64, fund_code: &str) -> Result<Decimal> {
    let mut stmt = conn.prepare(
        "SELECT
            SUM(CASE 
                WHEN type IN ('buy', 'import', 'reinvest') THEN CAST(IFNULL(shares, '0') AS REAL) 
                WHEN type = 'sell' THEN -CAST(IFNULL(shares, '0') AS REAL) 
                ELSE 0 
            END)
         FROM transaction_log
         WHERE wallet_id = ?1 AND fund_code = ?2 AND status = 'settled'",
    )?;
    let val: Option<f64> = stmt.query_row(
        rusqlite::params![wallet_id, fund_code],
        |row| row.get(0),
    )?;
    let shares_f64 = val.unwrap_or(0.0);
    Ok(Decimal::from_f64(shares_f64)
        .unwrap_or_default()
        .round_dp(2))
}
/// Get the earliest date of pending transactions.
/// If code is provided, only check that fund. Otherwise check all.
pub fn get_earliest_pending_date(conn: &Connection, code: Option<&str>) -> Result<Option<String>> {
    let sql = if let Some(c) = code {
        format!(
            "SELECT MIN(date) FROM transaction_log WHERE status = 'pending' AND fund_code = '{}'",
            c
        )
    } else {
        "SELECT MIN(date) FROM transaction_log WHERE status = 'pending'".to_string()
    };

    let mut stmt = conn.prepare(&sql)?;
    let mut rows = stmt.query([])?;
    if let Some(row) = rows.next()? {
        Ok(row.get(0)?)
    } else {
        Ok(None)
    }
}

pub fn delete_fund(conn: &Connection, code: &str) -> Result<()> {
    conn.execute("DELETE FROM fund WHERE code = ?1", [code])?;
    Ok(())
}

pub struct FundAnalysis {
    pub fund_code: String,
    pub snapshot_date: Option<String>,
    pub rating_3y: Option<i32>,
    pub rating_5y: Option<i32>,
    pub rank_pct_3y: Option<f64>,
    pub sharpe_3y: Option<f64>,
    pub calmar_3y: Option<f64>,
    pub max_drawdown_3y: Option<f64>,
    pub investor_gap_3y: Option<f64>,
    pub last_update: String,
}

pub fn add_fund_analysis(conn: &Connection, analysis: &FundAnalysis) -> Result<()> {
    conn.execute(
        "INSERT OR REPLACE INTO fund_analysis (
            fund_code, snapshot_date, rating_3y, rating_5y, rank_pct_3y, 
            sharpe_3y, calmar_3y, max_drawdown_3y, investor_gap_3y
        ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)",
        rusqlite::params![
            analysis.fund_code,
            analysis.snapshot_date,
            analysis.rating_3y,
            analysis.rating_5y,
            analysis.rank_pct_3y,
            analysis.sharpe_3y,
            analysis.calmar_3y,
            analysis.max_drawdown_3y,
            analysis.investor_gap_3y,
        ],
    )?;
    Ok(())
}

pub fn get_fund_analysis(conn: &Connection, code: &str) -> Result<Option<FundAnalysis>> {
    let mut stmt = conn.prepare(
        "SELECT 
            fund_code, snapshot_date, rating_3y, rating_5y, rank_pct_3y, 
            sharpe_3y, calmar_3y, max_drawdown_3y, investor_gap_3y, last_update 
        FROM fund_analysis WHERE fund_code = ?1",
    )?;
    let mut rows = stmt.query([code])?;
    if let Some(row) = rows.next()? {
        Ok(Some(FundAnalysis {
            fund_code: row.get(0)?,
            snapshot_date: row.get(1)?,
            rating_3y: row.get(2)?,
            rating_5y: row.get(3)?,
            rank_pct_3y: row.get(4)?,
            sharpe_3y: row.get(5)?,
            calmar_3y: row.get(6)?,
            max_drawdown_3y: row.get(7)?,
            investor_gap_3y: row.get(8)?,
            last_update: row.get(9)?,
        }))
    } else {
        Ok(None)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_init_db() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();

        init_db(path).expect("Failed to init DB");

        let conn = Connection::open(path).unwrap();
        let mut stmt = conn
            .prepare("SELECT name FROM sqlite_master WHERE type='table'")
            .unwrap();
        let tables: Vec<String> = stmt
            .query_map([], |row| row.get(0))
            .unwrap()
            .map(|r| r.unwrap())
            .collect();

        assert!(tables.contains(&"wallet".to_string()));
        assert!(tables.contains(&"fund".to_string()));
        assert!(tables.contains(&"nav_history".to_string()));
        assert!(tables.contains(&"transaction_log".to_string()));
        assert!(tables.contains(&"app_config".to_string()));
    }

    #[test]
    fn test_find_next_available_nav_same_day() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");

        let conn = Connection::open(path).unwrap();

        // Insert a fund
        add_fund(
            &conn,
            "000300",
            "沪深300",
            Some("股票型"),
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");

        // Insert NAV for the same day
        insert_nav_history_idempotent(&conn, "000300", "2026-03-16", "1.5000")
            .expect("Failed to insert nav");

        // Should find NAV on the same day
        let result =
            find_next_available_nav(&conn, "000300", "2026-03-16", 20).expect("Failed to query");
        assert!(result.is_some());
        let (date, _nav) = result.unwrap();
        assert_eq!(date, "2026-03-16");
    }

    #[test]
    fn test_find_next_available_nav_delayed() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");

        let conn = Connection::open(path).unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            Some("股票型"),
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");

        // Insert NAV only for 3 days later
        insert_nav_history_idempotent(&conn, "000300", "2026-03-19", "1.5200")
            .expect("Failed to insert nav");

        // Should find NAV after 3 days
        let result =
            find_next_available_nav(&conn, "000300", "2026-03-16", 20).expect("Failed to query");
        assert!(result.is_some());
        let (date, _nav) = result.unwrap();
        assert_eq!(date, "2026-03-19");
    }

    #[test]
    fn test_find_next_available_nav_not_found() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");

        let conn = Connection::open(path).unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            Some("股票型"),
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .expect("Failed to add fund");

        // No NAV inserted
        let result =
            find_next_available_nav(&conn, "000300", "2026-03-16", 20).expect("Failed to query");
        assert!(result.is_none());
    }

    #[test]
    fn test_get_earliest_pending_date() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        add_wallet(&conn, "test").unwrap();
        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        // No pending -> returns None
        assert_eq!(get_earliest_pending_date(&conn, None).unwrap(), None);

        // Add pending transactions
        add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1000",
            None,
            None,
            "1.5",
            "2024-03-01",
            "pending",
            None,
            "manual",
        )
        .unwrap();
        add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1000",
            None,
            None,
            "1.5",
            "2024-02-01",
            "pending",
            None,
            "manual",
        )
        .unwrap();

        // Global check
        assert_eq!(
            get_earliest_pending_date(&conn, None).unwrap(),
            Some("2024-02-01".to_string())
        );

        // Specific fund check
        assert_eq!(
            get_earliest_pending_date(&conn, Some("000300")).unwrap(),
            Some("2024-02-01".to_string())
        );
        assert_eq!(
            get_earliest_pending_date(&conn, Some("999999")).unwrap(),
            None
        );
    }

    #[test]
    fn test_delete_wallet() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        // 1. 添加钱包并设置为活跃
        add_wallet(&conn, "wallet1").unwrap();
        let id1 = get_wallet_id_by_name(&conn, "wallet1").unwrap().unwrap();
        set_active_wallet(&conn, id1).unwrap();
        assert_eq!(get_active_wallet_id(&conn).unwrap(), Some(id1));

        // 2. 为该钱包添加交易记录
        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        add_transaction(
            &conn,
            id1,
            "000300",
            "buy",
            "1000",
            Some("1000"),
            Some("1.0"),
            "1.5",
            "2024-03-01",
            "settled",
            None,
            "manual",
        )
        .unwrap();

        // 验证记录存在
        let mut stmt = conn
            .prepare("SELECT count(*) FROM transaction_log WHERE wallet_id = ?1")
            .unwrap();
        let count: i64 = stmt.query_row([id1], |row| row.get(0)).unwrap();
        assert_eq!(count, 1);

        // 3. 删除该钱包
        delete_wallet(&conn, id1).unwrap();

        // 验证钱包已删除
        assert!(get_wallet_id_by_name(&conn, "wallet1").unwrap().is_none());

        // 验证活跃钱包配置已清理
        assert_eq!(get_active_wallet_id(&conn).unwrap(), None);

        // 验证交易记录已被级联删除
        let mut stmt = conn
            .prepare("SELECT count(*) FROM transaction_log WHERE wallet_id = ?1")
            .unwrap();
        let count: i64 = stmt.query_row([id1], |row| row.get(0)).unwrap();
        assert_eq!(count, 0);
    }

    #[test]
    fn test_add_wallet() {
        let conn = setup_test_db().unwrap();

        add_wallet(&conn, "MyWallet").unwrap();

        let id = get_wallet_id_by_name(&conn, "MyWallet").unwrap();
        assert!(id.is_some());
        assert_eq!(id.unwrap(), 1);
    }

    #[test]
    fn test_add_duplicate_wallet() {
        let conn = setup_test_db().unwrap();

        add_wallet(&conn, "MyWallet").unwrap();
        let result = add_wallet(&conn, "MyWallet");

        assert!(result.is_err());
    }

    #[test]
    fn test_get_all_wallets() {
        let conn = setup_test_db().unwrap();

        add_wallet(&conn, "Wallet1").unwrap();
        add_wallet(&conn, "Wallet2").unwrap();

        let wallets = get_all_wallets(&conn).unwrap();
        assert_eq!(wallets.len(), 2);
        assert!(wallets.iter().any(|w| w.name == "Wallet1"));
        assert!(wallets.iter().any(|w| w.name == "Wallet2"));
    }

    #[test]
    fn test_get_funds_with_valuations() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        // Setup wallet and fund
        add_wallet(&conn, "TestWallet").unwrap();
        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.5000").unwrap();

        // Buy 1000 shares at 1.0
        add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1000",
            Some("1000"),
            Some("1.0"),
            "0",
            "2026-03-01",
            "settled",
            None,
            "manual",
        )
        .unwrap();

        let valuations = get_funds_with_valuations(&conn, Some(1)).unwrap();
        assert_eq!(valuations.len(), 1);
        assert_eq!(valuations[0].fund.code, "000300");
        assert_eq!(valuations[0].latest_nav, Some("1.5000".to_string()));
        assert!((valuations[0].total_shares - 1000.0).abs() < 0.01);
    }

    #[test]
    fn test_get_funds_with_valuations_no_wallet_shares() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        add_wallet(&conn, "TestWallet").unwrap();
        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.5000").unwrap();

        // No transactions - should still show fund with 0 shares
        let valuations = get_funds_with_valuations(&conn, Some(1)).unwrap();
        assert_eq!(valuations.len(), 1);
        assert_eq!(valuations[0].total_shares, 0.0);
    }

    #[test]
    fn test_settle_transaction() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        add_wallet(&conn, "TestWallet").unwrap();
        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        // Add pending transaction
        add_transaction(
            &conn,
            1,
            "000300",
            "buy",
            "1000",
            None,
            None,
            "1.5",
            "2026-03-01",
            "pending",
            None,
            "manual",
        )
        .unwrap();

        // Settle
        settle_transaction(&conn, 1, "666.67", "1.5000").unwrap();

        // Verify
        let mut stmt = conn
            .prepare("SELECT shares, nav, status FROM transaction_log WHERE id = 1")
            .unwrap();
        let (shares, nav, status): (String, String, String) = stmt
            .query_row([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))
            .unwrap();

        assert_eq!(shares, "666.67");
        assert_eq!(nav, "1.5000");
        assert_eq!(status, "settled");
    }

    #[test]
    fn test_delete_fund() {
        let tmp_file = NamedTempFile::new().unwrap();
        let path = tmp_file.path();
        init_db(path).expect("Failed to init DB");
        let conn = Connection::open(path).unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-09", "1.5000").unwrap();

        // Verify fund exists
        assert!(get_fund_by_code_or_name(&conn, "000300").unwrap().is_some());

        // Delete
        delete_fund(&conn, "000300").unwrap();

        // Verify fund deleted
        assert!(get_fund_by_code_or_name(&conn, "000300").unwrap().is_none());
        // Verify NAV also deleted (cascade)
        let mut stmt = conn
            .prepare("SELECT count(*) FROM nav_history WHERE fund_code = '000300'")
            .unwrap();
        let count: i64 = stmt.query_row([], |row| row.get(0)).unwrap();
        assert_eq!(count, 0);
    }

    #[test]
    fn test_get_fund_by_code_or_name_not_found() {
        let conn = setup_test_db().unwrap();

        let result = get_fund_by_code_or_name(&conn, "999999").unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn test_search_funds_locally() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        add_fund(
            &conn,
            "001512",
            "易方达创业板",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        let results = search_funds_locally(&conn, "300").unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].code, "000300");

        let results = search_funds_locally(&conn, "创业板").unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].code, "001512");

        let results = search_funds_locally(&conn, "易方达").unwrap();
        assert_eq!(results.len(), 1);

        let results = search_funds_locally(&conn, "不存在").unwrap();
        assert!(results.is_empty());
    }

    #[test]
    fn test_get_all_funds() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        add_fund(
            &conn,
            "001512",
            "易方达创业板",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        let funds = get_all_funds(&conn).unwrap();
        assert_eq!(funds.len(), 2);
    }

    #[test]
    fn test_get_latest_nav() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-01", "1.2000").unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-15", "1.5000").unwrap();

        let nav = get_latest_nav(&conn, "000300").unwrap();
        assert_eq!(nav, Some("1.5000".to_string()));
    }

    #[test]
    fn test_get_nav_at_date() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-01", "1.2000").unwrap();

        let nav = get_nav_at_date(&conn, "000300", "2026-03-01").unwrap();
        assert!(nav.is_some());
        assert_eq!(nav.unwrap(), Decimal::from_str("1.2000").unwrap());
    }

    #[test]
    fn test_add_fund_analysis() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        let analysis = FundAnalysis {
            fund_code: "000300".to_string(),
            snapshot_date: Some("2026-03-09".to_string()),
            rating_3y: Some(4),
            rating_5y: Some(5),
            rank_pct_3y: Some(23.5),
            sharpe_3y: Some(1.2),
            calmar_3y: Some(0.8),
            max_drawdown_3y: Some(-15.5),
            investor_gap_3y: Some(3.2),
            last_update: "2026-03-09 10:00:00".to_string(),
        };

        add_fund_analysis(&conn, &analysis).unwrap();

        let retrieved = get_fund_analysis(&conn, "000300").unwrap();
        assert!(retrieved.is_some());
        let retrieved = retrieved.unwrap();
        assert_eq!(retrieved.rating_3y, Some(4));
        assert_eq!(retrieved.sharpe_3y, Some(1.2));
    }

    #[test]
    fn test_insert_nav_history_idempotent() {
        let conn = setup_test_db().unwrap();

        add_fund(
            &conn,
            "000300",
            "沪深300",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        )
        .unwrap();

        // Insert same date twice
        insert_nav_history_idempotent(&conn, "000300", "2026-03-01", "1.2000").unwrap();
        insert_nav_history_idempotent(&conn, "000300", "2026-03-01", "1.3000").unwrap();

        // Should still have only 1 record with updated value
        let mut stmt = conn.prepare("SELECT count(*), nav FROM nav_history WHERE fund_code = '000300' AND date = '2026-03-01'").unwrap();
        let (count, nav): (i64, String) = stmt
            .query_row([], |row| Ok((row.get(0)?, row.get(1)?)))
            .unwrap();
        assert_eq!(count, 1);
        assert_eq!(nav, "1.3000");
    }
}
