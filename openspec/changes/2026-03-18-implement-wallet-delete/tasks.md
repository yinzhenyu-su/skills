# Tasks: Implement `wallet delete` subcommand

## 1. Database Layer (db.rs)
- [ ] Add `delete_wallet_by_name(conn, name)` function.
- [ ] Add `clear_active_wallet(conn)` function to remove the 'active_wallet_id' entry from `app_config`.

## 2. CLI Definition (cli.rs)
- [ ] Add `Delete` variant to `WalletCommands` enum.
- [ ] Add `#[command(alias = "del")]` attribute.

## 3. Implementation (main.rs)
- [ ] Handle `WalletCommands::Delete` match arm.
- [ ] Include confirmation logic.
- [ ] Handle active wallet cleanup if needed.

## 4. Verification
- [ ] Create a new wallet.
- [ ] Delete the wallet using `fund wallet delete`.
- [ ] Verify the wallet is removed from `fund wallet list`.
- [ ] Verify that transactions are also deleted (if any).
- [ ] Verify the active wallet cleanup logic.
