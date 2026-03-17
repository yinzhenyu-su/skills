# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a monorepo called "skills" containing AI skills and CLI tools. It uses **OpenSpec** (规范驱动开发) workflow for managing feature development.

### Core Components

1. **Fund Manager** (`skills/fund-manager/`): A Rust CLI tool for tracking and analyzing personal fund investments
2. **Trending Skill** (`skills/trending.md`): Gets trending topics from major Chinese platforms

## Building and Running

### Fund Manager

```bash
cd skills/fund-manager
cargo build          # Build the project
cargo run -- [args]  # Run with arguments
cargo test           # Run all tests
cargo test <name>    # Run a specific test
cargo fmt            # Format code
cargo clippy         # Lint code
```

### OpenSpec Workflow

Use these slash commands for the OpenSpec development process:

- `/opsx:propose` - Propose a new change with all artifacts
- `/opsx:explore` - Explore and design a change
- `/opsx:apply` - Implement tasks from a change
- `/opsx:archive` - Archive a completed change

## Architecture

### Fund Manager Structure

```
skills/fund-manager/
├── src/
│   ├── main.rs       # Entry point, command dispatch
│   ├── cli.rs        # CLI command definitions (clap)
│   ├── db.rs         # SQLite database operations
│   ├── config.rs     # Configuration management
│   ├── sync.rs       # Data synchronization logic
│   ├── resolver.rs   # Smart input resolution
│   ├── finance.rs    # Financial calculations
│   ├── provider/     # Data providers (implement traits)
│   └── db_tests.rs   # Unit tests for database
└── tests/
    └── cli_tests.rs  # Integration tests
```

### Key Patterns

- **Data Providers**: Implement `DataProvider` trait in `src/provider/` to add new fund data sources
- **Database**: SQLite via rusqlite, schema defined in `db.rs`
- **Async**: Use tokio for async operations
- **CLI**: Uses clap derive macros, interactive prompts via inquire

## Development Conventions

1. All major features must go through OpenSpec workflow (proposal → design → tasks)
2. Changes are documented in `openspec/changes/<change-name>/`
3. Use `cargo fmt` before committing
4. Write tests for new functionality in `tests/` directory
5. Provider pattern: add new data sources by implementing traits in `src/provider/`

## Important Files

- `skills/fund-manager/Cargo.toml` - Project dependencies
- `openspec/specs/` - Reusable specification components
- `.claude/skills/` - Claude Code skill definitions
- `.claude/commands/opsx/` - OpenSpec command definitions
