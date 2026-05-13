## Why

当前项目有多类 skills：
- **二进制型**: fund-manager（Rust CLI，需要多平台交叉编译）
- **通用型**: trending、joke-learner、xiaomi-tts（纯文档或脚本，跨平台通用）

之前的发布流程只针对 fund-manager 设计，但未来扩展需要统一的发版流程。核心洞察：**只有需要按平台分发不同二进制文件的 skill 才需要 bootstrap 脚本，通用型 skill 直接 Raw URL 引用即可。**

## What Changes

### Skill 类型分类

| 类型 | 示例 | 发布方式 | 分发方式 |
|------|------|----------|----------|
| **二进制型** (binary) | fund-manager | git tag → CI 构建 → Release | bootstrap 下载 |
| **通用型** (generic) | trending, joke-learner | git push 即生效 | Raw URL 直接引用 |

### New Files

| 文件 | 用途 |
|------|------|
| `skills/VERSION` | 统一版本号文件 |
| `skills/CHANGELOG.md` | 统一变更日志 |
| `skills/scripts/lib-skill-url.sh` | 共享 URL 模板库（二进制型用） |
| `skills/fund-manager/.skill.toml` | 二进制型 skill 构建声明 |

### Modified Files

| 文件 | 变更 |
|------|------|
| `.github/workflows/release.yml` | 扩展为只构建二进制型 skill |
| `skills/fund-manager/SKILL.md` | 更新 bootstrap 引用方式 |

### 通用型 Skill 不需要

- `.skill.toml` - 无需构建矩阵声明
- `bootstrap.sh` - 无需按平台下载
- CI 构建 - 纯文本内容无需编译

## Capabilities

### New Capabilities

- `multi-skill-release`: 支持二进制型 skill 统一发版
- `binary-skill-build`: 通过 `.skill.toml` 声明构建目标
- `shared-url-lib`: 共享的 URL 模板库
- `generic-skill分发`: 通用型 skill 通过 Raw URL + tag 版本分发

### Impact

- 二进制型 skill 共用统一版本号，简化版本管理
- CI 只构建有 `.skill.toml` 且 `type = "binary"` 的 skill
- 通用型 skill push 即生效，无需 CI
- 新增二进制型 skill 只需添加 `.skill.toml` 和 `bootstrap.sh`
- 新增通用型 skill 只需添加 `SKILL.md`，无需任何配置

## Raw URL 格式

通用型 skill SKILL.md 引用：

```
https://raw.githubusercontent.com/yinzhenyu-su/skills/v{VERSION}/skills/{skill}/SKILL.md

示例（v0.1.0）:
https://raw.githubusercontent.com/yinzhenyu-su/skills/v0.1.0/skills/trending/SKILL.md
```
