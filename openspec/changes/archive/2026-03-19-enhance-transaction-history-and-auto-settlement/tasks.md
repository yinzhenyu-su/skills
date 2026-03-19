# Tasks: Transaction History & Auto Settlement

## Phase 1: Foundation & Database
- [x] Add `memo` and `source` columns to `transaction_log` table in `db.rs`.
- [x] Implement migration logic for existing data.
- [x] Update `db::add_transaction` (or equivalent) to support new fields and types.

## Phase 2: History Command
- [x] Create `db::get_transaction_history` with filters (fund, wallet, type, limit).
- [x] Implement `History` subcommand in `cli.rs`.
- [x] Develop the UI table rendering in `main.rs` using `comfy-table`.
- [x] Add color coding for different transaction types.

## Phase 3: Auto Settlement
- [x] Implement `finance::settle_transaction` logic.
- [x] Integrate settlement check into `sync::sync_fund_details`.
- [x] Add a confirmation message/summary in the `fund sync` output.
- [x] Add tests for settlement edge cases (e.g., NAV still missing).

## Phase 4: New Transaction Business Logic
- [x] Add `fund dividend` command (or expand `Buy/Sell`).
- [x] Update `db::get_holdings` calculation to correctly account for dividends and reinvestments.
- [x] Verify cost basis and profit calculations.

## Phase 5: Polish & Validation
- [x] Add integration tests for the full lifecycle (Buy pending -> Sync -> Settled).
- [x] Update help messages and localization.
