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
            "成功添加钱包：Test Wallet",
        ));

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
fn test_wallet_delete() {
    let ctx = TestContext::new("wallet-delete");

    // 1. Add a wallet
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("ToDelete")
        .assert()
        .success();

    // 2. Delete it (using alias 'del' and -y)
    ctx.cmd()
        .arg("-y")
        .arg("wallet")
        .arg("del")
        .arg("ToDelete")
        .assert()
        .success()
        .stdout(predicate::str::contains("✅ 钱包 'ToDelete' 已成功删除。"));

    // 3. Verify it's gone from list
    ctx.cmd()
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("ToDelete").not());

    // 4. Try deleting non-existent wallet
    ctx.cmd()
        .arg("wallet")
        .arg("delete")
        .arg("NoSuchWallet")
        .assert()
        .failure()
        .stderr(predicate::str::contains("❌ 错误：找不到名为 'NoSuchWallet' 的钱包。"));
}

#[test]
fn test_wallet_delete_active() {
    let ctx = TestContext::new("wallet-delete-active");

    // 1. Add and use wallet
    ctx.cmd()
        .arg("wallet")
        .arg("add")
        .arg("ActiveWallet")
        .assert()
        .success();

    ctx.cmd()
        .arg("wallet")
        .arg("use")
        .arg("ActiveWallet")
        .assert()
        .success();

    // 2. Verify it is active
    ctx.cmd()
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("*"));

    // 3. Delete it
    ctx.cmd()
        .arg("-y")
        .arg("wallet")
        .arg("delete")
        .arg("ActiveWallet")
        .assert()
        .success()
        .stdout(predicate::str::contains("✅ 钱包 'ActiveWallet' 已成功删除。(由于该钱包原为活跃钱包，当前已重置为未选中任何钱包。)"));

    // 4. Verify no wallet is active
    ctx.cmd()
        .arg("wallet")
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("*").not());
}
