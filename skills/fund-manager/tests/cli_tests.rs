mod common;

use assert_cmd::Command;
use common::context::TestContext;
use predicates::prelude::*;
use std::env;
use std::fs;

const TEST_DATE: &str = "2026-03-09";

#[test]
fn test_wallet_add() {
    let ctx = TestContext::new("wallet-add");

    // fund wallet add "Test Wallet"
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("Test Wallet")
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "Successfully added wallet: Test Wallet",
        ));

    // Verify DB entry
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("Test Wallet")
        .assert()
        .failure()
        .stderr(predicate::str::contains("already exists"));
}

#[test]
fn test_wallet_use() {
    let ctx = TestContext::new("wallet-use");

    // 1. Add two wallets
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("Wallet1")
        .assert()
        .success();

    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("Wallet2")
        .assert()
        .success();

    // 2. Switch to Wallet2
    ctx.cmd()
        .arg("wallet")
        .arg("use")
        .arg("Wallet2")
        .assert()
        .success()
        .stdout(predicate::str::contains("Now using wallet: Wallet2"));

    // 3. Switch to non-existent wallet
    ctx.cmd()
        .arg("wallet")
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
        .stderr(predicate::str::contains(
            "Warning: Could not fetch latest data",
        ));
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
    cmd.arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();

    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();

    // Manually insert fund and NAV for testing
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('000300', '沪深300', '0.0015')",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', ?1, '1.25')",
            [TEST_DATE],
        )
        .unwrap();
    }

    // 2. Run fund buy (auto mode is now default, no --auto flag needed)
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.env("SKIP_SYNC", "1");
    cmd.arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1001.50")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success()
        .stdout(predicate::str::contains("Bought 000300"))
        .stdout(predicate::str::contains(TEST_DATE));
}

#[test]
fn test_status_valuation_and_pl() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-status-pl");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Main").assert().success();
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("use").arg("Main").assert().success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('000300', '沪深300', '0.00')",
            [],
        )
        .unwrap();
        // Day 1: Buy 1000 shares at 1.00 (Cost = 1000)
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-08', '1.00')", []).unwrap();
        conn.execute("INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date, status) 
                      VALUES (1, '000300', 'buy', '1000', '1000', '1.00', '0', '2026-03-08', 'settled')", []).unwrap();
        // Day 2: NAV goes to 1.20
        conn.execute("INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.20')", []).unwrap();
    }

    // 2. Run fund status with sync disabled
    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.env("SKIP_SYNC", "1");
    cmd.arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("1200.00"))
        .stdout(predicate::str::contains("200.00"))
        .stdout(predicate::str::contains("20.00%"));
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
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("E2E")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("E2E")
        .assert()
        .success();

    // 2. Add Fund
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000001")
        .arg("Fund1")
        .assert()
        .success();

    // 2.5 Mock NAV
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000001', ?1, '1.50')",
            [TEST_DATE],
        )
        .unwrap();
    }

    // 3. Buy 1500 (auto settles because nav exists)
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("000001")
        .arg("--money")
        .arg("1500")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success();

    // 4. Sell 400
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000001")
        .arg("--shares")
        .arg("400")
        .arg("--nav")
        .arg("1.50")
        .arg("--date")
        .arg(TEST_DATE)
        .write_stdin("y\n")
        .assert()
        .success();

    // 5. Check status
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("000001"));

    // 6. Delete fund
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000001")
        .write_stdin("y\n")
        .assert()
        .success();

    // 7. Verify empty status
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
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
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("WalletA")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("WalletB")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("WalletA")
        .assert()
        .success();

    // Mock db
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('160119', '南方500', '0.00')",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('160119', ?1, '1.00')",
            [TEST_DATE],
        )
        .unwrap();
    }

    // 2. Buy fund 160119 for WalletB
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("160119")
        .arg("--money")
        .arg("1000")
        .arg("--wallet")
        .arg("WalletB")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success()
        .stdout(predicate::str::contains("Bought 160119"));

    // 3. Verify WalletA is empty
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status") // Uses WalletA by default
        .assert()
        .success()
        .stdout(predicate::str::contains("160119").not());

    // 4. Verify WalletB has data in wallet list
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
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

    // 1. Setup
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1000")
        .arg("--shares")
        .arg("1000")
        .arg("--nav")
        .arg("1.0")
        .assert()
        .success();

    // 2. Delete Fund
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000300")
        .write_stdin("y\n")
        .assert()
        .success();

    // 3. Verify DB is clean
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        let fund_count: i64 = conn
            .query_row("SELECT COUNT(*) FROM fund", [], |row| row.get(0))
            .unwrap();
        assert_eq!(fund_count, 0);
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

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000300")
        .arg("-y")
        .assert()
        .success();
}

