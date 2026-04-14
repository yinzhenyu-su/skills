# Qrypt - 夸克网盘 rclone 兼容加密挂载工具

Qrypt 是一个面向 macOS 的 Quark Drive 挂载工具。它通过 macFUSE 将夸克网盘目录挂载到本地，并兼容 rclone crypt 的核心加密逻辑。

## 特性

- rclone 兼容加解密：支持 scrypt 派生、EME-AES 文件名处理、NaCl Secretbox 分块内容处理。
- 路径挂载：支持通过 `--root-path` 指定网盘子目录作为挂载根。
- 本地缓存：支持分块缓存与元数据持久化（SQLite）。
- macOS 适配：过滤 `.DS_Store` 等元数据请求，减少无效云端操作。
- 上传闭环：支持分片上传后的 hash 上报与完成确认流程。

## 环境要求

1. 安装 macFUSE（<https://macfuse.github.io/）。>
2. Go 1.22+（建议与项目 `go.mod` 保持一致）。

## 构建

```bash
cd skills/qrypt
go build -o qrypt ./cmd/qrypt
```

## 用法

### 挂载命令

```bash
./qrypt mount \
    --cookie "<QUARK_COOKIE>" \
    --password "<RCLONE_PASSWORD>" \
    --salt "<RCLONE_SALT_OPTIONAL>" \
    --root-path "/Encrypt" \
    --mount "/Users/<you>/Qrypt" \
    --cache "./cache"
```

### 参数说明

- `-c, --cookie`：夸克 Cookie（必填）。
- `-p, --password`：rclone password（必填）。
- `-s, --salt`：rclone salt（可选）。
- `-r, --root-path`：要挂载的网盘路径，默认 `/`。
- `-m, --mount`：本地挂载点（必填）。
- `-a, --cache`：本地缓存目录，默认 `./cache`。

## 验证

```bash
go test ./...
./qrypt --help
```

## 已知限制

- 在部分 macOS 环境中，复制到挂载目录可能出现 `Operation not permitted`，通常与系统权限或 FUSE 策略相关。
- 若写入受限，请优先检查终端/IDE/macFUSE 权限与系统安全策略。
