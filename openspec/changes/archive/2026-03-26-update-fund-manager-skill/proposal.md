## Why

SKILL.md 文档中包含与实际 CLI 代码不符的命令示例，AI 首次调用时会因命令不存在而失败，影响 AI 对该 skill 的信任度。

## What Changes

- 修正 `fund-manager index` → `fund-manager market`（命令名变更）
- 补全缺失的 `fund-manager fund config` 命令示例（配置分红方式）
- `import-holding` 已是正确写法，无需修改

## Capabilities

### New Capabilities

- `fund-manager-skill-docs`: 更新 SKILL.md 文档，确保命令示例与实际 CLI 一致

### Modified Capabilities

（无 spec 级别变更，纯文档修复）

## Impact

- 仅修改 `skills/fund-manager/SKILL.md`
