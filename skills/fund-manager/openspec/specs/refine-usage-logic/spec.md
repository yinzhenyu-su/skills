# Spec: Refine CLI Error Usage Logic

## Scope
This spec covers the changes to `AdviceEngine` and `format_clap_error` in `resolver.rs`.

## Functional Requirements
- **Suggestion Detection**: The `AdviceEngine` MUST return the full path of any suggested command.
- **Dynamic Usage Rendering**: If a suggested command is detected, the error output MUST show the usage for *that* specific command.
- **Chinese Punctuation Standard**: Usage messages MUST use the full-width Chinese colon `：` (e.g., `   用法示例：...`).
- **Compatibility**: If no specific suggestion is made, the default usage from `clap::Error` MUST be used (but formatted with the standardized punctuation).

## Implementation Details

### Data Structures
Update `AdviceEngine` to return a `Suggestion` struct:
```rust
struct Suggestion {
    hint: String,
    path: Option<Vec<String>>,
}
```

### `AdviceEngine::check_unexpected_arg_intent`
- Returns `Option<Suggestion>`.
- For misplaced subcommand detection:
    - Calls `find_subcommand_path`.
    - Returns `Suggestion` with the hint and the path.
- For fuzzy match detection:
    - Calls `suggest_command_spelling`.
    - Returns `Suggestion` with the hint and the path (if reachable).

### `AdviceEngine::format_clap_error`
- Logic update:
    ```rust
    let suggestion = Self::check_unexpected_arg_intent(arg);
    let mut custom_usage: Option<String> = None;

    if let Some(s) = suggestion {
        output.push_str(&format!("{}\n", s.hint));
        if let Some(path) = s.path {
            if let Some(suggested_cmd) = Self::find_command_by_path(&Cli::command(), &path) {
                custom_usage = Some(Self::format_usage(suggested_cmd));
            }
        }
    }
    ```

### Formatting Logic
A helper method `format_usage(cmd: &Command)` should be used to ensure:
- `Usage: ` is replaced with `   用法示例：`.
- `fund-manager` is replaced with `fund`.
- Extra newlines are handled correctly.

## Success Criteria
- Running `fund use` should suggest `fund wallet use` and show its usage.
- Running `fund sync` (misplaced) should suggest `fund wallet sync` and show its usage.
- All "Usage Example" outputs MUST use `用法示例：`.
