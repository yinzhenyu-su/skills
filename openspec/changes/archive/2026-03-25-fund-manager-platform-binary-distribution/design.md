## Context

`fund-manager` 是 Rust CLI 项目，当前 skill 文档以源码构建为主，不适合希望“即装即用”的场景。目标是将 skill 调用入口改为包装脚本：首次运行时自动探测平台并下载对应二进制，后续走本地缓存执行。

CI 侧需要稳定输出三平台产物并保证命名规范，从而让下载脚本不依赖临时路径或人工判断。

## Goals / Non-Goals

**Goals:**

- 提供统一入口脚本，在 Linux/macOS/Windows 上选择正确目标产物。
- 为 `fund-manager` 提供可缓存、可版本锁定、可覆盖下载地址的启动机制。
- 建立 GitLab CI 三平台构建与产物发布流程。
- 定义稳定的产物命名规范，降低脚本解析复杂度。

**Non-Goals:**

- 不改变 `fund-manager` 业务命令、数据库结构与金融计算逻辑。
- 不在本次引入交叉编译替代原生 runner 的复杂方案（例如一步到位支持所有架构）。
- 不在本次实现自动增量更新或后台守护升级。

## Decisions

1. 使用“包装脚本 + 核心二进制”模式

- Decision: `skills/fund-manager/scripts/fund-manager.sh` 作为统一入口，内部调用 `bootstrap-fund-manager.sh` 完成下载与缓存。
- Rationale: 让 skill 对使用者保持单入口，降低首次使用成本。
- Alternatives considered:
  - 继续要求本地 `cargo build`: 依赖重且体验不一致。
  - 在 SKILL.md 手工分平台安装: 用户步骤多、出错率高。

1. 平台识别以目标 triple 为中心

- Decision: 通过 `uname -s` 与 `uname -m` 映射到产物 triple（如 `x86_64-unknown-linux-gnu`、`aarch64-apple-darwin`、`x86_64-pc-windows-msvc`）。
- Rationale: 与 Rust 编译目标和 CI 产物命名一致，脚本逻辑可预测。
- Alternatives considered:
  - 仅用 OS 粗粒度分类（linux/macos/windows）: 无法区分架构。

1. 下载源优先稳定发布端点

- Decision: 默认使用 GitLab 发布端点（Package Registry 或 Release assets），并保留 URL 覆盖变量。
- Rationale: CI 临时 artifacts 可能过期，不适合作为长期安装源。
- Alternatives considered:
  - 直接依赖 job artifacts 下载 API: 链接稳定性与可追溯性较差。

1. GitLab CI 使用三平台原生 runner

- Decision: 通过 `linux`、`macos`、`windows` 三类 job 构建发布包，统一命名后上传。
- Rationale: 避免复杂交叉编译差异，提高构建一致性。
- Alternatives considered:
  - 单 Linux runner 交叉编译全部平台: 对工具链与链接器要求高，调试成本大。

1. 版本策略支持 latest 与 pin

- Decision: 支持 `FUND_MANAGER_VERSION` 指定版本；未指定时使用 latest/stable。
- Rationale: 兼顾快速体验与可复现部署。
- Alternatives considered:
  - 强制 latest: 容易引入不可预期升级风险。

## Risks / Trade-offs

- [Risk] GitLab 缺少 macOS/Windows runner，导致三平台目标无法同时产出。
  - Mitigation: 在提案执行前验收 runner 可用性；先以 Linux 打通流程并标记平台缺口。

- [Risk] 直接使用 artifacts 下载导致过期失效。
  - Mitigation: 发布到长期可访问端点（Package Registry/Release），并在脚本中提供回退 URL。

- [Risk] 平台/架构识别遗漏造成下载错误。
  - Mitigation: 明确支持矩阵并在不支持平台上给出可操作错误提示。

- [Risk] 私有仓库令牌泄露。
  - Mitigation: 仅从环境变量读取 token，不写入仓库，不在日志打印敏感信息。

- [Risk] 本地缓存损坏导致运行失败。
  - Mitigation: 引入哈希校验，失败时自动重新下载。

## Migration Plan

1. 增加脚本与 skill 文档，不改变现有 Rust 代码路径。
2. 增加 GitLab CI 三平台构建 job，并输出统一命名产物。
3. 在测试仓库/分支验证下载链路与权限策略。
4. 发布首个可下载版本，验证不同 OS 首次拉取与复用缓存行为。
5. 若出现下载异常，设置 `FUND_MANAGER_CORE_URL` 指向回退产物并快速恢复。

## Open Questions

- 是否立即支持 `aarch64-unknown-linux-gnu`，还是先覆盖主流 `x86_64`。
- GitLab 下载源最终选 Package Registry 还是 Release assets。
- 版本解析规则是否需要支持语义版本前缀（如 `v1.2.3` 与 `1.2.3` 兼容）。
