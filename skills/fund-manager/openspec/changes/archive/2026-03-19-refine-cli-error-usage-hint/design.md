# Design: Refine CLI Error Usage Hints

## Overview
The refinement aims to synchronize the "Hint" provided by the `AdviceEngine` with the "Usage Example" shown in CLI error messages. If a suggestion for a correct subcommand path is made, the usage example should show the usage for *that* suggested command.

## Architecture & Implementation Details

### 1. Structure for Advice
Introduce a struct or tuple to hold both the hint string and the suggested command path (as a `Vec<String>`). This allows `format_clap_error` to act on the path.

```rust
struct Suggestion {
    hint: String,
    path: Option<Vec<String>>,
}
```

### 2. Refactor `AdviceEngine`
Modify `AdviceEngine::check_unexpected_arg_intent` to return an `Option<Suggestion>` instead of `Option<String>`.

- **Subcommand Path Lookup**: When `find_subcommand_path` succeeds, populate the `path` field in `Suggestion`.
- **Fuzzy Match Suggestion**: When `suggest_command_spelling` succeeds, also try to return the path if possible, or just the hint if it's at the same level.

### 3. Usage Extraction and Formatting
Implement a utility function to extract a `clap::Command` object from a given string path (e.g., `["fund", "wallet", "use"]`).

```rust
fn find_command_by_path<'a>(root: &'a Command, path: &[String]) -> Option<&'a Command>
```

Add a method to format the usage for a specific `Command` in the stylized format used by the application:
- Replace `Usage: ` with `   用法示例：`.
- Replace `fund-manager` with `fund`.
- Ensure the full-width colon `：` is used consistently.

### 4. Integration in `format_clap_error`
In `AdviceEngine::format_clap_error`:
- Store the result of `check_unexpected_arg_intent`.
- If a suggested path exists:
    1. Look up the `Command` for that path starting from `Cli::command()`.
    2. If found, use its `render_usage()` output to construct the usage string.
    3. Override the default `usage` from `clap::Error`.
- Ensure all usage examples use `   用法示例：` with the full-width colon.

## Examples

### Case: Misplaced Subcommand
Input: `fund use NAME`
Current:
```
❌ 未识别的参数或子命令 'use'
💡 Hint: 你是不是想找：'fund wallet use'？
   用法示例: fund [COMMAND]
```
Proposed:
```
❌ 未识别 the parameter or subcommand 'use'
💡 Hint: 你是不是想找：'fund wallet use'？
   用法示例：fund wallet use <NAME>
```

### Case: Fuzzy Match
Input: `fund walet list`
Current:
```
❌ 未识别 the parameter or subcommand 'walet'
❓ 未识别的子命令 'walet'。你是不是想找：'wallet'？
   用法示例: fund [COMMAND]
```
Proposed:
```
❌ 未识别 the parameter or subcommand 'walet'
❓ 未识别的子命令 'walet'。你是不是想找：'wallet'？
   用法示例：fund wallet [COMMAND]
```
