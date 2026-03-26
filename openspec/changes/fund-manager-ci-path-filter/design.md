## Context

当前 `.github/workflows/release.yml` 的触发条件为：

```yaml
on:
  push:
    branches: [ main ]
    tags: [ 'v*' ]
  pull_request:
    branches: [ main ]
```

任何对 main 分支的 push 或 PR 都会触发完整的跨平台构建（Linux/Windows/macOS × 4 个 target），而不管变更是否影响 fund-manager 的二进制产物。

## Goals / Non-Goals

**Goals:**
- 减少不必要的 CI 构建触发，节省资源
- 确保源代码和依赖变更仍能触发正确的构建

**Non-Goals:**
- 不修改构建过程本身（构建命令、target、矩阵不变）
- 不改变 tag 发布行为（tag push 仍全量构建）

## Decisions

### 1. 使用 GitHub Actions `paths` 过滤

在 `on.push` 和 `on.pull_request` 中添加 `paths` 数组：

```yaml
on:
  push:
    branches: [ main ]
    paths:
      - 'skills/fund-manager/src/**'
      - 'skills/fund-manager/Cargo.toml'
      - 'skills/fund-manager/Cargo.lock'
    tags: [ 'v*' ]
  pull_request:
    branches: [ main ]
    paths:
      - 'skills/fund-manager/src/**'
      - 'skills/fund-manager/Cargo.toml'
      - 'skills/fund-manager/Cargo.lock'
```

**为什么不用 `paths-ignore`？** `paths` 是白名单模式，更安全、更明确——只允许明确列出的路径触构建，不会因为新增了其他应触发的路径而漏掉。

### 2. Tag push 不加 path 过滤

Tag push (`refs/tags/v*`) 保持无 path 过滤，确保发布流程始终全量验证。

### 3. 包含 Cargo.* 但排除 .skill.toml 等元数据

`Cargo.toml` 和 `Cargo.lock` 影响依赖解析和二进制产物，属于构建必需。
`.skill.toml`、`.gitignore`、文档文件不影响构建结果，不包含。

## Risks / Trade-offs

| 风险 |  Mitigation |
|------|-------------|
| 依赖路径写错导致漏触发 | 严格对照 Cargo 项目结构，`skills/fund-manager/` 下只有 `src/` 和 `Cargo.*` 影响构建 |
| PR 中改文档不会触发构建 | 符合预期，PR 主要验证代码变更 |
