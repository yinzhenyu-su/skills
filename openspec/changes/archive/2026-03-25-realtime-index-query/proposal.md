## Why

Users currently use `fund-manager` to track and analyze personal fund investments. However, the overall market trend is a critical context for evaluating individual fund performance. Adding the ability to query major global and domestic stock indices in real-time allows users to quickly gauge market sentiment and make more informed investment decisions without leaving the CLI tool.

## What Changes

- Add a new CLI command or subcommand (e.g., `fund-manager index` or `fund-manager market`) to query real-time market indices.
- Support a predefined list of major indices:
  - Domestic: 深圳成指 (SZSE Component), 上证指数 (SSE Composite), 创业板指 (ChiNext), 沪深 300 (CSI 300), 科创 50 (STAR 50), 上证 50 (SSE 50), 中证 500 (CSI 500).
  - Hong Kong: 恒生指数 (Hang Seng), 恒生科技 (Hang Seng TECH).
  - US: 道琼斯指数 (Dow Jones), 纳斯达克指数 (Nasdaq), 标普 500 (S&P 500).
- Integrate with a reliable data provider (e.g., Sina Finance, Tencent Finance, or Eastmoney) to fetch real-time index data (current value, absolute change, percentage change).
- Display the indices in a tabular format in the terminal with appropriate color coding (red for up, green for down, or vice versa depending on localization preferences; typically red for up in China).

## Capabilities

### New Capabilities
- `realtime-index-query`: The ability to query and display real-time data for major global and domestic stock indices.

### Modified Capabilities

## Impact

- **CLI Interface**: A new command or subcommand will be added to the clap definition.
- **Data Provider**: A new HTTP client or provider implementation will be required to fetch index data from external APIs.
- **UI**: A new table view using `comfy-table` will be added to display the index data.
