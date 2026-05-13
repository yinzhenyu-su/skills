# Design: Implement `wallet delete` subcommand

## UI/UX Changes
Add a new subcommand `delete` (and its alias `del`) to the `wallet` command.

```bash
fund wallet delete <name>
fund wallet del <name>
```

The command will require a single positional argument `<name>` representing the name of the wallet to be deleted.

### Confirmation Workflow
1.  Verify the wallet exists.
2.  If the wallet exists, display a warning:
    "Are you sure you want to delete wallet '<name>'? This will PERMANENTLY remove ALL its transaction history."
3.  Wait for user confirmation (y/N).
4.  If confirmed, proceed with deletion.
5.  If it was the active wallet, clear the active wallet configuration or pick another one.

## Database Changes
Update `db.rs` to include:
```rust
pub fn delete_wallet_by_name(conn: &Connection, name: &str) -> Result<usize> {
    conn.execute("DELETE FROM wallet WHERE name = ?1", [name])
}

pub fn clear_active_wallet(conn: &Connection) -> Result<()> {
    conn.execute("DELETE FROM app_config WHERE key = 'active_wallet_id'", [])?;
    Ok(())
}
```

Wait, `clear_active_wallet` could also be `db::set_active_wallet(conn, 0)` or something if we don't want to delete the key. But `DELETE FROM app_config WHERE key = 'active_wallet_id'` is cleaner.

## Logic Changes (main.rs)
In the match arm for `WalletCommands`:
```rust
            WalletCommands::Delete { name } => {
                let wallet_id_opt = db::get_wallet_id_by_name(&conn, &name).expect("DB error");
                if let Some(id) = wallet_id_opt {
                    let active_id = db::get_active_wallet_id(&conn).expect("DB error");
                    let is_active = Some(id) == active_id;

                    let prompt = format!(
                        "Are you sure you want to delete wallet '{}' and ALL its transaction history?",
                        name
                    );
                    if confirm_action(&prompt, cli.yes) {
                        db::delete_wallet_by_name(&conn, &name).expect("Failed to delete wallet");
                        if is_active {
                            db::clear_active_wallet(&conn).expect("Failed to clear active wallet");
                            println!("Successfully deleted wallet: {}. (It was the active wallet, now no active wallet is selected.)", name);
                        } else {
                            println!("Successfully deleted wallet: {}", name);
                        }
                    } else {
                        println!("Deletion cancelled.");
                    }
                } else {
                    eprintln!("Error: Wallet '{}' does not exist.", name);
                    std::process::exit(1);
                }
            }
```

## Considerations
- **Multiple Wallets**: If the active wallet is deleted, maybe we should automatically set another one as active?
  *   Current decision: Just clear it and let the user set a new one with `fund wallet use <name>`. This is simpler and less surprising.
- **Empty Wallets**: Should we skip confirmation for empty wallets?
  *   Current decision: No, keep it consistent.
