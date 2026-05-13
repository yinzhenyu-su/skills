## 1. Modify Release Workflow

- [x] 1.1 在 `.github/workflows/release.yml` 的 `on.push` 中为 main 分支添加 `paths` 过滤：`skills/fund-manager/src/**`、`skills/fund-manager/Cargo.toml`、`skills/fund-manager/Cargo.lock`
- [x] 1.2 在 `.github/workflows/release.yml` 的 `on.pull_request` 中为 main 分支添加相同的 `paths` 过滤
- [x] 1.3 确认 tag push (`refs/tags/v*`) 保持无 path 过滤
- [x] 1.4 提交变更并验证 workflow 语法正确
