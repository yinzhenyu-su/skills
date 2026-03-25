## ADDED Requirements

### Requirement: Query Real-time Indices
The system MUST provide a CLI command to query and display real-time data for a predefined list of major stock indices.

#### Scenario: User queries all default indices
- **WHEN** the user executes the `index` command without arguments
- **THEN** the system fetches real-time data for all major domestic, HK, and US indices
- **AND** displays the data grouped by market in a table format containing columns: Index Name, Current Value, Change, and Change %
- **AND** positive changes are formatted in red, and negative changes are formatted in green
- **AND** the table indicates the last update time of the data
- **AND** if an index cannot be found in the API response, it gracefully ignores it instead of failing

#### Scenario: User queries specific indices
- **WHEN** the user executes the `index` command with one or more specific names (e.g., `fund-manager index 沪深300 纳斯达克`)
- **THEN** the system ONLY fetches and displays the requested indices
- **AND** if an unrecognized index name is provided, the system MUST emit a clear warning message that lists the available/supported index names

#### Scenario: Data provider API is unreachable
- **WHEN** the user executes the `index` command
- **AND** the external data provider API is unreachable or returns an error
- **THEN** the system displays a clear error message indicating the failure to fetch index data
- **AND** the command exits with a non-zero status code

#### Scenario: Batch querying for performance
- **WHEN** the system fetches data for multiple indices
- **THEN** it MUST fetch the data efficiently (e.g., via a single batch API request or concurrent requests) to minimize network latency
