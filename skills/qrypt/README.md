# Qrypt - 夸克网盘 rclone 兼容加密挂载工具

Qrypt 是一个跨平台的 Quark Drive 挂载工具，支持 macOS 和 Linux。它通过 FUSE 将夸克网盘目录挂载到本地，并兼容 rclone crypt 的核心加密逻辑。

## 特性

- **rclone 兼容加解密**：支持 scrypt 派生、EME-AES 文件名处理、NaCl Secretbox 分块内容处理
- **配置文件支持**：TOML 格式配置文件，命令行参数可覆盖
- **路径挂载**：支持通过 `--root-path` 指定网盘子目录作为挂载根
- **本地缓存**：内存索引 + 磁盘 batch 文件分块缓存
- **跨平台支持**：macOS (macFUSE) 和 Linux (libfuse)
- **智能过滤**：过滤 `.DS_Store` 等元数据请求，减少无效云端操作
- **上传闭环**：支持分片上传后的 hash 上报与完成确认流程
- **并发上传**：支持多文件并发同步
- **优雅退出**：Ctrl+C 信号处理，安全卸载

## 环境要求

### macOS

- 安装 macFUSE：<https://macfuse.github.io/>
- Go 1.22+

### Linux

- 安装 libfuse：`sudo apt install fuse` (Debian/Ubuntu) 或 `sudo yum install fuse` (CentOS/RHEL)
- Go 1.22+
- 用户需要在 fuse 组：`sudo usermod -a -G fuse $USER`

## 构建

### macOS（原生）

