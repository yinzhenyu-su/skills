<!--
  这是一个纯 CI/CD 变更，不涉及 fund-manager 二进制能力的改变。
  现有的 fund-manager specs 均不受影响，此处仅记录 CI 触发路径的规范。
-->

## ADDED Requirements

### Requirement: CI build shall trigger only on relevant source changes

对于 `push` 和 `pull_request` 事件（target branches: main），CI 构建流程仅在以下路径发生变更时触发：

#### Scenario: Source code change triggers build
- **WHEN** a push or pull_request modifies any file under `skills/fund-manager/src/`
- **THEN** the Release workflow SHALL trigger

#### Scenario: Cargo dependency change triggers build
- **WHEN** a push or pull_request modifies `skills/fund-manager/Cargo.toml` or `skills/fund-manager/Cargo.lock`
- **THEN** the Release workflow SHALL trigger

#### Scenario: Documentation change does not trigger build
- **WHEN** a push or pull_request modifies `skills/fund-manager/SKILL.md` or `skills/fund-manager/CLAUDE.md`
- **THEN** the Release workflow SHALL NOT trigger

#### Scenario: Version file change does not trigger build
- **WHEN** a push or pull_request modifies `skills/VERSION`
- **THEN** the Release workflow SHALL NOT trigger

### Requirement: Tag push shall always trigger full build

#### Scenario: Tag push triggers build regardless of paths
- **WHEN** a tag matching `refs/tags/v*` is pushed
- **THEN** the Release workflow SHALL trigger regardless of changed paths
