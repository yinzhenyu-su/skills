## Why

当前 `skills/VERSION` 和 `skills/CHANGELOG.md` 两个文件是之前设计 CI 发版流程时引入的，但实际实现后发现：

- **VERSION**: CI 不依赖它，只读 git tag
- **CHANGELOG.md**: GitHub Release Notes 自动生成，内容重复

这两个文件增加维护负担但没有实际价值，需要简化。

## What Changes

- 删除 `skills/VERSION` 文件
- 删除 `skills/CHANGELOG.md` 文件
- CI 保持不变，纯靠 git tag 驱动版本

## Capabilities

### New Capabilities

（无新能力引入）

### Modified Capabilities

（无现有能力变更）

## Impact

- **删除文件**: `skills/VERSION`, `skills/CHANGELOG.md`
- **CI**: `.github/workflows/release.yml` 不受影响，已依赖 git tag
- **Bootstrap**: `skills/fund-manager/scripts/bootstrap.sh` 不受影响
