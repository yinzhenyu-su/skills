## Context

The `fund-manager` CLI currently focuses on tracking personal fund investments. To provide users with broader market context, we need a quick way to view major global and domestic stock indices. This feature will fetch and display real-time index data directly in the terminal.

## Goals / Non-Goals

**Goals:**
- Implement a new CLI command `index` to query real-time market indices.
- Support a predefined list of major indices: SZSE Component, SSE Composite, ChiNext, CSI 300, STAR 50, SSE 50, CSI 500, Hang Seng, Hang Seng TECH, Dow Jones, Nasdaq, S&P 500.
- Fetch real-time data (current value, absolute change, percentage change).
- Display the results in a formatted, color-coded table (red for up, green for down).

**Non-Goals:**
- Historical index data, trendlines, or terminal charts.
- Individual stock queries.
- User-customizable index lists (for the initial release).

## Decisions

- **Data Provider**: We will use the **Morningstar Watch List API** (`https://www.morningstar.cn/cn-api/v2/market/watch-list`).
  - *Rationale*: Returns standard UTF-8 JSON which is much easier to parse via `serde_json` than older GBK/JS-variable APIs (like Sina). It requires no complex authentication or Referer headers, returns data in a single batch request, and aligns well with `fund-manager`'s existing reliance on Morningstar for fund data.
- **CLI Subcommand**: Add an `index` subcommand to the main CLI router.
  - *Rationale*: `fund-manager index` is semantic and aligns with existing commands.
- **Data Model**: Define an `IndexData` struct containing `name`, `current`, `change`, and `change_percent`.
  - *Rationale*: Decouples the raw API response format from the presentation layer. In this case, parsing from Morningstar's `chinaEquity` and `globalEquity` arrays.
- **UI Display**: Use the `comfy-table` crate to render the indices.
  - *Rationale*: Reuses the existing table rendering library in the project for consistent UI styling.

## Risks / Trade-offs

- **[API Instability or Rate Limiting]** → The chosen public API might change its response format or throttle requests. **Mitigation**: Implement robust error handling and parsing logic. Fail gracefully with a helpful message rather than panicking.
- **[Network Latency]** → Fetching multiple indices might be slow if done sequentially. **Mitigation**: The Morningstar watch-list API returns all major indices (China, HK, US) in a single batch JSON response, eliminating the need for concurrent network requests.
