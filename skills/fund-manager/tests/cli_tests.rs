mod common;

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
        .stdout(predicate::str::contains("成功添加钱包：Test Wallet"));

    // Verify DB entry
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("Test Wallet")
        .assert()
        .failure()
        .stderr(predicate::str::contains("已存在"));
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
        .stdout(predicate::str::contains("当前已切换至钱包：Wallet2"));

    // 3. Switch to non-existent wallet
    ctx.cmd()
        .arg("wallet")
        .arg("use")
        .arg("NoSuchWallet")
        .assert()
        .failure()
        .stderr(predicate::str::contains("不存在"));
}

#[test]
fn test_status_sync_failure_warning() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sync-failure");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // 1. Setup: Add a wallet and switch to it
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Main").assert().success();

    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("use").arg("Main").assert().success();

    // 2. Run fund status with forced sync failure
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.env("FORCE_SYNC_FAILURE", "1");
    cmd.arg("status")
        .assert()
        .success() // App should not crash
        .stderr(predicate::str::contains("⚠️ 警告：无法获取最新数据"));
}

#[test]
fn test_buy_auto_calculation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-buy-auto");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    // 1. Setup: Add wallet, switch to it
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();

    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
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
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
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
        .stdout(predicate::str::contains("成功买入 000300"))
        .stdout(predicate::str::contains(TEST_DATE));
}

#[test]
fn test_status_valuation_and_pl() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-status-pl");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());
    cmd.arg("wallet").arg("add").arg("Main").assert().success();
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
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
    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("E2E")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("E2E")
        .assert()
        .success();

    // 2. Add Fund
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000001")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
        .arg("-y")
        .assert()
        .success();

    // 5. Check status
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("000001"));

    // 6. Delete fund
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000001")
        .arg("-y")
        .assert()
        .success();

    // 7. Verify empty status
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("WalletA")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("WalletB")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
        .stdout(predicate::str::contains("成功买入 160119"));

    // 3. Verify WalletA is empty
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status") // Uses WalletA by default
        .assert()
        .success()
        .stdout(predicate::str::contains("160119").not());

    // 4. Verify WalletB has data in wallet list
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("delete")
        .arg("000300")
        .arg("-y")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
        .arg("-y")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--money")
        .arg("600")
        .arg("--date")
        .arg("2026-03-09")
        .arg("-y")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
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
        .arg("-y")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("0.00"));
}

#[test]
fn test_fund_import_holding_csv() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-import-holding-csv");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
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

    // CSV format: 基金名称,持有金额,持有收益
    let csv_path = temp_app_dir.join("holdings.csv");
    fs::write(
        &csv_path,
        "name,amount,profit\n000300,5000,500\ninvalid,100,10\n",
    )
    .unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import-holding")
        .arg("--file")
        .arg(csv_path.to_str().unwrap())
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("5000"));
}

#[test]
fn test_fund_import_holding_override() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-import-holding-override");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Main")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("000300")
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

    // First import: 持有5000，盈利500
    let csv_path1 = temp_app_dir.join("holdings1.csv");
    fs::write(&csv_path1, "name,amount,profit\n000300,5000,500\n").unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import-holding")
        .arg("--file")
        .arg(csv_path1.to_str().unwrap())
        .assert()
        .success();

    // Override import: 持有8000，盈利800
    let csv_path2 = temp_app_dir.join("holdings2.csv");
    fs::write(&csv_path2, "name,amount,profit\n000300,8000,800\n").unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("import-holding")
        .arg("--file")
        .arg(csv_path2.to_str().unwrap())
        .arg("--override")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("8000"))
        .stdout(predicate::str::contains("5000").not());
}

#[test]
fn test_fund_inspect_not_found() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-inspect-not-found");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();

    let mut cmd = assert_cmd::cargo::cargo_bin_cmd!("fund-manager");
    cmd.env("FUND_MANAGER_APP_DIR", temp_app_dir.to_str().unwrap());

    // fund fund inspect 999999
    cmd.arg("fund")
        .arg("inspect")
        .arg("999999")
        .assert()
        .failure()
        .stderr(predicate::str::contains("未找到基金"));
}

