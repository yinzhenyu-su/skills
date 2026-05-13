# Real-time Index Query

## Requirements

### Requirement: Query Real-time Indices
The system MUST provide a CLI command to query and display real-time data for a predefined list of major stock indices, exchange rates, and commodities.

#### Scenario: User queries all default market data
- **WHEN** the user executes the `fund market` command without arguments
- **THEN** the system fetches real-time data for all default indices, forex pairs, and commodities from the Morningstar API
- **AND** displays the data grouped by category (e.g., 中国股市, 全球股市, 外汇与汇率, 大宗商品)
- **AND** the table includes columns: 名称, 当前点位, 涨跌, 涨跌幅 (%)
- **AND** positive changes are formatted in red, and negative changes are formatted in green
- **AND** the table indicates the last update time and data source

#### Scenario: User queries specific market items
- **WHEN** the user executes the `fund market` command with specific names (e.g., `沪深300`, `美元/人民币`, `黄金`)
- **THEN** the system ONLY fetches and displays the requested items across all supported categories
- **AND** if an unrecognized name is provided, the system emits a clear warning message

#### Scenario: User filters by category
- **WHEN** the user executes `fund market --fx` or `fund market --com` or `fund market --index`
- **THEN** the system ONLY displays items belonging to the requested category (Forex, Commodities, or Equity Indices)

#### Scenario: Data provider API is unreachable
- **WHEN** the user executes the `fund market` command
- **AND** the Morningstar API is unreachable or returns an error
- **THEN** the system displays a clear error message and exits with a non-zero status code

#### Scenario: Batch querying for performance
- **WHEN** the system fetches data for multiple market items
- **THEN** it MUST fetch the data efficiently (e.g., via a single batch API request or concurrent requests) to minimize network latency
