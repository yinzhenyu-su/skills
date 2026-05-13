# Proposal: Enhance Transaction History and Auto Settlement

## Summary
Implement a comprehensive transaction history management system, supporting multiple transaction types, rich metadata, and an automated settlement mechanism integrated with the sync process.

## Motivation
The current transaction tracking is basic and lacks visibility into historical activities. Users cannot record dividends or see a structured view of their past actions. Additionally, manual entry of shares/NAV for recent transactions is tedious; the system should handle this automatically once NAV data becomes available.

## Goals
- Support diverse transaction types: Buy, Sell, Dividend, Reinvest, Import.
- Add metadata fields: `memo` and `source`.
- Implement a professional `fund history` command with filtering.
- Automate "Pending to Settled" transition during `fund sync`.
- Ensure cost calculation (Net Cost) correctly handles dividends and imports.

## Non-Goals
- Real-time stock price tracking (remains focused on funds/NAV).
- Bank account balance synchronization.

## User Stories
- **As a user**, I want to see my transaction history in a clear table so I can verify my investments.
- **As a user**, I want to record a buy order today without knowing the final NAV, and have the system fix the shares for me tomorrow automatically.
- **As a user**, I want to record dividends to accurately reflect my true investment cost.
- **As a user**, I want to filter my history by wallet to manage different portfolios separately.
