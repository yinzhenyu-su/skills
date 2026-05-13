# Design: Transaction Management & Auto Settlement

## Database Changes

### `transaction_log` Table Extension
Modify the table (or migration) to include:
- `memo`: TEXT (Optional user notes)
- `source`: TEXT (Default: 'manual', options: 'csv', 'import')
- `type`: Expand logic to handle `dividend`, `reinvest`, `import`.

## Functional Design

### 1. Automated Settlement Logic
When `fund sync` (or `sync_fund_details`) is executed:
1. Identify all transactions where `status = 'pending'`.
2. For each pending transaction:
   - Search `nav_history` for an entry matching `transaction.date`.
   - If found:
     - If `type = 'buy'`: Calculate `shares` and `fee` using the stored `money` and retrieved `nav`.
     - If `type = 'sell'`: Calculate `money` (payout) using `shares` and `nav`.
     - Update the record with calculated values and set `status = 'settled'`.
   - If not found: Keep as `pending`.
3. Provide a summary in the CLI output (e.g., "✅ 3 transactions settled").

### 2. Expanded Transaction Types
- **`dividend`**: Decreases `net_cost` of the holding. `money` is the cash received. `shares` remains 0.
- **`reinvest`**: Increases `shares`. `money` is 0 (or internal reinvestment amount). Does not change `net_cost` (as it's self-funded from dividends).
- **`import`**: Increases both `shares` and `net_cost` (Initial entry).

### 3. CLI Command: `fund history`
**Syntax:** `fund history [FUND] [--wallet <NAME>] [--type <TYPE>] [--limit <N>]`

**Table Columns:**
- Date (YYYY-MM-DD)
- Type (Colored: Buy-Green, Sell-Red, etc.)
- Money (Right-aligned)
- NAV (Net Asset Value)
- Shares (Right-aligned)
- Fee
- Wallet
- Status (Icon: ✅/⏳)

## Calculation Rules (Finance)
- **Net Cost** = Σ(Buy Money + Fees) - Σ(Sell Money - Fees) - Σ(Cash Dividends).
- **Current Valuation** = Current Shares * Latest NAV.
- **Total Profit** = Current Valuation - Net Cost.
