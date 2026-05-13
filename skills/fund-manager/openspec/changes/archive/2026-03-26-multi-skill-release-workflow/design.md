## 1. 目录结构

```
skills/
├── VERSION                      # 统一版本号 (v1.0.0)
├── CHANGELOG.md                 # 统一变更日志
├── scripts/
│   └── lib-skill-url.sh        # 共享 URL 模板库（二进制型用）
│
├── fund-manager/               # 二进制型 skill
│   ├── SKILL.md                # Skill 定义
│   ├── .skill.toml             # 构建声明 (type = "binary")
│   ├── scripts/
│   │   └── bootstrap.sh       # Bootstrap 脚本
│   └── src/
│
├── trending/                   # 通用型 skill
│   └── SKILL.md                # 直接引用，无需其他配置
│
├── joke-learner/               # 通用型 skill
│   └── SKILL.md
│
└── xiaomi-tts/                 # 通用型 skill
    └── SKILL.md

.github/workflows/
└── release.yml                 # CI/CD（二进制型 skill 构建）
```

## 2. VERSION 文件格式

```
v1.0.0
```

纯文本文件，只包含版本号（带 v 前缀）。所有 skills 共用此版本。

## 3. .skill.toml Schema

`.skill.toml` 只为**二进制型** skill 创建。通用型 skill 不需要此文件。

```toml
type = "binary"                  # 必需：声明为二进制型
name = "fund-manager"            # 必需：Skill 名称
description = "中国公募基金投资管理 CLI 工具"  # 必需：简短描述
license = "MIT"                  # 必需：许可证

[build]                          # 构建配置
default-targets = [              # 目标平台
    "x86_64-unknown-linux-gnu",
    "aarch64-unknown-linux-gnu",
    "x86_64-apple-darwin",
    "aarch64-apple-darwin",
    "x86_64-pc-windows-gnu",
    "aarch64-pc-windows-gnullvm",
]
build-command = "cargo build --release"

[output]                         # 输出配置
binary-name = "fund-manager"
```

### 字段说明

| 字段 | 必需 | 说明 |
|------|------|------|
| `type` | 是 | 固定值 `"binary"` |
| `name` | 是 | Skill 目录名 |
| `description` | 是 | 简短描述 |
| `license` | 是 | 许可证 |
| `build.default-targets` | 否 | 构建目标，默认全部 6 平台 |
| `build.build-command` | 否 | 构建命令，默认 `cargo build --release` |
| `output.binary-name` | 否 | 二进制名，默认目录名 |

## 4. lib-skill-url.sh 共享库

```bash
#!/bin/bash
# 共享 URL 模板库（二进制型 skill bootstrap 使用）

SKILL_URL_BASE="${SKILL_URL_BASE:-https://github.com}"

# 解析 target triple 到人类可读格式
parse_target() {
    local target="$1"
    case "$target" in
        x86_64-unknown-linux-gnu)  echo "linux-x64" ;;
        aarch64-unknown-linux-gnu) echo "linux-arm64" ;;
        x86_64-apple-darwin)       echo "macos-x64" ;;
        aarch64-apple-darwin)      echo "macos-arm64" ;;
        x86_64-pc-windows-gnu)    echo "windows-x64" ;;
        aarch64-pc-windows-gnullvm) echo "windows-arm64" ;;
        *)                          echo "$target" ;;
    esac
}

# 获取归档扩展名
get_archive_ext() {
    local target="$1"
    case "$target" in
        *windows*) echo "zip" ;;
        *)         echo "tar.gz" ;;
    esac
}

# 构建下载 URL
build_download_url() {
    local repo="$1"
    local skill_name="$2"
    local version="$3"
    local target="$4"

    local short_target=$(parse_target "$target")
    local ext=$(get_archive_ext "$target")
    local ver="${version#v}"

    echo "${SKILL_URL_BASE}/${repo}/releases/download/v${ver}/${skill_name}/${skill_name}-v${ver}-${short_target}.${ext}"
}
```

## 5. CI/CD 流程

```
git tag v1.0.0
       │
       ▼
GitHub Actions (release.yml)
       │
       ├─ 读取 skills/VERSION
       ├─ 扫描 skills/*/.skill.toml
       ├─ 仅处理 type = "binary" 的 skill
       │
       ▼
   ┌───────────────────────────────────────┐
   │  二进制型 skill 构建                    │
   │  (fund-manager)                       │
   │  - 6 平台交叉编译                      │
   │  - 打包成 tar.gz/.zip                  │
   └───────────────────────────────────────┘
       │
       ▼
   ┌───────────────────────────────────────┐
   │  GitHub Release /v1.0.0/              │
   │  fund-manager-v1.0.0-linux-x64.tar.gz │
   │  fund-manager-v1.0.0-windows-x64.zip │
   │  ...                                   │
   └───────────────────────────────────────┘
```

### CI 判断逻辑

```yaml
# 伪代码
for skill_dir in skills/*/:
    skill_toml = skill_dir/.skill.toml
    if skill_toml exists:
        type = read(skill_toml).type
        if type == "binary":
            # 执行构建并发布
        else:
            # 跳过（通用型 skill 无需 CI）
    else:
        # 无 .skill.toml = 通用型 skill，跳过
```

## 6. 发布流程

### 二进制型 skill

1. 更新 `skills/VERSION` 文件
2. 更新 `skills/CHANGELOG.md`
3. `git add . && git commit -m "release: v1.0.0"`
4. `git tag v1.0.0 && git push --tags`
5. CI 自动构建并上传到 GitHub Release
6. 用户通过 bootstrap 脚本下载

### 通用型 skill

1. 更新 `skills/{skill}/SKILL.md`
2. `git add . && git commit -m "docs: update {skill}"`
3. `git push`
4. **无需 tag 或 CI** - Raw URL 引用自动指向新内容

## 7. 通用型 skill Raw URL 格式

```
https://raw.githubusercontent.com/yinzhenyu-su/skills/v{VERSION}/skills/{skill}/SKILL.md
```

### 示例

| Skill | Raw URL |
|-------|---------|
| trending v0.1.0 | `https://raw.githubusercontent.com/yinzhenyu-su/skills/v0.1.0/skills/trending/SKILL.md` |
| joke-learner v0.1.0 | `https://raw.githubusercontent.com/yinzhenyu-su/skills/v0.1.0/skills/joke-learner/SKILL.md` |
| xiaomi-tts v0.1.0 | `https://raw.githubusercontent.com/yinzhenyu-su/skills/v0.1.0/skills/xiaomi-tts/SKILL.md` |

### 版本更新时机

通用型 skill 在 `git push` 后立即生效，但建议配合 tag 记录版本点：

```bash
# 通用型 skill 更新后
git tag v0.2.0
git push --tags
```

这样 Raw URL `v0.2.0` 指向新版本，`v0.1.0` 保留旧版本。

## 8. GitHub Release 产物命名（二进制型）

```
{skill-name}-v{version}-{os}-{arch}.{ext}

Examples:
fund-manager-v1.0.0-linux-x64.tar.gz
fund-manager-v1.0.0-windows-x64.zip
```
