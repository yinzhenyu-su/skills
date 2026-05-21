## Phase 1: IPC 层替换

- [x] 1.1 添加 `nhooyr.io/websocket` 依赖
- [x] 1.2 WebSocket Server 实现（接受连接、分帧、dispatch）
- [x] 1.3 WebSocket Client 实现（请求/响应、事件接收、binary 收发）
- [x] 1.4 JSON-RPC handler 包装为 WebSocket Text frame（兼容桥接）
- [x] 1.5 SubscribeEvents 退役，事件改由同一连接推送
- [x] 1.6 测试：WebSocket 握手、并发请求、二进制帧、事件推送

## Phase 2: SessionManager + 薄 CLI

- [x] 2.1 SessionManager + Session 核心实现
- [x] 2.2 后台 TokenRefresher goroutine（驱动自行管理 token，无需主动刷新）
- [x] 2.3 VFS 改为从 SessionManager borrow session
- [x] 2.4 `withTempMount` 退役，所有 RPC handler 改用 SessionManager
- [ ] 2.5 CLI 移除 config/cipher/driver 创建逻辑（待确认方向）
- [ ] 2.6 删除 `loadToolCfg` / `loadToolCfgOnly` / `loadToolDriverForMount`（待确认方向）
- [x] 2.7 测试：session 复用、refcount 正确性、token 刷新

## Phase 3: TransferOrchestrator

- [~] 3.1 TransferOrchestrator 核心实现（推迟至 Phase 4，VFS 归入 daemon 后自然合并）
- [~] 3.2 VFS staging flush 由 uploadChan → orchestrator（推迟至 Phase 4）
- [~] 3.3 CLI push 由 WorkerPool → orchestrator（推迟至 Phase 4）
- [x] 3.4 TokenBucket 全局限速
- [x] 3.5 ProgressHub 统一进度推送
- [~] 3.6 WorkerPool 退役（推迟至 Phase 4）
- [x] 3.7 测试：编译通过、vet 通过、全量测试通过

## Phase 4: 缓存一致性 + FUSE 归入 daemon

- [x] 4.5 清理废弃的 JSON-RPC 和 direct 代码
  - 删除 `server.go`（旧 JSON-RPC Server，WS Server 替代）
  - 删除 `client.go`（旧 JSON-RPC Client，WS Client 替代）
  - 精简 `protocol/codec.go`（仅保留 NewError/NewResult）
  - 删除 `protocol/codec_test.go` + `server_test.go`
  - 迁移 `dispatch_test.go` → 使用 `WSServer`
  - 删除根目录残留 `dispatch_test.go`
  - 迁移 `FindSocketPath`/`IsDaemonRunning` 到 `socket.go`
- [ ] 4.1 CacheInvalidator 实现
- [ ] 4.2 `qrypt mount` 改为 daemon 别名
- [ ] 4.3 cat/find/cp 改为走 daemon
- [ ] 4.4 回归测试
