# Tasks: Automated Dividend Tracking

## Phase 1: Database and Schema

- [x] Add `dividend_mode` column to `fund` table. (Update `db.rs` migration).
- [x] Implement `db::update_fund_dividend_mode` to update the preference.

## Phase 2: Core Logic (Smart Detection)

- [x] Implement `finance::detect_dividend` function to calculate dividend per share from two NAV/AccNAV pairs.
- [x] Implement `db::get_earliest_transaction_date` for a wallet/fund.
- [x] Enhance `sync_funds` to use `detect_dividend` while iterating through the fetched range.
- [x] Integrate \"Time Isolation Wall\" logic into the dividend recording flow in `sync.rs`.
- [x] Ensure `sync_funds` automatically records `dividend` or `reinvest` transactions based on the `dividend_mode`.

## Phase 3: CLI and UI

- [x] Add `fund fund config <CODE> --dividend-mode <cash|reinvest>` subcommand in `cli.rs` and `main.rs`.
- [x] Update `db::get_holdings` to include `cumulative_dividend`.
- [x] Update `status` command output to include the \"累计分红\" column.
- [x] Update `wallet list` command output to include \"累计分红\" in the portfolio summary.

## Phase 4: Testing and Verification

- [x] Add unit test for `finance::detect_dividend`.
- [x] Add integration test for `sync` with automatic dividend recording.
- [x] Verify that `import-holding` historical dividends are correctly ignored.
