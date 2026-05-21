## 1. `qrypt mount` 嵌入 daemon 启动序列

- [x] 1.1 将 `cmd/qryptd/main.go` 的 daemon 启动逻辑（Daemon init → WS Server start → signal handling → graceful shutdown）移入 `cmd/qrypt/mount.go` 的 `runMount()` 函数
- [x] 1.2 `qrypt mount` 检测 socket 不存在时，直接在当前进程启动 daemon，不再报错退出
- [x] 1.3 新增 `--daemon` flag：headless 模式，跳过 FUSE mount 只启动 WS server + 服务端组件
- [x] 1.4 socket 文件生命周期：启动时写入 `~/.qrypt/qryptd.sock`，停止时删除
- [x] 1.5 信号处理：SIGINT/SIGTERM → DaemonShutdown → WS Stop → exit
- [x] 1.6 编译验证：`go build ./cmd/qrypt`

## 2. `qryptd` 变薄为包装

- [x] 2.1 `cmd/qryptd/main.go` 精简为 `exec.Command("qrypt", "mount", "--daemon")`，透传 `--config` / `--log-level` 参数
- [x] 2.2 添加 deprecation 提示："qryptd is deprecated, use 'qrypt mount --daemon' instead"
- [x] 2.3 编译验证：`go build ./cmd/qryptd`

## 3. CLI 命令 auto-discovery

- [x] 3.1 在 `cmd/qrypt/util.go` 新增 `ensureDaemon()` 函数：检查 socket → 不存在则自动启动 headless daemon → 等待 socket 出现 → 返回 WSClient
- [x] 3.2 `startDaemonHeadless()`：`exec.Command("qrypt", "mount", "--daemon")` 后台启动，等待 socket 就绪（超时 500ms）
- [x] 3.3 迁移 ls.go：移除 `IsDaemonRunning` 检查 + 错误提示，改为调用 `ensureDaemon()`
- [x] 3.4 迁移 cat.go：同上
- [x] 3.5 迁移 push.go：同上
- [x] 3.6 迁移 pull.go：同上
- [x] 3.7 迁移 rm.go：同上
- [x] 3.8 迁移 mv.go：同上
- [x] 3.9 迁移 mkdir.go：同上
- [x] 3.10 迁移 mount_admin.go：移除 `IsDaemonRunning` 检查，改为调用 `ensureDaemon()`
- [x] 3.11 编译验证：`go build ./cmd/qrypt`、`go vet ./...`

## 4. Headless daemon 生命周期管理

- [x] 4.1 headless daemon idle timeout：WS 连接数为 0 时启动 30 秒计时器，超时后自动 shutdown
- [x] 4.2 有活跃 mount 时（非 headless 模式）idle timeout 不生效（headless bool 控制）
- [x] 4.3 新增 `--stop-daemon` flag 用于手动关闭 running daemon（发送 shutdown RPC）
- [x] 4.4 编译验证：`go build ./cmd/qrypt`

## 5. 清理与回归

- [x] 5.1 `cmd/qryptd/main.go` 已精简为包装器
- [x] 5.2 所有现有单元测试通过：`go test ./...` ✓ (18 个包全部通过)
- [ ] 5.3 手动验证：`qrypt mount`（FUSE + daemon 同一进程）
- [ ] 5.4 手动验证：`qrypt push`（daemon 不存在时自动启动）
- [ ] 5.5 手动验证：`qryptd`（向后兼容）
- [x] 5.6 更新 `cmd/qrypt/status.go` 中的 daemon 检测逻辑，移除旧的 `checkQryptProcess`
