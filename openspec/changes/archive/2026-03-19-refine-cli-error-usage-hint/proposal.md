# Proposal: Refine CLI Error Usage Hints

## Problem
Currently, when a user enters a misplaced subcommand (e.g., `fund use` instead of `fund wallet use`), the `AdviceEngine` provides a hint suggesting the correct command path:
`💡 Hint: 你是不是想找：'fund wallet use'？`

However, the usage example shown immediately below it still corresponds to the command that failed to parse (the root `fund` command or the current level), which is confusing and irrelevant to the suggested fix.

Example of current behavior:
```
❌ 未识别的参数或子命令 'use'
💡 Hint: 你是不是想找：'fund wallet use'？
   用法示例: fund [COMMAND]
```

## Proposed Change
Update the error formatting logic to detect when a subcommand suggestion is made and override the default usage example with the usage instructions for the *suggested* command.

Example of proposed behavior:
```
❌ 未识别的参数或子命令 'use'
💡 Hint: 你是不是想找：'fund wallet use'？
   用法示例：fund wallet use <NAME>
```

This will also involve standardizing the use of the full-width Chinese colon `：` in usage examples to match the hints.

## Goals
- Improve the relevancy of usage examples in error messages.
- Reduce user friction when subcommands are misplaced.
- Standardize Chinese punctuation in CLI hints and usage examples.
