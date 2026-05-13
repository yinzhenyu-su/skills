# Tasks: Implement `wallet delete` subcommand

## 1. Database Layer (db.rs)
- [x] Add `delete_wallet_by_name(conn, name)` function.
- [x] Add `clear_active_wallet(conn)` function to remove the 'active_wallet_id' entry from `app_config`.

## 2. CLI Definition (cli.rs)
- [x] Add `Delete` variant to `WalletCommands` enum.
- [x] Add `#[command(alias = "del")]` attribute.

## 3. Implementation (main.rs)
- [x] Handle `WalletCommands::Delete` match arm.
- [x] Include confirmation logic.
- [x] Handle active wallet cleanup if needed.

## 4. Verification
- [x] Create a new wallet.
- [x] Delete the wallet using `fund wallet delete`.
- [x] Verify the wallet is removed from `fund wallet list`.
- [x] Verify that transactions are also deleted (if any).
- [x] Verify the active wallet cleanup logic.
