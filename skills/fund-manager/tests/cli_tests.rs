use assert_cmd::Command;
use predicates::prelude::*;
use std::fs;
use std::env;

#[test]
fn test_wallet_add() {
    // Setup a temporary directory for the app
    let temp_app_dir = env::temp_dir().join("fund-manager-test-wallet-add");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    
    // fund wallet add "Test Wallet"
    cmd.arg("wallet")
       .arg("add")
       .arg("Test Wallet")
       .assert()
       .success()
       .stdout(predicate::str::contains("Successfully added wallet: Test Wallet"));

    // Verify DB entry
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet")
       .arg("add")
       .arg("Test Wallet")
       .assert()
       .failure()
       .stderr(predicate::str::contains("already exists"));
}

#[test]
fn test_wallet_use() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-wallet-use");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // 1. Add two wallets
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Wallet1").assert().success();

    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Wallet2").assert().success();

    // 2. Switch to Wallet2
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet")
       .arg("use")
       .arg("Wallet2")
       .assert()
       .success()
       .stdout(predicate::str::contains("Now using wallet: Wallet2"));

    // 3. Switch to non-existent wallet
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet")
       .arg("use")
       .arg("NoSuchWallet")
       .assert()
       .failure()
       .stderr(predicate::str::contains("does not exist"));
}

#[test]
fn test_status_sync_failure_warning() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sync-failure");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // 1. Setup: Add a wallet and switch to it
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Main").assert().success();
    
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("use").arg("Main").assert().success();

    // 2. Run fund status with forced sync failure
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.env("FORCE_SYNC_FAILURE", "1");
    cmd.arg("status")
       .assert()
       .success() // App should not crash
       .stderr(predicate::str::contains("Warning: Could not fetch latest data"));
}

#[test]
fn test_buy_auto_calculation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-buy-auto");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // 1. Setup: Add wallet, switch to it
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Invest").assert().success();
    
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("use").arg("Invest").assert().success();

    // Manually insert fund and NAV for testing
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let now = chrono::Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();
        conn.execute("INSERT INTO fund (code, name, management_fee, last_sync_at) VALUES ('000300', '沪深300', '0.0015', ?1)", [&now]).unwrap();
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.25')", []).unwrap();
    }
    
    // 2. Run fund buy
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("buy")
       .arg("000300")
       .arg("--money")
       .arg("1001.50")
       .arg("--auto")
       .assert()
       .success()
       .stdout(predicate::str::contains("Bought 000300"));
}

#[test]
fn test_status_valuation_and_pl() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-status-pl");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // Better setup: use the tool to init
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Main").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("use").arg("Main").assert().success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let now = chrono::Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();
        conn.execute("INSERT INTO fund (code, name, management_fee, last_sync_at) VALUES ('000300', '沪深300', '0.00', ?1)", [&now]).unwrap();
        // Day 1: Buy 1000 shares at 1.00 (Cost = 1000)
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-08', '1.00')", []).unwrap();
        conn.execute("INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date) 
                      VALUES (1, '000300', 'buy', '1000', '1000', '1.00', '0', '2026-03-08')", []).unwrap();
        // Day 2: NAV goes to 1.20
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.20')", []).unwrap();
    }

    // 2. Run fund status
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("status")
       .assert()
       .success()
       .stdout(predicate::str::contains("1200.00"))
       .stdout(predicate::str::contains("200.00"))
       .stdout(predicate::str::contains("20.00%"));
}

#[test]
fn test_cold_start_flow() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-cold-start");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Add wallet
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("add").arg("MyWallet")
        .assert().success();

    // 2. Use wallet
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("use").arg("MyWallet")
        .assert().success();

    // 3. Add fund
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund").arg("add").arg("000001").arg("华夏成长")
        .assert().success();

    // 4. Trigger sync (via status)
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status")
        .assert().success();

    // 5. Buy (this will trigger auto-sync)
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy").arg("000001").arg("--money").arg("1500").arg("--auto")
        .assert().success()
        .stdout(predicate::str::contains("Bought 000001"));

    // 6. Check status
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status")
        .assert().success()
        .stdout(predicate::str::contains("000001"))
        .stdout(predicate::str::contains("1500"));
}

