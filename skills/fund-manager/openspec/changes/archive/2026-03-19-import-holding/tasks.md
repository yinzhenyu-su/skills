## 1. CLI Commands

- [x] 1.1 Add `ImportHolding` variant to `Commands` enum in `cli.rs`
- [x] 1.2 Add `PreviewBuy` variant to `Commands` enum in `cli.rs`
- [x] 1.3 Add `PreviewSell` variant to `Commands` enum in `cli.rs`
- [x] 1.4 Define CLI arguments for `import-holding`: `--file`, `--wallet`, `--merge`, `--override`
- [x] 1.5 Define CLI arguments for `preview`: `<fund>`, `--money`, `--shares`, `--nav`, `--date`
- [x] 1.6 Define CLI arguments for `preview-sell`: `<fund>`, `--shares`, `--money`, `--nav`, `--date`
- [x] 1.7 Update command dispatch in `main.rs` to route to handlers

## 2. Import Logic

- [x] 2.1 Implement CSV parsing function for holdings format (名称,持有金额,持有收益)
- [x] 2.2 Implement calculation function: `calculate_holding_from_profit(amount, profit, nav) -> (shares, cost_basis)`
- [x] 2.3 Implement `handle_import_holding` async function
- [x] 2.4 Integrate NAV lookup via existing provider system
- [x] 2.5 Integrate fund name resolution via existing resolver
- [x] 2.6 Write import record to transaction_log with type='import'
- [x] 2.7 Implement merge mode logic (skip if import record exists)
- [x] 2.8 Implement override mode logic (delete existing import record then insert)
- [x] 2.9 Implement import result reporting

## 3. Preview Buy Logic

- [x] 3.1 Implement `handle_preview_buy` async function
- [x] 3.2 Resolve NAV from --nav, --date, or latest (priority order)
- [x] 3.3 Calculate fee and shares from money/shares input
- [x] 3.4 Retrieve current holdings for the fund
- [x] 3.5 Calculate post-buy holdings changes (total shares, total cost, average cost)
- [x] 3.6 Format and display preview output
- [x] 3.7 Handle case when fund has no existing holdings

## 4. Preview Sell Logic

- [x] 4.1 Implement `handle_preview_sell` async function
- [x] 4.2 Resolve NAV from --nav, --date, or latest (priority order)
- [x] 4.3 Calculate shares and amount from shares/money input
- [x] 4.4 Retrieve current holdings for the fund
- [x] 4.5 Check if sell shares exceeds holding (show warning)
- [x] 4.6 Calculate fee and net received
- [x] 4.7 Calculate post-sell holdings changes (total shares, total cost, average cost)
- [x] 4.8 Format and display preview output

## 5. Integration & Testing

- [ ] 5.1 Write integration tests for `import-holding` command
- [ ] 5.2 Test merge mode behavior
- [ ] 5.3 Test override mode behavior
- [ ] 5.4 Test CSV parsing edge cases
- [ ] 5.5 Write integration tests for `preview` command
- [ ] 5.6 Write integration tests for `preview-sell` command
- [ ] 5.7 Test --nav and --date parameter combinations
- [ ] 5.8 Test calculation accuracy with decimal precision
- [ ] 5.9 Verify `fund status` correctly aggregates import + buy + sell records
- [x] 5.10 Run `cargo fmt` and `cargo clippy`
