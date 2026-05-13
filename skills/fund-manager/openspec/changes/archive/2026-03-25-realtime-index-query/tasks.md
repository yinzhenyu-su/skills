## 1. Data Provider Implementation

- [x] 1.1 Define `IndexData` struct to represent index name, current value, absolute change, and percentage change.
- [x] 1.2 Use the Morningstar API (`https://www.morningstar.cn/cn-api/v2/market/watch-list`) and define `serde` structs (e.g., `WatchListResponse`, `WatchListData`, `IndexItem`) to deserialize the JSON response.
- [x] 1.3 Add an async data fetching function (e.g., `fetch_indices()`) using `reqwest` that requests the Morningstar JSON API.
- [x] 1.4 Implement mapping logic to filter and extract the required indices (e.g., 沪深300, 纳斯达克综合, etc.) from `chinaEquity` and `globalEquity` into a clean list of `IndexData`. Ensure missing indices are ignored silently.
- [x] 1.5 Add support for filtering the final list based on an optional list of user-provided index names.
- [x] 1.6 Add error handling to gracefully handle network timeouts or parsing failures.
- [x] 1.7 Write unit tests for the parsing logic with sample JSON API responses.

## 2. CLI Implementation

- [x] 2.1 Add the `Index` subcommand to the main `FundManager` CLI definition in `src/cli.rs` (using `clap`). It should accept an optional list of target index names (`#[arg(num_args(0..))] names: Vec<String>`).
- [x] 2.2 Add comprehensive `long_about` documentation and examples to the `Index` subcommand in `src/cli.rs` so users know they can run `fund index` or `fund index 沪深300 纳斯达克`.
- [x] 2.3 Wire the `index` subcommand to a new handler function in `src/main.rs` or a dedicated module.
- [x] 2.4 In the handler logic, if any user-provided name is unrecognized, print a clear warning to stderr listing all available supported index names.

## 3. UI Display

- [x] 3.1 Implement a function to format the fetched `IndexData` into a `comfy-table::Table`. Consider grouping by market or keeping a fixed sorting order.
- [x] 3.2 Add color coding (red for positive changes, green for negative changes) using `crossterm` styles integrated with `comfy-table`.
- [x] 3.3 Ensure the table headers clearly show: Name, Current Value, Change, Change %.
- [x] 3.4 Add the data `lastUpdate` time to the terminal output (either in table title or as a footer).

## 4. Integration and Verification

- [x] 4.1 Connect the handler function to fetch the data, apply name filters (if any), and display it via the table formatter.
- [x] 4.2 Update integration tests (e.g., in `tests/cli_tests.rs`) to verify the CLI command runs without crashing (mocking the HTTP client if necessary).
- [x] 4.3 Verify color output, missing index handling, and filtered name querying manually in the terminal.