#[test]
fn test_e2e_full_lifecycle() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-e2e-lifecycle");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Init & Wallet
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("E2E").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("E2E").assert().success();

    // 2. Add Fund
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000001").arg("Fund1").assert().success();

    // 2.5 Trigger sync
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status").assert().success();

    // 3. Buy 1000
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("buy").arg("000001").arg("--money").arg("1500").arg("--auto")
       .assert().success();

    // 4. Sell 400
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("sell").arg("000001").arg("--shares").arg("400").arg("--nav").arg("1.50")
       .write_stdin("y\n")
       .assert().success();

    // 5. Check status
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("status")
       .assert().success()
       .stdout(predicate::str::contains("000001"));

    // 6. Delete fund
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("fund").arg("delete").arg("000001")
       .write_stdin("y\n")
       .assert().success();

    // 7. Verify empty status
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("status")
       .assert().success()
       .stdout(predicate::str::contains("Fund1").not());
}

#[test]
fn test_wallet_override_and_auto_discovery() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-discovery");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Two wallets, but use Wallet A
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("add").arg("WalletA").assert().success();
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("add").arg("WalletB").assert().success();
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("use").arg("WalletA").assert().success();

    // Mock db to bypass Nginx forbidden on API during tests
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let now = chrono::Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();
        conn.execute("INSERT INTO fund (code, name, management_fee, last_sync_at) VALUES ('160119', '南方500', '0.00', ?1)", [&now]).unwrap();
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('160119', '2026-03-09', '1.00')", []).unwrap();
    }

    // 2. Buy fund 160119 for WalletB
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy").arg("160119").arg("--money").arg("1000").arg("--auto").arg("--wallet").arg("WalletB")
        .assert().success()
        .stdout(predicate::str::contains("Bought 160119"));

    // 3. Verify WalletA is empty
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status") // Uses WalletA by default
        .assert().success()
        .stdout(predicate::str::contains("160119").not());

    // 4. Verify WalletB has data in wallet list
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("list")
        .assert().success()
        .stdout(predicate::str::contains("WalletB"))
        .stdout(predicate::str::contains("1000"));
}

#[test]
fn test_fund_delete_cascade() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-delete-cascade");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet, Fund, Buy, NAV
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();

    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy").arg("000300").arg("--money").arg("1000").arg("--shares").arg("1000").arg("--nav").arg("1.0").assert().success();

    // 2. Delete Fund
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("fund")
       .arg("delete")
       .arg("000300")
       .write_stdin("y\n") // Confirm deletion
       .assert()
       .success();

    // 3. Verify DB is clean
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let fund_count: i64 = conn.query_row("SELECT COUNT(*) FROM fund", [], |row| row.get(0)).unwrap();
        let trans_count: i64 = conn.query_row("SELECT COUNT(*) FROM transaction_log", [], |row| row.get(0)).unwrap();
        let nav_count: i64 = conn.query_row("SELECT COUNT(*) FROM nav_history", [], |row| row.get(0)).unwrap();

        assert_eq!(fund_count, 0);
        assert_eq!(trans_count, 0);
        assert_eq!(nav_count, 0);
    }
}

#[test]
fn test_fund_delete_with_yes_flag() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-delete-yes");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Invest").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Invest").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();

    // 2. Delete with -y (no stdin needed)
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000300")
        .arg("-y")
        .assert()
        .success()
        .stdout(predicate::str::contains("Skipping confirmation"));

    // 3. Verify DB is clean
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let count: i64 = conn.query_row("SELECT COUNT(*) FROM fund", [], |row| row.get(0)).unwrap();
        assert_eq!(count, 0);
    }
}

