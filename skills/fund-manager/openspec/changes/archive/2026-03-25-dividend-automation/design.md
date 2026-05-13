# Design: Automated Dividend Tracking

## Architecture Overview

The system will leverage the existing `sync` and `db` modules to detect and record dividends.

### 1. Database Schema Updates

We need to store fund-specific configurations, including the dividend handling mode.

```sql
ALTER TABLE fund ADD COLUMN dividend_mode TEXT DEFAULT 'cash'; -- 'cash' or 'reinvest'
```

### 2. Smart Dividend Detection Logic

The logic will reside in `sync.rs`. During the `sync_funds` process:

1.  Fetch $NAV$ and $AccNAV$ history from `EastmoneyLsjzProvider`.
2.  Iterate through the history. For each day $t$ and $t-1$:
    *   Calculate $D = (AccNAV_t - AccNAV_{t-1}) - (NAV_t - NAV_{t-1})$.
    *   If $D > 0.0001$ (allowing for minor precision errors):
        *   **Check "Time Isolation Wall"**:
            *   Query $MIN(date)$ of all transactions for this fund in the current wallet.
            *   If $t \le MIN(date)$, ignore the dividend.
            *   If $t > MIN(date)$, proceed to recording.
        *   **Record Transaction**:
            *   Get current `total_shares` for the fund in that wallet at date $t$.
            *   If `dividend_mode` is `cash`:
                *   Insert `type='dividend'`, `money = D * total_shares`.
            *   If `dividend_mode` is `reinvest`:
                *   Insert `type='reinvest'`, `shares = (D * total_shares) / NAV_t`.

### 3. UI Changes

*   **`db::get_holdings`**: Calculate `cumulative_dividend` by summing `money` for `type='dividend'`.
*   **`main::handle_status`**: Add a "累计分红" column to the `Table`.
*   **`main::handle_wallet_list`**: Add a "累计分红" column to the summary table.

### 4. New CLI Commands

*   `fund fund config <CODE> --dividend-mode <cash|reinvest>`: Update the `dividend_mode` for a fund.

## Data Consistency and Precision

*   Use `Decimal` (from `rust_decimal`) for all calculations.
*   The $AccNAV$ baseline strategy will be implicitly handled by the $t \le MIN(date)$ check, but for `import` transactions, we must ensure the `import` date is treated as the starting point.
