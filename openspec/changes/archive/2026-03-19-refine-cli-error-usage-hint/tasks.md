# Tasks: Refine CLI Error Usage Hints

## Implementation

- [x] **Define `Suggestion` Struct**: Add the `Suggestion` struct to `resolver.rs` (or define it within `AdviceEngine`).
- [x] **Refactor `AdviceEngine::check_unexpected_arg_intent`**:
    - [x] Update return type to `Option<Suggestion>`.
    - [x] Populate `path` for misplaced subcommands.
    - [x] Populate `path` for fuzzy match suggestions where possible.
- [x] **Implement `AdviceEngine::find_command_by_path`**: Add a private helper method to look up a `clap::Command` by its name path.
- [x] **Implement `AdviceEngine::render_custom_usage`**: Add a private helper method to render a command's usage and apply the `用法示例：` formatting.
- [x] **Update `AdviceEngine::format_clap_error`**:
    - [x] Integrated the new suggestion logic.
    - [x] Use `render_custom_usage` if a suggestion path exists.
    - [x] Update the default usage formatting to use `用法示例：` (full-width colon).

## Verification

- [x] **Manual Tests**:
    - [x] `fund use` should show usage for `fund wallet use`.
    - [x] `fund sync` should show usage for `fund wallet sync`.
    - [x] `fund walet list` should show usage for `fund wallet list`.
- [x] **Automated Tests**:
    - [x] Update `test_advice_engine_check_param_swap_format` if necessary.
    - [x] Add new unit tests for `format_clap_error` with misplaced subcommands.
- [x] **Audit Punctuation**: Verify that all `用法示例` lines now use the `：` character.
