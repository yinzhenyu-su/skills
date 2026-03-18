# Proposal: Add `wallet delete` (del) subcommand

## Problem
Currently, there's no way to delete a wallet from the CLI. Once a wallet is created, it persists in the database forever unless manually removed using an external SQLite tool.

## Proposed Solution
Add a `delete` subcommand (with `del` alias) to the `wallet` command. This will allow users to remove a wallet and all its associated transaction data.

## Scope
- Add `Delete { name: String }` to `WalletCommands`.
- Add `delete_wallet_by_name` to `db.rs`.
- Handle confirmation and active wallet cleanup in `main.rs`.

## Constraints & Considerations
- Use `ON DELETE CASCADE` to clean up related transactions automatically.
- Require confirmation before deletion.
- If the active wallet is deleted, the active wallet configuration should be cleared or updated.
- Prevent deleting the only wallet? (Or just warn)

## Success Criteria
- Running `fund wallet delete <name>` successfully removes the wallet and its transactions.
- Running `fund wallet del <name>` also works.
- Confirmation prompt appears.
- If active wallet is deleted, it's no longer marked as active in `fund wallet list`.
