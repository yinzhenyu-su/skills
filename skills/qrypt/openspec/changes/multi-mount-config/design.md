## Context

qrypt 当前架构为单一驱动模型：配置文件中 `[drive]` 选择一种后端类型，工厂 create 一个 Driver 实例，QryptFS 绑定一个 driver/cipher/rootFid。用户要挂载多个网盘时只能启动多个独立进程，无统一管理入口，也无法实现跨盘数据流。

已有 `cmd/qryptd/` 和 `internal/daemon/` 守护进程骨架，含 Unix socket JSON-RPC server，但当前 Daemon 仍是单实例设计。

## Goals / Non-Goals

**Goals:**
- 一套 TOML 配置文件同时声明多个网盘挂载实例
- 旧格式 `[drive]` + `[mount]` 自动迁移，零配置破坏
- 每个 mount 拥有独立的 cipher、cache dir、driver 实例、FUSE 挂载点
- `qryptd` 守护进程统一管理所有挂载的生命周期
- CLI 工具通过 `mount_name:path` 语法选定目标网盘
- 跨 mount 文件流式传输（`qrypt cp src_mount:path dst_mount:path`）

**Non-Goals:**
- 单 FUSE 挂载点下的多命名空间聚合（方案 B — 留待未来）
- driver 接口自身改动（`drive.Driver` 无需修改）
- 热插拔/运行时增删 mount
- 图形化管理界面

## Decisions

### Decision: `[[mounts]]` 数组取代单 `[drive]` 选择

**方案**: 配置从 `[drive] { type, quark, yun139, localfs }` 改为 `[[mounts]] { name, type, mount_point, params, encryption, ... }`。全局默认值抽到 `[defaults]`。

**理由**:
- TOML 原生支持 `[[array]]`，不需要自创列表语法
- 每个 mount 字段平铺，避免嵌套过深
- `[defaults]` 提供自然的多级覆盖：`mount.encryption → defaults.encryption`

**替代方案**: 扁平 key-value 命名（`mount.personal.type`, `mount.personal.cookie`）— 需要手写 parse，不如 TOML array 自然。

### Decision: 单进程多挂载（qryptd 管理所有 mounts）

**方案**: 一个 `qryptd` 进程持有所有 FUSE 挂载，Unix socket 对外提供统一管理接口。

**理由**:
- 跨盘传输可以实现进程内流式管道，无需经过本地文件系统
- 共享日志、资源监控、事件流
- 已有的 `cmd/qryptd/` 骨架可以直接扩展

**替代方案**: 多进程（每个 mount 一个 `qrypt mount`）— 隔离性更好，但跨盘传输必须绕道本地磁盘，管理多个进程也需要额外的 IPC。

### Decision: MountManager 作为核心编排器

```go
type MountManager struct {
    mounts map[string]*MountInstance
    cfg    *config.Config
}

type MountInstance struct {
    Name    string
    State   protocol.MountState
    FS      *fs.QryptFS
    Cipher  *crypt.RcloneCipher
    Driver  drive.Driver
    Cache   *cache.CacheManager
    Mount   mountBackend
    ResolvedCfg *config.ResolvedMountConfig
    StartedAt   time.Time
    LastError   string
}
```

现有 `Daemon` 重构为调用 `MountManager` 的薄封装，保持 `Service` 接口不变（方法加 `mount` 参数）。

### Decision: `mount_name:path` 前缀语法

规则：
- `[a-z0-9-]{1,32}:/` 前缀 → 解析为 mount name + path
- `.`、`/`、`~` 打头的路径 → 当作纯 path
- 其余 → 当作纯 path

避免 Windows 盘符 `C:` 误匹配（大写字母排除）。

### Decision: 跨盘传输走 driver-to-driver 流式管道

```
src.Driver.Read → src.Cipher.Decrypt → dst.Cipher.Encrypt → dst.Driver.Put
```

不经过 FUSE 层，直接操作 Driver 接口。全程流式，不断写入磁盘。

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| 多个 FUSE 进程共用同一 qryptd 进程，一个崩溃影响所有 | `MountInstance` 之间完全隔离（独立 cipher/cache/driver），某个 mount 的 driver panic 不影响其他 |
| 配置文件中多个 cookie 同时存在，安全风险增加 | 配置文件权限建议 `0600`；每个 mount 可独立设 encryption |
| `mount_name:path` 语法与未来需求冲突（如 Windows 绝对路径） | 解析规则可扩展；目前 macOS/Linux only，无冲突 |
| 单 qryptd 的 cache dir 占用变大（每个 mount 独立 `~/.qrypt/cache/<name>/`） | 用户可通过 `mounts[].cache.max_size` 限制 |
| 跨盘传输性能受限于单线程流式管道 | 大文件可通过 multipart 并行（现有分块逻辑可复用） |

## Migration Plan

1. config 层：新数据结构和 LoadConfig 迁移逻辑 → 单元测试
2. MountManager：生命周期管理 → 单元测试
3. qryptd：改造为调用 MountManager → 集成测试
4. CLI：mount 子命令 + mount_name:path 解析 → 测试
5. 跨盘传输：TransferManager + 管道 → 单元测试
6. 旧格式配置在 qrypt init 默认生成新格式 → 确保读写无回归

## Open Questions

- `cp` 命令是否需要进度报告？当前的单文件操作没有进度，跨盘传输可能涉及大文件
- qryptd 是否需要在启动失败时降级（部分 mount 失败，其余继续）？当前设计方向是：失败记录到 LastError，其他 mount 正常启动
