## Why

`sks/VERSION` 文件已在简化变更中被删除，但 `release.yml` 的 setup job 中仍保留读取该文件的死代码。虽然功能上不受影响（VERSION 文件不存在时走 fallback 逻辑，结果与实际构建一致），但冗余代码增加维护负担和认知复杂度。

## What Changes

- 删除 `release.yml` setup job 中的 "Read VERSION" step
- 删除 setup job 的 `version` output（未被消费）
- 更新 setup job 注释（删除 "Read version from VERSION file"）

## Capabilities

### New Capabilities

（无新能力引入）

### Modified Capabilities

（无现有能力变更）

## Impact

- `.github/workflows/release.yml`：删除死代码，精简 setup job
