## Why

当前 qrypt 架构经过多轮迭代（单驱动 → 多 mount → daemon RPC 路由），已经暴露出几个根本问题无法通过小修小补解决：

1. **Driver 连接无复用** — 每次 CLI 操作创建新的 Driver 实例，重新完成完整的 HTTP 认证握手
2. **两条上传路径独立** — VFS staging flush 走 uploadChan，CLI push 走 WorkerPool，互不感知，竞争带宽
3. **CLI 过于厚重** — CLI 进程独立加载 config、创建 cipher、创建 driver，可以直接绕过 daemon，该逻辑重复且分散
4. **缓存无一致性** — VFS 块缓存和 daemon 操作互不知晓，FS 没有 cache invalidation
5. **事件机制别扭** — 需要独立 subscribe_events 握手，两个连接才能同时做 RPC + 收事件

这些问题的本质是：**原架构从单进程 FUSE 工具生长出来，daemon 是后加的。** IPC 和进程边界是之后才考虑的，导致 daemon 像一个包装层而非核心枢纽。

## Scope

本变更重新设计 qrypt 的整体进程架构和 IPC 协议。覆盖范围：

| 模块 | 变化 |
|------|------|
| IPC 协议 | JSON-RPC over Unix socket → WebSocket over Unix socket（单连接双工 + 数据帧） |
| CLI | 从"全能客户端"变薄为"JSON 拼装器 + 数据中继"，所有业务逻辑集中到 daemon |
| Daemon | 新增 SessionManager（连接池化）、TransferOrchestrator（统一上传调度）、全局缓存一致性 |
| VFS | FUSE 必须运行在 daemon 进程内（不再支持独立 `qrypt mount` 进程） |
| Config | 仅 daemon 持有配置，CLI 通过 IPC 查询/修改 |

**不覆盖**：
- 跨平台协议（Windows 支持）
- Web UI / 图形化管理
- 分布式/多机场景

## Impact

| 包 | 影响 |
|----|------|
| `cmd/qrypt/` | 大幅简化，移除 config/cipher/driver 创建逻辑，70%+ 代码可删 |
| `cmd/qryptd/` | 扩展为主入口，新增 SessionManager / TransferOrchestrator 初始化 |
| `internal/daemon/` | 重构：WebSocket Server 替代 JSON-RPC Server；新增 SessionManager；新增 TransferOrchestrator |
| `internal/protocol/` | 重写为新 WebSocket 帧协议（JSON text + binary 数据帧） |
| `internal/fs/` | VFS 从持有 Driver 改为从 SessionManager borrow；上传路径改为 TransferOrchestrator.Enqueue |
| `internal/config/` | 配置加载移入 daemon，CLI 不再直接加载 |
| `internal/sync/` | WorkerPool 退役，合并入 TransferOrchestrator |
| `internal/transfer/` | 角色变化：从"一个传输命令"扩展为"全局传输调度器" |
