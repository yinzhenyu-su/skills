# Qrypt Core v2 重构完成清单

> 当前状态：v2 架构重构已完成。
> 最终架构验证：`docs/core-architecture-v2.md`

## Target Architecture

```
cmd/qrypt/       internal/daemon/    internal/mount/  mobile/
  │                   │                    │            │
  └───────────────────┴────────────────────┴────────────┘
                          │ 通过 core/qrypt 接口消费
                          ▼
                core/qrypt/ — 平台无关 kernel
          Cipher (RcloneCipher)     RcloneCipher 实现
          CacheManager              磁盘缓存 + pending nodes
          Store                     staging 文件管理
          CacheInvalidator          事件驱动的缓存失效
          SessionManager            ref-counted driver session
          EventBus / ProgressHub    事件 / 进度
          RateLimiter               限流
          Orchestrator              上传工作池
          FileAPI                   统一文件操作入口
          Driver/Writer/Uploader/Cipher/Entry 接口
                          │
                          ▼
                internal/backend/ — 具体实现
          quark/ yun139/ localfs/ mockdrive/
          driver.go (类型别名) errors.go (sentinel errors)
```

## 已完成工作

### 目录清理
| 原位置 | 去向 | 状态 |
|--------|------|------|
| `internal/cipher/` | → `core/qrypt/rclone_cipher.go` | ✅ 删除 |
| `internal/index/` | → `core/qrypt/chunk_cache.go` | ✅ 删除 |
| `internal/upload/store.go` | → `core/qrypt/staging.go` | ✅ 删除 |
| `coreadapter/` | — | ✅ 删除 |

### 接口消除
| 组件 | 处理方式 |
|------|----------|
| `CipherAdapter` | 删除 — `*RcloneCipher` 直接满足 `qrypt.Cipher` |
| `DriverAdapter` | 删除 — `backend.Driver = qrypt.Driver` 类型别名 |
| `NewSingleDriverFactory` | → `qrypt.SingleDriverFactory` 移入 core |
| `ToCoreEntry`/`ToBackendEntry` | 删除 — 同一类型不再需要转换 |
| `Noop*` 默认实现 | 已删除 |
| `NetworkConfig`/`BackgroundTask`/`AppLifecycle` | 已删除 |

### 类型统一
| 类型 | 现状 |
|------|------|
| `Entry` | `qrypt.Entry` 是唯一 Entry 类型（+ParentID/+Extra） |
| `backend.Driver` | 类型别名 `= qrypt.Driver` |
| `backend.Writer` | 类型别名 `= qrypt.Writer` |
| `backend.Uploader` | 类型别名 `= qrypt.Uploader` |

## 验收检查

- [x] `go build ./...` 通过
- [x] `grep -r "yinzhenyu/skills/qrypt/internal" core/qrypt/` 无输出
- [x] `grep -r "Noop" core/qrypt/` 无业务 Noop
- [x] `go test -race ./core/qrypt/` 通过
- [x] FUSE mount + WS RPC 端到端可用（e2e 不退化）
- [x] `internal/daemon/service.go` ≤ 500 行
- [x] `internal/daemon/` 仅 5 文件
- [ ] `gomobile bind` AAR/XCFramework 生成成功（需要 gomobile 工具链）
- [ ] FileAPI 覆盖率 100%，其他 ≥80%
