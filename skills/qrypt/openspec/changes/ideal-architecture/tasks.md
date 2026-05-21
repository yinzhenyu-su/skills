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

- [x] 3.1 TransferOrchestrator 核心实现
  - 共享 goroutine worker pool（VFS flush + CLI push 同一个队列）
  - 实现 fs.UploadQueue 接口，VFS 通过闭包提交上传任务
  - 共享 TokenBucket + ProgressHub
  - daemon 管理时自动绑定 VFS → orchestrator
  - 独立模式仍走 uploadChan（向后兼容）
- [x] 3.2 VFS staging flush 由 uploadChan → orchestrator（daemon 管理时）
- [x] 3.3 Daemon pushDirectory 由 WorkerPool → orchestrator
  - Walk directory + mkdir remote dirs inline
  - Submit per-file upload closures to shared Orchestrator
  - Progress tracking via daemon's ProgressHub + events
  - CLI standalone path continues to use sync.WorkerPool (unchanged)
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
- [x] 4.1 CacheInvalidator 实现
  - 新增 QryptFS.InvalidateDirCache（节点树缓存驱逐）
  - 新增 daemon.CacheInvalidator（订阅 EventSyncCompleted 事件，转发驱逐）
  - mountBackend 接口新增 VFS() 方法暴露 CacheInvalidatable
  - MountManager 新增 ForEachRunningMount 安全遍历
- [x] 4.2 `qrypt mount` 改为 daemon 别名
  - daemon 运行时: CLI 通过 WS RPC 委托 daemon 启停 FUSE
  - daemon 未运行: 降级到独立挂载模式（带 deprecation 警告）
  - `mount list` 优先显示 daemon 实时状态（daemon 运行时）
  - `mount start/stop` 走 daemon RPC
- [x] 4.3 cat → daemon（find/cp 待完成）
  - 新增 `cat_file` RPC method: daemon 读取+解密后通过 WebSocket binary frames 流式推送
  - CLI `qrypt cat` 优先走 daemon，fallback 到 direct
  - WSClient 新增 Conn() / Ctx() 方法支持 binary streaming
- [x] 4.4 回归测试：17 个包编译通过、vet 通过、全部单元测试通过
