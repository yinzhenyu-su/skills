## Why

main 分支的 fund-manager 构建流程触发过于频繁——任何 push 都会触发构建，包括只改文档（README、CLAUDE.md、SKILL.md）或 VERSION 文件等与二进制产物无关的变更。这既浪费 CI 资源，也增加了不必要的构建延迟。

## What Changes

- 修改 `.github/workflows/release.yml` 的 `on.push` 和 `on.pull_request` 触发条件，添加 `paths` 过滤
- push/pull_request on main 分支：只在 `skills/fund-manager/src/**` 或 `skills/fund-manager/Cargo.*` 变更时触发构建
- tag push (`refs/tags/v*`）：保持不变，无 path 过滤，确保发布时全量验证

### 过滤路径详情

| 路径模式 | 触发构建 | 说明 |
|----------|----------|------|
| `skills/fund-manager/src/**` | ✅ | 源代码变更 |
| `skills/fund-manager/Cargo.toml` | ✅ | 依赖声明变更 |
| `skills/fund-manager/Cargo.lock` | ✅ | 依赖锁文件变更 |
| `skills/fund-manager/SKILL.md` | ❌ | 文档变更 |
| `skills/fund-manager/CLAUDE.md` | ❌ | 文档变更 |
| `skills/fund-manager/.skill.toml` | ❌ | Skill 元数据 |
| `skills/VERSION` | ❌ | 版本号，仅用于 artifact 命名 |
| 其他文件 | ❌ | 不影响构建 |

## Capabilities

### New Capabilities
<!-- 这是一个纯 CI/CD 变更，不引入新的 fund-manager 能力 -->

### Modified Capabilities
<!-- 无 spec 级别的需求变更 -->

## Impact

- `.github/workflows/release.yml` — 修改触发条件配置
