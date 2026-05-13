# Tasks

## 1. 核心基础设施

- [x] 1.1 创建 `skills/VERSION` 文件，初始版本 v0.1.0
- [x] 1.2 创建 `skills/CHANGELOG.md` 变更日志模板
- [x] 1.3 ~~创建 `skills/scripts/lib-skill-url.sh`~~ (已简化，不需要共享库)

## 2. fund-manager Skill 配置（二进制型）

- [x] 2.1 创建 `skills/fund-manager/.skill.toml` 构建声明
- [x] 2.2 创建 `skills/fund-manager/scripts/bootstrap.sh` 入口脚本（简化版，直接 exec binary）
- [x] 2.3 更新 `skills/fund-manager/SKILL.md` 使用新的 bootstrap 脚本

## 3. CI/CD 更新

- [x] 3.1 修改 `.github/workflows/release.yml` 添加 skill 类型判断逻辑
- [x] 3.2 验证 CI 正确跳过无 `.skill.toml` 的通用型 skill
- [x] 3.3 创建 test tag 验证 GitHub Actions Release 生成

## 4. 验证测试

- [x] 4.1 运行 `cargo build --release` 验证 fund-manager 构建
- [x] 4.2 手动测试 bootstrap 脚本下载流程
- [x] 4.3 验证通用型 skill Raw URL 引用格式
