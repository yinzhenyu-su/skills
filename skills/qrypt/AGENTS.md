# QRPT — 夸克网盘加密挂载工具

## OVERVIEW

Go FUSE 工具，将夸克网盘 (Quark Drive) 挂载为本地文件系统，兼容 rclone crypt 加密。

## STRUCTURE

```
qrypt/
├── cmd/qrypt/       # CLI 命令 (cobra): mount, init, ls, find, push, pull, mv, rm, cat, status, config, tool
├── internal/
│   ├── cache/       # 内存索引 + 磁盘 batch 文件分块缓存
│   ├── config/      # TOML 配置加载 (多路径搜索)
│   ├── crypt/       # rclone 兼容的 EME-AES / NaCl Secretbox 加解密
│   ├── fs/          # FUSE 文件系统实现 (cgofuse)
│   ├── log/         # lumberjack 日志轮转
│   ├── quark/       # 夸克网盘 HTTP API 客户端
│   ├── staging/     # 写入暂存区 (release-before-write)
│   └── sync/        # 异步并发上传 sync loop
├── docs/            # 开发文档 + bug fix 记录
└── openspec/        # qrypt 专用的 OpenSpec 变更
```

## WHERE TO LOOK

| Concept | Location | Notes |
|---------|----------|-------|
| CLI entry | `cmd/qrypt/main.go` | cobra root command |
| FUSE ops | `internal/fs/` | getattr/readdir/mkdir/write/rename/delete |
| Encryption | `internal/crypt/` | NaCl + EME-AES, rclone compat |
| Upload sync | `internal/sync/uploader.go` | debounce + retry for write-back |
| Quark API | `internal/quark/` | file/upload/manage API endpoints |
| Staging | `internal/staging/store.go` | release-before-write pattern |
| Build | `go build -o qrypt ./cmd/qrypt` | single binary output |

## CONVENTIONS

- **Testing**: `_test.go` alongside source; e2e test in `internal/fs/e2e_test.go`
- **Config search**: `./qrypt.toml` → `~/.config/qrypt/qrypt.toml` → `~/.qrypt.toml` → `/etc/qrypt/qrypt.toml`
- **Mount**: macOS needs macFUSE, Linux needs libfuse + fuse group
- **Signals**: Ctrl+C triggers graceful unmount + upload flush

## COMMANDS

```bash
go build -o qrypt ./cmd/qrypt
go test ./...
./qrypt mount -f qrypt.toml
```