/// 测试用例 457001：基金盈亏计算验证
/// 真实数据：
/// - 基金代码: 457001
/// - 基金名称: 国富亚洲机会股票(QDII)A
/// - 申购费率: 0.15%
/// - 买入日期净值 (2026-03-02): 2.2583
/// - 当前净值 (2026-03-16): 2.1146
///
/// 预期计算：
/// - 投入 10000 元，手续费 14.99 元，净金额 9985.01 元
/// - 份额 = 9985.01 / 2.2583 = 4421.46 份
/// - 当前市值 = 4421.46 × 2.1146 = 9350.04 元
/// - 盈亏额 = 9350.04 - 10000 = -649.96 元
/// - 盈亏率 = -6.50%
///
/// 边界测试（卖出操作）：
/// - 卖出 5000 元后，剩余份额约 2056.97 份
/// - 持仓成本 = 10000 - 5000 = 5000 元
/// - 当前市值 = 2056.97 × 2.1146 ≈ 4349.67 元
/// - 盈亏额 ≈ -650.34 元
#[test]
fn test_fund_457001_profit_loss_calculation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-457001-pl");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // 1. Setup: 创建钱包并添加基金
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("TestWallet")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("TestWallet")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("add")
        .arg("457001")
        .arg("--fee")
        .arg("0.0015")
        .assert()
        .success();

    // 2. 手动插入净值数据
    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        // 买入日期净值
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('457001', '2026-03-02', '2.2583')",
            [],
        )
        .unwrap();
        // 当前净值
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('457001', '2026-03-16', '2.1146')",
            [],
        )
        .unwrap();
    }

    // 3. 执行买入操作：投入 10000 元
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("buy")
        .arg("457001")
        .arg("--money")
        .arg("10000")
        .arg("--date")
        .arg("2026-03-02")
        .assert()
        .success()
        .stdout(predicate::str::contains("4421.48")); // 预期份额

    // 4. 验证盈亏状态
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .arg("457001")
        .assert()
        .success()
        .stdout(predicate::str::contains("4421.48"))
        .stdout(predicate::str::contains("10000"))
        .stdout(predicate::str::contains("2.1146"))
        .stdout(predicate::str::contains("-6.50%"));

    // 5. 边界测试：执行卖出操作
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("457001")
        .arg("--money")
        .arg("5000")
        .arg("--date")
        .arg("2026-03-16")
        .arg("-y")
        .assert()
        .success();

    // 6. 验证卖出后的盈亏状态
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .arg("457001")
        .assert()
        .success()
        // 剩余份额约 2056.97
        .stdout(predicate::str::contains("2056"))
        // 持仓成本约 5000
        .stdout(predicate::str::contains("5000"))
        // 盈亏率约 -13%
        .stdout(predicate::str::contains("-13.0"));
}

#[test]
fn test_misplaced_subcommand_hint() {
    let ctx = TestContext::new("misplaced-subcommand");

    // fund wallet list use
    ctx.cmd()
        .arg("wallet")
        .arg("list")
        .arg("use")
        .assert()
        .failure()
        .stderr(predicate::str::contains("❌ 未识别的参数或子命令 'use'"))
        .stderr(predicate::str::contains(
            "💡 Hint: 你是不是想找：'fund wallet use'？",
        ));

    // fund walllet list (typo)
    ctx.cmd()
        .arg("walllet")
        .arg("list")
        .assert()
        .failure()
        .stderr(predicate::str::contains(
            "❌ 未识别的参数或子命令 'walllet'",
        ))
        .stderr(predicate::str::contains(
            "❓ 未识别的子命令 'walllet'。你是不是想找：'wallet'？",
        ));
}

#[test]
fn test_missing_required_argument_format() {
    let ctx = TestContext::new("missing-args");

    // fund buy (missing fund and money)
    ctx.cmd()
        .arg("buy")
        .assert()
        .failure()
        .stderr(predicate::str::contains("❌ 缺少基金标识符参数"))
        .stderr(predicate::str::contains("用法示例：fund buy"));
}

#[test]
fn test_index_command() {
    let ctx = TestContext::new("index-command");
    ctx.cmd().arg("index").assert().success();
}