#[test]
fn test_sell_basic_flow() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-basic");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet and Fund
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("add").arg("Invest").assert().success();
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet").arg("use").arg("Invest").assert().success();
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();

    // 2. Buy some
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy").arg("000300").arg("--money").arg("1000").arg("--shares").arg("1000").arg("--nav").arg("1.0").assert().success();

    // 3. Sell half
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("sell").arg("000300").arg("--shares").arg("500").arg("--nav").arg("1.2")
        .write_stdin("y\n")
        .assert().success()
        .stdout(predicate::str::contains("Sold 000300: 500 shares"));

    // 4. Check status (Remaining 500 shares)
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status")
        .assert().success()
        .stdout(predicate::str::contains("500"));
}

#[test]
fn test_sell_auto_calculation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-auto");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet, Fund, Buy 1000 shares at 1.0
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();
    
    // Day 1 NAV = 1.0
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-08', '1.0')", []).unwrap();
    }

    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy").arg("000300").arg("--money").arg("1000").arg("--shares").arg("1000").arg("--nav").arg("1.0").assert().success();

    // 2. Sell $600 worth of shares when NAV is 1.2
    // Expected shares = 600 / 1.2 = 500
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute("INSERT OR REPLACE INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.2')", []).unwrap();
    }

    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("sell").arg("000300").arg("--money").arg("600").arg("--auto")
        .write_stdin("y\n")
        .assert().success();
}

#[test]
fn test_sell_insufficient_shares() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-insufficient");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet, Fund, Buy 100
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Invest").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str).arg("buy").arg("000300").arg("--money").arg("100").arg("--shares").arg("100").arg("--nav").arg("1.0").assert().success();

    // 2. Sell 150 (Insufficient)
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", app_dir_str)
       .arg("sell")
       .arg("000300")
       .arg("--shares")
       .arg("150")
       .arg("--nav")
       .arg("1.0")
       .assert()
       .failure()
       .stderr(predicate::str::contains("Insufficient shares"));
}

#[test]
fn test_sell_preview_confirmation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-confirm");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet, Fund, Buy 1000
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Main").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Main").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("buy").arg("000300").arg("--money").arg("1000").arg("--shares").arg("1000").arg("--nav").arg("1.0").assert().success();

    // 2. Sell with confirmation - User says No
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("sell").arg("000300").arg("--shares").arg("500").arg("--nav").arg("1.2")
        .write_stdin("n\n")
        .assert()
        .success()
        .stdout(predicate::str::contains("Transaction cancelled"));

    // Verify shares still 1000
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status").assert().success().stdout(predicate::str::contains("1000"));

    // 3. Sell with confirmation - User says Yes
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("sell").arg("000300").arg("--shares").arg("500").arg("--nav").arg("1.2")
        .write_stdin("y\n")
        .assert()
        .success()
        .stdout(predicate::str::contains("Sold 000300"));

    // Verify shares 500
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status").assert().success().stdout(predicate::str::contains("500"));
}

#[test]
fn test_sell_help_output() {
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.arg("sell").arg("--help")
       .assert()
       .success()
       .stdout(predicate::str::contains("--shares <SHARES>"))
       .stdout(predicate::str::contains("--fee <FEE>"))
       .stdout(predicate::str::contains("--money <MONEY>"))
       .stdout(predicate::str::contains("--nav <NAV>"));
}

#[test]
fn test_sell_all_shares() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-all");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: Wallet, Fund, Buy 1000
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("add").arg("Main").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("wallet").arg("use").arg("Main").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("fund").arg("add").arg("000300").arg("沪深300").assert().success();
    Command::cargo_bin("fund-manager").unwrap().env("FUND_MANAGER_APP_DIR", app_dir_str).arg("buy").arg("000300").arg("--money").arg("1000").arg("--shares").arg("1000").arg("--nav").arg("1.0").assert().success();

    // 2. Sell all
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("sell").arg("000300").arg("--shares").arg("all").arg("--nav").arg("1.2")
        .write_stdin("y\n")
        .assert()
        .success()
        .stdout(predicate::str::contains("Sold 000300: 1000 shares"));

    // 3. Check status - should be 0 or not listed
    Command::cargo_bin("fund-manager").unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("status").assert().success().stdout(predicate::str::contains("0.00"));
}
