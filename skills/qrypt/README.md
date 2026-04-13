# Qrypt - 夸克网盘加密挂载工具

Qrypt 是一个专为 macOS 优化的夸克网盘加密挂载工具。它通过 FUSE 将夸克网盘挂载为本地磁盘，并支持实时加解密、LRU 分块缓存和元数据过滤。

## 特性

- **透明加密**: 采用 AES-GCM (64KB 分块) 实时加解密。
- **macOS 优化**: 过滤 `.DS_Store` 等无用请求，提升 Finder 响应速度。
- **高效缓存**: 支持本地磁盘分块缓存，采用 LRU 清理策略（支持高低水位线）。
- **高性能驱动**: 基于 Go 语言实现，支持分块并行下载和上传。

## 安装

1.  **安装 macFUSE**:
    - 前往 [macfuse.io](https://macfuse.github.io/) 下载并安装。
    - 在 Apple Silicon 芯片上，可能需要重启并进入恢复模式以允许内核扩展。
2.  **编译 Qrypt**:
    ```bash
    git clone ...
    cd skills/qrypt
    go build ./cmd/qrypt
    ```

## 使用方法

### 挂载

```bash
./qrypt mount -c "你的Cookie" -k "你的加密密钥" -m "/Users/你的用户名/QuarkDrive"
```

参数说明：
- `-c, --cookie`: 夸克网盘的 Cookie（需从浏览器抓取）。
- `-k, --key`: 16 字符以上的加密密钥。
- `-m, --mount`: 本地挂载点。
- `-a, --cache`: (可选) 本地缓存目录，默认 `./cache`。

## 注意事项

- **加解密兼容性**: Qrypt 采用独立的分块加密格式，不保证与其他工具（如 rclone crypt）直接互通。
- **秒传限制**: 由于采用全量加密，上传过程不支持夸克原生的秒传功能。
- **性能**: 4K 视频播放建议配置较大的 LRU 缓存空间。