#[test]
fn test_status_columns_holding_days_fee_allocation() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-status-cols");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Test")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Test")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('000300', '沪深300', '0.00')",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2024-01-01', '1.00')",
            [],
        ).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-01-01', '1.20')",
            [],
        ).unwrap();
        conn.execute(
            "INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date, status)
             VALUES (1, '000300', 'buy', '1000', '1000', '1.00', '0', '2024-01-01', 'settled')",
            [],
        ).unwrap();
    }

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("持有天数"))
        .stdout(predicate::str::contains("赎回费率"))
        .stdout(predicate::str::contains("仓位占比"));
}

#[test]
fn test_preview_sell_shows_holding_days_and_fee_rate() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-preview-sell-fee");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Test")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Test")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('000300', '沪深300', '0.00')",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-01-01', '1.50')",
            [],
        ).unwrap();
        conn.execute(
            "INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date, status)
             VALUES (1, '000300', 'buy', '1000', '1000', '1.00', '0', '2026-01-01', 'settled')",
            [],
        ).unwrap();
        // Insert a redemption fee tier: 0+ days → 0.5%
        conn.execute(
            "INSERT INTO redemption_fee_tiers (fund_code, min_days, max_days, fee_rate) VALUES ('000300', 0, NULL, '0.005')",
            [],
        ).unwrap();
    }

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("preview")
        .arg("sell")
        .arg("000300")
        .arg("--shares")
        .arg("500")
        .arg("--nav")
        .arg("1.50")
        .assert()
        .success()
        .stdout(predicate::str::contains("赎回费率"));
}

#[test]
fn test_sell_auto_fee_rate_from_tiers() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-sell-auto-fee");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Test")
        .assert()
        .success();
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Test")
        .assert()
        .success();

    {
        let db_path = temp_app_dir.join("fund.db");
        let conn = rusqlite::Connection::open(db_path).unwrap();
        conn.execute(
            "INSERT INTO fund (code, name, management_fee) VALUES ('000300', '沪深300', '0.00')",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-01-01', '1.00')",
            [],
        ).unwrap();
        conn.execute(
            "INSERT INTO nav_history (fund_code, date, nav) VALUES ('000300', '2026-03-09', '1.20')",
            [],
        ).unwrap();
        conn.execute(
            "INSERT INTO transaction_log (wallet_id, fund_code, type, money, shares, nav, fee, date, status)
             VALUES (1, '000300', 'buy', '1000', '1000', '1.00', '0', '2026-01-01', 'settled')",
            [],
        ).unwrap();
        // Fee tier: 0+ days → 0.5%
        conn.execute(
            "INSERT INTO redemption_fee_tiers (fund_code, min_days, max_days, fee_rate) VALUES ('000300', 0, NULL, '0.005')",
            [],
        ).unwrap();
    }

    // sell without --fee: should auto-apply tier
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("sell")
        .arg("000300")
        .arg("--shares")
        .arg("500")
        .arg("--nav")
        .arg("1.20")
        .arg("--date")
        .arg(TEST_DATE)
        .arg("-y")
        .assert()
        .success()
        .stdout(predicate::str::contains("赎回费用"));
}

#[test]
fn test_fund_reset_with_yes_flag() {
    let temp_app_dir = env::temp_dir().join("fund-manager-test-reset-yes");
    if temp_app_dir.exists() {
        fs::remove_dir_all(&temp_app_dir).unwrap();
    }
    fs::create_dir_all(&temp_app_dir).unwrap();
    let app_dir_str = temp_app_dir.to_str().unwrap();

    // Create a wallet and fund to populate the database
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("add")
        .arg("Invest")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("use")
        .arg("Invest")
        .assert()
        .success();

    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .env("SKIP_SYNC", "1")
        .arg("fund")
        .arg("add")
        .arg("000300")
        .assert()
        .success();

    // Verify wallet exists before reset
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("Invest"));

    // Run reset with -y flag
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("fund")
        .arg("reset")
        .arg("-y")
        .assert()
        .success()
        .stdout(predicate::str::contains("✅ 数据已重置"));

    // Verify all wallets are gone after reset
    assert_cmd::cargo::cargo_bin_cmd!("fund-manager")
        .env("FUND_MANAGER_APP_DIR", app_dir_str)
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("钱包名称")); // Header only, no wallets
}
