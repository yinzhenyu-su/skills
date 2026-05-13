## 1. CLI 结构变更

- [x] 1.1 在 `src/cli.rs` 的 `Commands` 枚举中添加 `Reset` 变体
- [x] 1.2 从 `FundCommands` 枚举中删除 `Reset` 变体

## 2. 命令处理逻辑

- [x] 2.1 在 `src/main.rs` 添加 `Commands::Reset` 分支处理逻辑（从 `FundCommands::Reset` 移过来）

## 3. 帮助信息修正

- [x] 3.1 修正 `src/cli.rs` 中 Reset 的 `long_about` 示例：`fund reset` → `fund-manager reset`
- [x] 3.2 改进 Reset 的 `long_about`：列举具体删除项，强调不可恢复
- [x] 3.3 修正所有顶层命令的示例前缀：`fund status` → `fund-manager status` 等（共12处）
- [x] 3.4 更新 `src/db.rs` 注释：`Used by the 'fund reset' command` → `Used by the 'reset' command`

## 4. 测试更新

- [x] 4.1 更新 `tests/cli_tests.rs` 中的测试命令路径：`.arg("fund").arg("reset")` → `.arg("reset")`
- [x] 4.2 验证测试通过：`cargo test test_fund_reset_with_yes_flag`

## 5. OpenSpec 归档更新

- [x] 5.1 更新 `openspec/changes/archive/2026-03-26-add-fund-reset-command/proposal.md` 中的命令引用
- [x] 5.2 更新 `openspec/changes/archive/2026-03-26-add-fund-reset-command/design.md` 中的命令路径和示例
- [x] 5.3 更新 `openspec/changes/archive/2026-03-26-add-fund-reset-command/tasks.md` 中的示例
- [x] 5.4 更新 `openspec/changes/archive/2026-03-26-add-fund-reset-command/specs/fund-reset/spec.md` 中的命令路径
