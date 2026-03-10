use rusqlite::{Connection, Result};
use std::path::Path;
use rust_decimal::Decimal;
use rust_decimal::prelude::FromPrimitive;

pub fn init_db<P: AsRef<Path>>(path: P) -> Result<()> {
    let conn = Connection::open(path)?;
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
            current_fee_rate TEXT,
            last_sync_at DATETIME
        )",
        [],
    )?;

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

    // Create transaction table
    conn.execute(
        "CREATE TABLE IF NOT EXISTS transaction_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            wallet_id INTEGER NOT NULL,
            fund_code TEXT NOT NULL,
            type TEXT NOT NULL, -- 'buy' or 'sell'
            money TEXT NOT NULL,
            shares TEXT NOT NULL,
            nav TEXT NOT NULL,
            fee TEXT NOT NULL,
            date TEXT NOT NULL,
            FOREIGN KEY (wallet_id) REFERENCES wallet (id) ON DELETE CASCADE,
            FOREIGN KEY (fund_code) REFERENCES fund (code) ON DELETE CASCADE
        )",
        [],
    )?;

    // Create app_config table for active wallet
    conn.execute(
        "CREATE TABLE IF NOT EXISTS app_config (
            key TEXT PRIMARY KEY,
            value TEXT
        )",
        [],
    )?;

    Ok(())
}

pub fn add_wallet(conn: &Connection, name: &str) -> Result<()> {
    conn.execute(
        "INSERT INTO wallet (name) VALUES (?1)",
        [name],
    )?;
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

pub fn add_fund(conn: &Connection, code: &str, name: &str, fee_rate: &str) -> Result<()> {
    conn.execute(
        "INSERT OR REPLACE INTO fund (code, name, current_fee_rate) VALUES (?1, ?2, ?3)",
        [code, name, fee_rate],
    )?;
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

pub fn insert_nav_history_idempotent(conn: &Connection, code: &str, date: &str, nav: &str) -> Result<()> {
    conn.execute(
        "INSERT OR REPLACE INTO nav_history (fund_code, date, nav) VALUES (?1, ?2, ?3)",
        [code, date, nav],
    )?;
    Ok(())
}

pub struct Fund {
    pub code: String,
    pub name: String,
    pub current_fee_rate: Option<String>,
    pub last_sync_at: Option<String>,
}

pub fn get_fund_by_code_or_name(conn: &Connection, identifier: &str) -> Result<Option<Fund>> {
    let mut stmt = conn.prepare("SELECT code, name, current_fee_rate, last_sync_at FROM fund WHERE code = ?1 OR name = ?1")?;
    let mut rows = stmt.query([identifier])?;
    if let Some(row) = rows.next()? {
        Ok(Some(Fund {
            code: row.get(0)?,
            name: row.get(1)?,
            current_fee_rate: row.get(2)?,
            last_sync_at: row.get(3)?,
        }))
    } else {
        Ok(None)
    }
}

pub fn get_all_funds(conn: &Connection) -> Result<Vec<Fund>> {
    let mut stmt = conn.prepare("SELECT code, name, current_fee_rate, last_sync_at FROM fund")?;
    let rows = stmt.query_map([], |row| {
        Ok(Fund {
            code: row.get(0)?,
            name: row.get(1)?,
            current_fee_rate: row.get(2)?,
            last_sync_at: row.get(3)?,
        })
    })?;

    let mut funds = Vec::new();
    for row in rows {
        funds.push(row?);
    }
    Ok(funds)
}

pub fn get_latest_nav(conn: &Connection, code: &str) -> Result<Option<String>> {
    let mut stmt = conn.prepare("SELECT nav FROM nav_history WHERE fund_code = ?1 ORDER BY date DESC LIMIT 1")?;
    let mut rows = stmt.query([code])?;
    if let Some(row) = rows.next()? {
        Ok(Some(row.get(0)?))
    } else {
        Ok(None)
    }
}

pub fn add_transaction(
    conn: &Connection,
    wallet_id: i64,
    fund_code: &str,
    t_type: &str,
    money: &str,
    shares: &str,
    nav: &str,
    fee: &str,
    date: &str,
) -> Result<()> {
    conn.execute(
        "INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date) 
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
        rusqlite::params![wallet_id, fund_code, t_type, money, shares, nav, fee, date],
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
            SUM(CASE WHEN t.type = 'buy' THEN CAST(t.shares AS REAL) ELSE -CAST(t.shares AS REAL) END) as total_shares,
            SUM(CASE WHEN t.type = 'buy' THEN CAST(t.money AS REAL) ELSE -CAST(t.money AS REAL) END) as net_cost,
            (SELECT nav FROM nav_history WHERE fund_code = f.code ORDER BY date DESC LIMIT 1) as latest_nav
         FROM fund f
         JOIN transaction_log t ON f.code = t.fund_code
         WHERE t.wallet_id = ?1
         GROUP BY f.code"
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
            SUM(CASE WHEN type = 'buy' THEN CAST(shares AS REAL) ELSE -CAST(shares AS REAL) END)
         FROM transaction_log 
         WHERE wallet_id = ?1 AND fund_code = ?2"
    )?;
    let val: Option<f64> = stmt.query_row([wallet_id.to_string(), fund_code.to_string()], |row| row.get(0))?;
    let shares_f64 = val.unwrap_or(0.0);
    Ok(Decimal::from_f64(shares_f64).unwrap_or_default().round_dp(2))
}

pub fn delete_fund(conn: &Connection, code: &str) -> Result<()> {
    conn.execute("DELETE FROM fund WHERE code = ?1", [code])?;
    Ok(())
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
        let mut stmt = conn.prepare("SELECT name FROM sqlite_master WHERE type='table'").unwrap();
        let tables: Vec<String> = stmt.query_map([], |row| row.get(0)).unwrap().map(|r| r.unwrap()).collect();
        
        assert!(tables.contains(&"wallet".to_string()));
        assert!(tables.contains(&"fund".to_string()));
        assert!(tables.contains(&"nav_history".to_string()));
        assert!(tables.contains(&"transaction_log".to_string()));
        assert!(tables.contains(&"app_config".to_string()));
    }
}