#[test]
fn test_sell_basic_flow() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-basic");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1000")
        .arg("--shares")
        .arg("1000")
        .arg("--nav")
        .arg("1.0")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--shares")
        .arg("500")
        .arg("--nav")
        .arg("1.2")
        .arg("--date")
        .arg(TEST_DATE)
        .write_stdin("y\n")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
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

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-08', '1.0')",
            [],
        )
        .unwrap();
        conn.execute("INSERT OR REPLACE INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.2')", []).unwrap();
    }

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1000")
        .arg("--shares")
        .arg("1000")
        .arg("--nav")
        .arg("1.0")
        .arg("--date")
        .arg("2026-03-08")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--money")
        .arg("600")
        .arg("--date")
        .arg("2026-03-09")
        .write_stdin("y\n")
        .assert()
        .success();
}

#[test]
fn test_sell_insufficient_shares() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-insufficient");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("100")
        .arg("--shares")
        .arg("100")
        .arg("--nav")
        .arg("1.0")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--shares")
        .arg("150")
        .arg("--nav")
        .arg("1.0")
        .assert()
        .failure();
}

#[test]
fn test_sell_all_shares() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-all");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1000")
        .arg("--shares")
        .arg("1000")
        .arg("--nav")
        .arg("1.0")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--shares")
        .arg("all")
        .arg("--nav")
        .arg("1.2")
        .arg("--date")
        .arg(TEST_DATE)
        .write_stdin("y\n")
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("0.00"));
}

#[test]
fn test_fund_import_arguments() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-import-args");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', ?1, '1.0')",
            [TEST_DATE],
        )
        .unwrap();
    }

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import")
        .arg("000300")
        .arg("1000")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("1000"));
}

#[test]
fn test_fund_import_csv() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-import-csv");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2024-01-01', '1.0')",
            [],
        )
        .unwrap();
    }

    let csv_path = temp_app_dir.join("data.csv");
    fs::write(
        &csv_path,
        "name,money,date\n000300,5000,2024-01-01\ninvalid,100,2024-01-01\n",
    )
    .unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import")
        .arg("--file")
        .arg(csv_path.to_str().unwrap())
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("5000"));
}

#[test]
fn test_fund_import_override() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-import-override");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .arg("沪深300")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', ?1, '1.0')",
            [TEST_DATE],
        )
        .unwrap();
    }

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("000300")
        .arg("--money")
        .arg("1000")
        .arg("--shares")
        .arg("1000")
        .arg("--nav")
        .arg("1.0")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import")
        .arg("000300")
        .arg("5000")
        .arg("--override")
        .arg("--date")
        .arg(TEST_DATE)
        .assert()
        .success();

    Command::cargo_bin("fund-manager")
        .unwrap()
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("5000"))
        .stdout(predicate::str::contains("6000").not());
}

#[test]
fn test_fund_inspect_not_found() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-inspect-not-found");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let mut cmd = Command::cargo_bin("fund-manager").unwrap();
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    
    // fund fund inspect 999999
    cmd.arg("fund")
        .arg("inspect")
        .arg("999999")
        .assert()
        .failure()
        .stderr(predicate::str::contains("Failed to fetch"));
}