需要先安装 [macFUSE](https://macfuse.github.io/)，然后：

```bash
cd skills/qrypt
go build -o qrypt ./cmd/qrypt    # CGO_ENABLED=1（默认）
```

编译产物包含全部命令（`mount` 需要 CGo/macFUSE）。

### Linux（原生）

```bash
sudo apt install libfuse-dev fuse  # Debian/Ubuntu
# 或 sudo yum install fuse-devel fuse  # CentOS/RHEL
go build -o qrypt ./cmd/qrypt      # CGO_ENABLED=1（默认）
```

编译产物包含全部命令。

### 跨平台编译（macOS → Linux）

如果目标机器没有 FUSE 环境，或者只需要 CLI 工具（不含 `mount` 命令）：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o qrypt-linux ./cmd/qrypt
```

`mount` 命令通过 `cgofuse` 依赖 CGo + FUSE C 库，跨平台编译时自动排除。

### 构建变体对比

| 场景 | mount 命令 | 其他命令 | 依赖 |
|------|-----------|---------|------|
| macOS 原生 | ✅ | ✅ | macFUSE |
| Linux 原生 | ✅ | ✅ | libfuse-dev + fuse |
| macOS→Linux 跨平台 | ❌ | ✅ | 无 |
| macOS→Linux 跨平台 + mount | ✅（需交叉编译器） | ✅ | libfuse-dev + x86_64-linux-gnu-gcc |

## 快速开始

### 1. 生成配置文件

```bash
./qrypt init
```

这会在当前目录生成 `qrypt.toml` 配置文件。

### 2. 编辑配置文件

```toml
# qrypt.toml

[quark]
cookie = "你的夸克Cookie"
root_path = "/"

[encryption]
password = "你的rclone密码"
salt = ""  # 可选

[mount]
point = "~/QryptMount"

[cache]
dir = "~/.qrypt/cache"
max_size = "10GB"
```

### 3. 挂载

```bash
./qrypt mount -f qrypt.toml
```

## 命令行用法

### 挂载命令

```bash
# 使用配置文件
./qrypt mount -f qrypt.toml

# 使用命令行参数（覆盖配置文件）
./qrypt mount \
    -c "<QUARK_COOKIE>" \
    -p "<RCLONE_PASSWORD>" \
    -s "<RCLONE_SALT>" \
    -r "/Encrypt" \
    -m "/home/user/Qrypt" \
    -a "~/.qrypt/cache"
```

### 命令说明

| 命令 | 说明 |
|------|------|
| `qrypt mount` | 挂载夸克网盘 |
| `qrypt init` | 生成示例配置文件 |
| `qrypt --help` | 显示帮助信息 |

### 参数说明

| 参数 | 短写 | 说明 | 默认值 |
|------|------|------|--------|
| `--config` | `-f` | 配置文件路径 | 自动搜索 |
| `--cookie` | `-c` | 夸克 Cookie | - |
| `--password` | `-p` | rclone password | - |
| `--salt` | `-s` | rclone salt | 空 |
| `--root-path` | `-r` | 网盘挂载路径 | `/` |
| `--mount` | `-m` | 本地挂载点 | - |
| `--cache` | `-a` | 缓存目录 | `~/.qrypt/cache` |

## 配置文件

### 配置文件搜索顺序

1. `./qrypt.toml` (当前目录)
2. `~/.config/qrypt/qrypt.toml`
3. `~/.qrypt.toml`
4. `/etc/qrypt/qrypt.toml`

### 完整配置示例

```toml
[quark]
# 夸克网盘 Cookie（必填）
cookie = "your_cookie_here"
# 挂载的网盘路径（默认根目录）
root_path = "/"

[encryption]
# 加密密码（必填，与 rclone crypt 兼容）
password = "your_password"
# 加密盐（可选）
salt = ""

[cache]
# 缓存目录
dir = "~/.qrypt/cache"
# 最大缓存大小 (支持 KB, MB, GB, TB)
max_size = "10GB"

[mount]
# 本地挂载点
point = "~/QryptMount"
# 允许其他用户访问（需要 /etc/fuse.conf 配置）
allow_other = false

[sync]
# 同步失败重试次数
max_retries = 3
# 并发上传数
concurrent_uploads = 3
# 目录列表缓存时间 (支持 s, m, h)
dir_cache_ttl = "5m"

[log]
# 日志级别: debug, info, warn, error
level = "info"
# 日志文件路径（空则输出到终端）
file = ""
```

## 使用示例

### 基本使用

```bash
# 挂载到默认目录
./qrypt mount -f qrypt.toml

# 挂载后操作
cd ~/QryptMount
ls
echo "hello" > test.txt
cat test.txt
```

### 卸载

```bash
# Ctrl+C 优雅退出（推荐）
# 或手动卸载
fusermount -u ~/QryptMount  # Linux
umount ~/QryptMount          # macOS
```

### 挂载子目录

```toml
[quark]
root_path = "/MyEncryptedFolder"
```

## 测试

```bash
# 运行单元测试
go test -short ./...

# 运行 E2E 测试（需要配置环境变量）
export QRYPT_TEST_COOKIE="your_cookie"
export QRYPT_TEST_PASSWORD="your_password"
export QRYPT_TEST_REMOTE_PATH="/TestPath"
export QRYPT_TEST_MOUNT_POINT="/tmp/qrypt_test"
export QRYPT_TEST_CACHE_DIR="/tmp/qrypt_cache"

go test -v ./internal/vfs/...
```

## 故障排除

### 1. 挂载失败

```bash
# 检查 FUSE 是否安装
which fusermount  # Linux
ls /Library/Filesystems/macfuse.fs  # macOS

# 检查权限
sudo usermod -a -G fuse $USER  # Linux
```

### 2. Ctrl+C 无法退出

```bash
# 强制卸载
fusermount -uz ~/QryptMount
# 或
sudo umount -l ~/QryptMount
```

### 3. 文件上传后消失

这是夸克 API 索引延迟导致的，Qrypt 已内置 30 秒保护窗口。如果问题持续，请检查日志。

## 与 rclone crypt 兼容性

Qrypt 完全兼容 rclone crypt 的加密格式：

- 密码派生：scrypt(N=16384, r=8, p=1)
- 文件名加密：EME-AES (宽块模式)
- 内容加密：NaCl Secretbox (XSalsa20-Poly1305)
- 分块大小：默认 64KB + 16 字节 nonce + 16 字节 MAC

你可以用 rclone 创建加密内容，然后用 Qrypt 挂载，反之亦然。

## 许可证

MIT License
