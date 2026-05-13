## ADDED Requirements

### Requirement: Enhanced Market Metadata Display
The system SHALL display advanced market metadata, including 52-week price range and market status (Open/Closed).

#### Scenario: User views detailed market dashboard
- **WHEN** the user executes `fund market --detail` (or `-d`)
- **THEN** the system adds a column "52周区间 [L --- H]"
- **AND** displays a progress bar representing the current price position within the 52-week high/low range
- **AND** displays the percentage position (0-100%) next to the bar

#### Scenario: Display market opening status
- **WHEN** any market index or commodity is displayed
- **THEN** the system SHALL show an indicator (🟢/🔴) based on the `status` field from the API
- **AND** "🟢" indicates the market is currently open for trading
- **AND** "🔴" indicates the market is closed

### Requirement: Intra-day Trend Visualization
The system SHALL visualize intra-day price trends using ASCII sparklines for assets providing time-series data.

#### Scenario: User views market trends
- **WHEN** the user executes `fund market --trend` (or `-t`)
- **THEN** the system adds a column "今日趋势" (Today's Trend)
- **AND** for items containing a `ts` array (e.g., Gold, Forex), it SHALL render an ASCII sparkline (e.g., `  ▃▅▇`) based on the price points
- **AND** for items without `ts` data, it SHALL display "N/A" in that column

### Requirement: User Configurable Watchlist
The system SHALL allow users to define their preferred default market items in the configuration file.

#### Scenario: User defines custom watchlist
- **WHEN** the `config.yaml` contains a `default_market_items` list (e.g., `["沪深300", "美元/人民币", "黄金"]`)
- **AND** the user runs `fund market` without arguments
- **THEN** the system ONLY shows the items specified in the configuration
