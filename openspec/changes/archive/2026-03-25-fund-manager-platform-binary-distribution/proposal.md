## Why

`fund-manager` skill 当前依赖本地源码构建，用户首次使用门槛高且跨平台体验不一致。为了让 skill 开箱可用，需要将核心二进制改为按操作系统自动下载，并由 CI 稳定产出 Windows/macOS/Linux 的构建产物。

## What Changes

- 为 `fund-manager` skill 增加“按平台自动下载核心可执行文件”的引导与运行机制。
- 引入下载与缓存脚本，统一处理平台识别、版本选择、下载、解压、权限设置与执行透传。
- 新增 GitLab CI 流程，构建并发布三平台产物，形成可被 skill 稳定消费的产物命名与下载路径约定。
- 在 skill 文档中补充依赖、环境变量、版本控制和故障排查说明。

## Capabilities

### New Capabilities

- `platform-binary-bootstrap`: skill 能够根据当前操作系统与架构自动获取并缓存 `fund-manager` 核心二进制。
- `gitlab-cross-platform-build-artifacts`: CI 能够为 Linux、macOS、Windows 生成并发布统一命名的 `fund-manager` 可执行产物。

### Modified Capabilities

- None.

## Impact

- Affected docs: `skills/fund-manager/SKILL.md`.
- Affected scripts: `skills/fund-manager/scripts/` (新增下载引导与统一入口脚本)。
- Affected CI: 仓库根目录新增 `.gitlab-ci.yml` 或等效 GitLab CI 配置。
- External systems: GitLab CI runners、GitLab artifacts 或 package/release 分发端点。
- Operational impact: 需要配置 GitLab 项目标识与访问凭据（私有仓库场景）。
