mod common;

use common::context::TestContext;
use predicates::prelude::*;

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
